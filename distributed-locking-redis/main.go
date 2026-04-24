package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redsync/redsync/v4"
	syncredis "github.com/go-redsync/redsync/v4/redis"
	goredis "github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

const (
	inventoryKey = "inventory:iphone15"
	mutexName    = "flash_sale_lock:iphone15"
	totalStock   = 5
	totalUsers   = 20
)

// Global metrics for simulation report
var (
	successfulPurchases int32
	failedPayments      int32
	soldOutFailures     int32
	lockFailures        int32
)

func main() {
	// 1. Set up 3 Redis Nodes
	redisNodeURLs := []string{"localhost:6379", "localhost:6380", "localhost:6381"}
	var pools []syncredis.Pool
	// Also keep a raw redis client to primary node for atomic ops like inventory setup
	var primaryClient *redis.Client

	for i, url := range redisNodeURLs {
		client := redis.NewClient(&redis.Options{Addr: url})
		// Test connection
		if err := client.Ping(ctx).Err(); err != nil {
			log.Fatalf("Failed to connect to Redis %s: %v", url, err)
		}
		pools = append(pools, goredis.NewPool(client))
		if i == 0 {
			primaryClient = client
		}
	}

	// 2. Setup Redsync
	rs := redsync.New(pools...)

	// Reset inventory for the run
	primaryClient.Set(ctx, inventoryKey, totalStock, 0)
	fmt.Printf("=== Simulation Starting ===\n")
	fmt.Printf("Total Stock: %d | Concurrent Users: %d\n", totalStock, totalUsers)
	fmt.Println("--------------------------------------------------")

	rand.Seed(time.Now().UnixNano())

	var wg sync.WaitGroup
	wg.Add(totalUsers)

	startTime := time.Now()

	// 3. Simulate 20 concurrent users attempting to buy
	for i := 1; i <= totalUsers; i++ {
		go func(userID int) {
			defer wg.Done()

			// Slightly randomize request arrival
			time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)

			attemptPurchase(rs, primaryClient, userID)
		}(i)
	}

	wg.Wait()

	// 4. Summarize Simulation Results
	fmt.Println("--------------------------------------------------")
	fmt.Printf("=== Simulation Complete (Duration: %v) ===\n", time.Since(startTime))
	fmt.Printf("Successful Purchases : %d\n", successfulPurchases)
	fmt.Printf("Failed Payments      : %d (Inventory Restored)\n", failedPayments)
	fmt.Printf("Sold Out Rejections  : %d\n", soldOutFailures)
	fmt.Printf("Lock Drops/Busy      : %d\n", lockFailures)

	finalStock, _ := primaryClient.Get(ctx, inventoryKey).Int()
	fmt.Printf("Remaining Stock in Redis: %d\n", finalStock)
}

func attemptPurchase(rs *redsync.Redsync, rdb *redis.Client, userID int) {
	// 1. Setup Mutex (small expiry because checkout queue should be fast)
	mutex := rs.NewMutex(mutexName, redsync.WithExpiry(2*time.Second), redsync.WithTries(1))

	// 2. Lock Acquisition
	if err := mutex.Lock(); err != nil {
		fmt.Printf("User %d: Server Busy (Lock failed). Dropped.\n", userID)
		atomic.AddInt32(&lockFailures, 1)
		return // Fail fast!
	}

	// Ensure we release the lock after we decrement inventory
	defer mutex.Unlock()

	// 3. Check Inventory (Protected block)
	stock, err := rdb.Get(ctx, inventoryKey).Int()
	if err != nil {
		fmt.Printf("User %d: Error reading inventory: %v\n", userID, err)
		return
	}

	if stock <= 0 {
		fmt.Printf("User %d: Sold Out! (stock = %d)\n", userID, stock)
		atomic.AddInt32(&soldOutFailures, 1)
		return
	}

	// 4. Decrement Inventory in Redis
	_, err = rdb.Decr(ctx, inventoryKey).Result()
	if err != nil {
		fmt.Printf("User %d: Failed to decrement stock: %v\n", userID, err)
		return
	}

	fmt.Printf("User %d: Locked item for checkout! (Stock left implicitly: %d)\n", userID, stock-1)

	// Release lock explicitly before async payment block
	mutex.Unlock()

	// 5. ASYNC PROCESS: Simulate Payment Gateway Interaction
	processPayment(rdb, userID)
}

func processPayment(rdb *redis.Client, userID int) {
	// Simulate web request latency to payment provider (200-500ms)
	latency := time.Duration(200+rand.Intn(300)) * time.Millisecond
	time.Sleep(latency)

	// Simulate a 30% chance of payment failure
	if rand.Float32() < 0.3 {
		fmt.Printf("User %d: ❌ Payment Failed! Restoring inventory...\n", userID)
		atomic.AddInt32(&failedPayments, 1)

		// On failure, worker asynchronously restores the inventory
		rdb.Incr(ctx, inventoryKey)
		return
	}

	// Payment Success
	fmt.Printf("User %d: ✅ Payment Successful! Order created.\n", userID)
	atomic.AddInt32(&successfulPurchases, 1)
}
