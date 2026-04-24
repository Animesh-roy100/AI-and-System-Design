package main

import (
	"container/heap"
	"fmt"
	"math"
	"sort"
	"time"
)

// --- 1. Models & Core Entities ---

type Location struct {
	Lat float64
	Lng float64
}

// DistanceTo calculates a simple Euclidean distance (for dry run purposes).
// In production, this would use Haversine (for geospatial coordinates) or Mapbox/Google Routing APIs.
func (l Location) DistanceTo(other Location) float64 {
	return math.Sqrt(math.Pow(l.Lat-other.Lat, 2) + math.Pow(l.Lng-other.Lng, 2))
}

type Driver struct {
	ID        string
	Rating    float64 // Scale up to 5.0
	Location  Location
	Available bool
}

type OrderStatus string

const (
	Pending  OrderStatus = "PENDING"
	Assigned OrderStatus = "ASSIGNED"
	Failed   OrderStatus = "FAILED"
)

type Order struct {
	ID        string
	IsVIP     bool
	Location  Location
	Status    OrderStatus
	Timestamp time.Time // To track first-come-first-serve for tiebreakers

	// Internal index used by heap.Interface
	index int
}

// --- 2. Data Structures (Priority Queue) ---

// OrderQueue implements heap.Interface and holds Orders.
// It guarantees that VIPs pop first, then by earliest timestamp for tie-breaks.
type OrderQueue []*Order

func (pq OrderQueue) Len() int { return len(pq) }

func (pq OrderQueue) Less(i, j int) bool {
	// VIP wins over Non-VIP priority
	if pq[i].IsVIP != pq[j].IsVIP {
		return pq[i].IsVIP
	}
	// If both are same tier, earliest timestamp wins
	return pq[i].Timestamp.Before(pq[j].Timestamp)
}

func (pq OrderQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

// Push adds x to the queue
func (pq *OrderQueue) Push(x interface{}) {
	n := len(*pq)
	order := x.(*Order)
	order.index = n
	*pq = append(*pq, order)
}

// Pop removes the highest priority item from the queue
func (pq *OrderQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	order := old[n-1]
	old[n-1] = nil   // Free reference to avoid memory leak
	order.index = -1 // For safety
	*pq = old[0 : n-1]
	return order
}

// --- 3. Design Strategy (Strategy Pattern) ---

// MatchingStrategy defines an interface so matching logic can easily be swapped
// during surging, bad weather, or different city zones.
type MatchingStrategy interface {
	Match(order *Order, drivers []*Driver) *Driver
}

// StandardMatchingStrategy is the default logical strategy.
type StandardMatchingStrategy struct {
	MaxRadius float64
}

// Match applies the constraints:
// 1. VIPs get highest rated driver within radius
// 2. Non-VIPs get closest available driver
func (s *StandardMatchingStrategy) Match(order *Order, drivers []*Driver) *Driver {
	var candidates []*Driver

	// Filter by availability and radius
	for _, d := range drivers {
		dist := d.Location.DistanceTo(order.Location)
		if d.Available && dist <= s.MaxRadius {
			candidates = append(candidates, d)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	if order.IsVIP {
		// VIP logic: Sort by Rating (Descending)
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].Rating > candidates[j].Rating
		})
	} else {
		// Regular logic: Sort by Distance (Ascending)
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].Location.DistanceTo(order.Location) < candidates[j].Location.DistanceTo(order.Location)
		})
	}

	return candidates[0]
}

// --- 4. Dispatcher Service ---

type Dispatcher struct {
	Orders   OrderQueue
	Drivers  []*Driver
	Strategy MatchingStrategy
}

func NewDispatcher(strategy MatchingStrategy) *Dispatcher {
	pq := make(OrderQueue, 0)
	heap.Init(&pq)
	return &Dispatcher{
		Orders:   pq,
		Strategy: strategy,
	}
}

func (d *Dispatcher) AddOrder(o *Order) {
	heap.Push(&d.Orders, o)
}

func (d *Dispatcher) AddDriver(drv *Driver) {
	d.Drivers = append(d.Drivers, drv)
}

// RunDispatch processes the priority queue synchronously (dry run mode)
func (d *Dispatcher) RunDispatch() {
	fmt.Println("Processing all pending orders...")
	fmt.Println("--------------------------------")

	for d.Orders.Len() > 0 {
		order := heap.Pop(&d.Orders).(*Order)

		fmt.Printf("Evaluating Order: %s | VIP: %v\n", order.ID, order.IsVIP)

		driver := d.Strategy.Match(order, d.Drivers)
		if driver != nil {
			order.Status = Assigned
			driver.Available = false // Soft lock the driver
			fmt.Printf("✅ MATCHED -> Driver %s | Rating: %.1f | Dist: %.2f\n\n",
				driver.ID, driver.Rating, driver.Location.DistanceTo(order.Location))
		} else {
			order.Status = Failed
			fmt.Printf("❌ FAILED -> No driver available within radius.\n\n")
		}
	}
}

// --- 5. Dry Run Execution ---
func main() {
	fmt.Println("=== Driver Assignment System (Dry Run) ===")

	// Max Radius 10 units
	dispatcher := NewDispatcher(&StandardMatchingStrategy{MaxRadius: 10.0})

	// Drivers Setup
	// Driver 1: Very highly rated but further away
	dispatcher.AddDriver(&Driver{ID: "DRV_A(4.9_rating)", Rating: 4.9, Location: Location{Lat: 0, Lng: 4}, Available: true})

	// Driver 2: Okay rated, but very close
	dispatcher.AddDriver(&Driver{ID: "DRV_B(4.1_rating)", Rating: 4.1, Location: Location{Lat: 0, Lng: 1}, Available: true})

	// Driver 3: Moderately rated, away
	dispatcher.AddDriver(&Driver{ID: "DRV_C(4.5_rating)", Rating: 4.5, Location: Location{Lat: 5, Lng: 5}, Available: true})

	// Orders Setup (Scenario: Non-VIP comes in first chronologically, VIP comes in second)
	t1 := time.Now()
	t2 := t1.Add(1 * time.Second)

	fmt.Println("Incoming Order: ord_regular_1 (Non-VIP) at Lat: 0, Lng: 0")
	dispatcher.AddOrder(&Order{
		ID: "ord_regular_1", IsVIP: false, Location: Location{Lat: 0, Lng: 0}, Status: Pending, Timestamp: t1,
	})

	fmt.Println("Incoming Order: ord_vip_2 (VIP) at Lat: 0, Lng: 0")
	dispatcher.AddOrder(&Order{
		ID: "ord_vip_2", IsVIP: true, Location: Location{Lat: 0, Lng: 0}, Status: Pending, Timestamp: t2,
	})

	fmt.Println("\nQueue Snapshot before dispatching:")
	fmt.Printf("- Elements in queue: %d\n\n", dispatcher.Orders.Len())

	// Run
	dispatcher.RunDispatch()
}
