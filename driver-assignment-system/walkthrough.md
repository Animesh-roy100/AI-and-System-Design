# Driver Assignment System — Walkthrough

I have successfully designed and built the requested Driver Assignment System in Go. The complete, executable source code resides locally at: 
[main.go](file:///Users/animesh5.roy/projects/System%20Design/driver-assignment-system/main.go)

## Approach & Design Patterns

The solution is an in-memory LLD architecture built for extensibility:

1. **Strategic Priority Queuing (`container/heap`)**
   The core `OrderQueue` implements a Max-Heap structure that processes logic efficiently. VIP orders are **guaranteed** to bubble to the top of the queue and be processed before standard orders (even if the standard orders were placed earlier). Standard chronological logic serves as a tie-breaker.
2. **Strategy Pattern (`MatchingStrategy`)**
   Since matching criteria frequently change in real production environments (due to surge pricing, weather conditions, or local regulations), the matching logic is heavily decoupled from the core `Dispatcher`. We created a `StandardMatchingStrategy` which handles the rule logic. You could easily inject a `RainyDayMatchingStrategy` later without modifying the dispatcher code.
3. **Data Structuring & Indexing**
   For available drivers within proximity, we employ iterative spatial bounding and Go's `sort.SliceStable()` algorithm. It allows us to dynamically pivot between sorting by raw proximity (`Distance ASC`) or service quality (`Rating DESC`).

## Execution & Dry Run Let's Review

I configured a scenario with **3 Drivers** and **2 Orders**:
- **DRV_A**: Rating 4.9 (Highest rating, further away)
- **DRV_B**: Rating 4.1 (Lowest rating, but very close)
- **DRV_C**: Rating 4.5 (Moderate rating, far away)
- **Order 1**: Arrives first chronologically, is `Non-VIP`.
- **Order 2**: Arrives a second later, is `VIP`.

### Console Output
```bash
=== Driver Assignment System (Dry Run) ===

Incoming Order: ord_regular_1 (Non-VIP) at Lat: 0, Lng: 0
Incoming Order: ord_vip_2 (VIP) at Lat: 0, Lng: 0

Queue Snapshot before dispatching:
- Elements in queue: 2

Processing all pending orders...
--------------------------------
Evaluating Order: ord_vip_2 | VIP: true
✅ MATCHED -> Driver DRV_A(4.9_rating) | Rating: 4.9 | Dist: 4.00

Evaluating Order: ord_regular_1 | VIP: false
✅ MATCHED -> Driver DRV_B(4.1_rating) | Rating: 4.1 | Dist: 1.00
```

### Analysis of the Result

1. **Priority Sorting Worked**: Note how `ord_vip_2` is evaluated **first**, even though it arrived chronologically after `ord_regular_1`. The heap bubbled the VIP request immediately.
2. **Constraint Execution Worked**: 
   - `ord_vip_2` bypasses the closest driver (`DRV_B`) completely because VIP logic mandates that we match with the **Highest Rated Available Driver** first. Therefore, it pulls `DRV_A` (rating 4.9) from 4 miles away.
   - `ord_regular_1` is evaluated next. As a non-VIP request, the core constraint states we match with the **Closest Available Driver**. It ignores ratings and successfully locks onto `DRV_B` from just 1 mile away.
