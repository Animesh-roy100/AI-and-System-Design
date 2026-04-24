# Driver Assignment System — Implementation Plan

## Goal
Design and implement an in-memory LLD (Low-Level Design) for a dispatch system that assigns drivers to orders based on constraints like VIP status, Driver ratings, and proximity. Provide the executable Go code and a dry run.

## Approach & Design Patterns

1. **Entities**:
   - `Order`: `ID`, `Location`, `IsVIP`, `Status` (PENDING, ASSIGNED)
   - `Driver`: `ID`, `Location`, `Rating`, `IsAvailable`
2. **Design Patterns**:
   - **Strategy Pattern**: The exact matching rules frequently change in production (e.g., surge pricing matching vs. normal matching). We will define a `MatchingStrategy` interface so you can swap out matching logic easily.
   - **Observer/Event Pattern (Optional)**: To emit an event when an order is matched so the driver can be notified.
3. **Data Structures**:
   - **Order Queue**: A Priority Queue (Max-Heap) where VIP orders have higher priority than regular orders.
   - **Driver Pool**: A list or spatial index (for this LLD, a slice of drivers filtered by distance) where available drivers are sorted by `Rating` or `Distance` using Go's `sort` slice functionality.

## Proposed Code Structure (Golang)

### 1. Core Models (`models.go`)
Define `Driver` and `Order` structs.

### 2. Matching Engine (`matching.go`)
```go
type MatchingStrategy interface {
    Match(orders []*Order, drivers []*Driver) map[string]string // returns map of OrderID -> DriverID
}
```

**Concrete Strategy:** `DefaultMatchingStrategy`
- **Rule 1:** Pop the highest priority order (VIP orders first).
- **Rule 2:** Filter all `IsAvailable == true` drivers within a specific radius (e.g., 5km).
- **Rule 3:** For VIP orders, sort the available drivers by `Rating` descending. Pick the top one.
- **Rule 4:** For Non-VIP orders, sort the available drivers by `Distance` ascending. Pick the closest one.
- **Rule 5:** Mark driver as `IsAvailable = false` and order as `ASSIGNED`.

### 3. Dry Run Execution (`main.go`)
We will create a specific scenario:
- **Order 1 (VIP)** at Location A.
- **Order 2 (Non-VIP)** at Location B.
- **Driver X** (Rating 4.9, Close to A).
- **Driver Y** (Rating 4.2, Very close to A).
- **Driver Z** (Rating 4.8, Close to B).

We will trace the output to show that Order 1 (VIP) gets Driver X (highest rating despite Driver Y being closer), and Order 2 gets Driver Z.

## Open Questions for Review
1. **Driver Prioritization for VIPs**: Currently, the plan says VIPs get the *highest rated* driver within a radius, while non-VIPs get the *closest* driver. Does this condition align with your expectations, or would you like different balancing logic?
2. **Concurrency**: Do you want the system to handle concurrent incoming orders (using channels/goroutines), or is a synchronous queue-based processing function sufficient for this LLD?

## Verification Plan
1. Create a `driver-assignment-system` directory.
2. Write the Go code in `main.go`.
3. Execute `go run main.go` to perform the dry run and capture the console output to prove the matching logic.
