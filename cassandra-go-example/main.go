package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gocql/gocql"
)

// Event represents the activity tracking event
type Event struct {
	EventID        gocql.UUID `json:"event_id"`
	UserID         gocql.UUID `json:"user_id"`
	EventType      string     `json:"event_type"`
	EventTimestamp time.Time  `json:"event_timestamp"`
	Metadata       string     `json:"metadata"`
}

// App encapsulates our application dependencies
type App struct {
	Session *gocql.Session
}

func main() {
	// 1. Initialize Cassandra Session
	session, err := initCassandra()
	if err != nil {
		log.Fatalf("Failed to initialize Cassandra: %v", err)
	}
	defer session.Close()

	app := &App{Session: session}

	// 2. Setup HTTP Router
	mux := http.NewServeMux()
	mux.HandleFunc("/events", app.eventsHandler)

	// 3. Start HTTP Server with Graceful Shutdown
	srv := &http.Server{
		Addr:    ":8000",
		Handler: mux,
	}

	// Start server in a goroutine
	go func() {
		log.Println("Starting server on :8000")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// 4. Start the Load Simulator in the background
	go simulateHighVelocityData()

	// 5. Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Give the server 5 seconds to finish processing existing requests
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting gracefully")
}

// initCassandra configures a robust connection to Cassandra
func initCassandra() (*gocql.Session, error) {
	// Wait for Cassandra to be ready (create keyspace/table if not exists)
	if err := setupSchema(); err != nil {
		return nil, err
	}

	cluster := gocql.NewCluster("127.0.0.1")
	cluster.Port = 9042

	// --- ROBUSTNESS IMPROVEMENTS ---
	cluster.Keyspace = "activity_tracking"
	cluster.Consistency = gocql.LocalOne      // Prioritize write speed over strict consistency
	cluster.Timeout = 5 * time.Second         // Timeout for a single query
	cluster.ConnectTimeout = 10 * time.Second // Timeout to establish connection
	cluster.NumConns = 10                     // Connection pool size per host
	
	// Add retry policy for transient failures
	cluster.RetryPolicy = &gocql.ExponentialBackoffRetryPolicy{
		NumRetries: 3,
		Min:        100 * time.Millisecond,
		Max:        1 * time.Second,
	}

	return cluster.CreateSession()
}

func setupSchema() error {
	cluster := gocql.NewCluster("127.0.0.1")
	cluster.Port = 9042
	cluster.Timeout = 10 * time.Second

	session, err := cluster.CreateSession()
	if err != nil {
		return fmt.Errorf("could not connect to cassandra: %w", err)
	}
	defer session.Close()

	// Create Keyspace
	if err := session.Query(`
		CREATE KEYSPACE IF NOT EXISTS activity_tracking 
		WITH REPLICATION = { 'class' : 'SimpleStrategy', 'replication_factor' : 1 };
	`).Exec(); err != nil {
		return fmt.Errorf("failed to create keyspace: %w", err)
	}

	// Create Table
	if err := session.Query(`
		CREATE TABLE IF NOT EXISTS activity_tracking.events (
			event_id UUID PRIMARY KEY,
			user_id UUID,
			event_type TEXT,
			event_timestamp TIMESTAMP,
			metadata TEXT
		);
	`).Exec(); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

// eventsHandler routes GET and POST requests
func (a *App) eventsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.createEvent(w, r)
	case http.MethodGet:
		a.getRecentEvents(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) createEvent(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		UserID    gocql.UUID `json:"user_id"`
		EventType string     `json:"event_type"`
		Metadata  string     `json:"metadata"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Generate Event fields
	event := Event{
		EventID:        gocql.TimeUUID(),
		UserID:         payload.UserID,
		EventType:      payload.EventType,
		EventTimestamp: time.Now(),
		Metadata:       payload.Metadata,
	}

	// --- ROBUSTNESS IMPROVEMENT: Context Cancellation ---
	// Using WithContext(r.Context()) cancels the Cassandra query if the HTTP client drops the connection
	err := a.Session.Query(
		`INSERT INTO events (event_id, user_id, event_type, event_timestamp, metadata) VALUES (?, ?, ?, ?, ?)`,
		event.EventID, event.UserID, event.EventType, event.EventTimestamp, event.Metadata,
	).WithContext(r.Context()).Exec()

	if err != nil {
		log.Printf("Failed to insert event: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(event)
}

func (a *App) getRecentEvents(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	// --- ROBUSTNESS IMPROVEMENT: Context & Iterators ---
	// Using Iter() handles large result sets memory-efficiently by streaming rows
	iter := a.Session.Query(`SELECT event_id, user_id, event_type, event_timestamp, metadata FROM events LIMIT ?`, limit).
		WithContext(r.Context()).
		Iter()

	var events []Event
	var event Event

	// Scan each row efficiently
	for iter.Scan(&event.EventID, &event.UserID, &event.EventType, &event.EventTimestamp, &event.Metadata) {
		events = append(events, event)
	}

	if err := iter.Close(); err != nil {
		log.Printf("Failed to fetch events: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// simulateHighVelocityData generates high load by spawning concurrent workers
func simulateHighVelocityData() {
	// Give the server a moment to start
	time.Sleep(2 * time.Second)
	log.Println("Starting high-velocity simulation...")

	userIDs := []gocql.UUID{gocql.TimeUUID(), gocql.TimeUUID(), gocql.TimeUUID()}
	eventTypes := []string{"page_view", "click", "interaction"}
	metadataList := []string{`{"page": "home"}`, `{"button": "signup"}`, `{"section": "footer"}`}

	// --- ROBUSTNESS IMPROVEMENT: Concurrent Worker Pool ---
	// Using workers instead of a single sequential loop truly simulates high-throughput
	numWorkers := 10        // Number of concurrent writer threads
	eventsPerWorker := 200  // Total 2000 events

	var wg sync.WaitGroup
	startTime := time.Now()

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// --- ROBUSTNESS IMPROVEMENT: Connection Reuse ---
			// Custom transport allows high connection reuse, preventing TCP port exhaustion
			t := http.DefaultTransport.(*http.Transport).Clone()
			t.MaxIdleConns = 100
			t.MaxConnsPerHost = 100
			t.MaxIdleConnsPerHost = 100
			client := &http.Client{Transport: t, Timeout: 5 * time.Second}

			for i := 0; i < eventsPerWorker; i++ {
				payload := map[string]interface{}{
					"user_id":    userIDs[rand.Intn(len(userIDs))],
					"event_type": eventTypes[rand.Intn(len(eventTypes))],
					"metadata":   metadataList[rand.Intn(len(metadataList))],
				}

				eventBytes, _ := json.Marshal(payload)
				req, _ := http.NewRequest("POST", "http://localhost:8000/events", bytes.NewReader(eventBytes))
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				if err != nil {
					log.Printf("Worker %d: Request failed: %v", workerID, err)
					continue
				}
				resp.Body.Close()

				// Minimal delay to let scheduler breathe (remove to blast at 100% max speed)
				time.Sleep(1 * time.Millisecond)
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(startTime)
	log.Printf("Simulation completed! Sent %d events in %v", numWorkers*eventsPerWorker, duration)
}
