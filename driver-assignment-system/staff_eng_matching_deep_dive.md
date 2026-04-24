# Staff-Level Deep Dive: Driver Matching System

In a Staff Engineer interview, it's not enough to draw boxes connecting Kafka, WebSockets, and a Matching Engine. The interviewer wants to see how you handle **state, distributed concurrency, failure modes, partitioning, and exact semantics**.

Here is the deep dive into the three core components.

---

## 1. WebSockets at Scale (State & Routing)

WebSockets are persistent TCP connections. Unlike stateless HTTP requests where any load balancer can route to any API node, WebSockets are **stateful**. If Driver A is connected to `WS-Node-5`, and the Matching Engine wants to send an offer to Driver A, it *must* route that message to `WS-Node-5`.

### A. The Connection Registry Pattern
How does a backend service know which WS node holds a specific driver's connection?
1. **Connection Event:** Driver connects to `WS-Node-5`.
2. **Registry Update:** `WS-Node-5` updates a central Redis store: `SET driver_ws_node:{driver_id} "WS-Node-5" EX 60`.
3. **Heartbeats:** The Node keeps refreshing this TTL as long as the connection responds to WebSocket `PING/PONG` frames.
4. **Targeted Delivery:** When the Matching Engine generates an offer, it drops it into Kafka. A routing service (or the WS cluster itself) reads Redis to find the correct node and forwards the raw TCP payload to `WS-Node-5`.

### B. The Thundering Herd Problem
**Scenario:** `WS-Node-5` crashes. 10,000 drivers are suddenly disconnected.
**Staff Answer:** If all 10,000 driver apps instantly attempt to reconnect, they will DDoS your Load Balancer and Auth service. The client-side SDK *must* implement **Exponential Backoff with Full Jitter** (e.g., reconnect in 1s, 2s, 4s, 8s, plus a random wait time). 

### C. Half-Open Connections
Mobile networks are notoriously flaky. A driver drives into a tunnel; their phone drops the network, but the server never receives a `TCP FIN` packet. The server thinks the connection is open (Half-Open). 
**Staff Answer:** Relying purely on TCP Keep-Alives is insufficient. The Application layer must enforce a 5-second `PING` interval. If `PONG` is missing twice, the server forcefully closes the socket and flags the driver as offline.

---

## 2. Apache Kafka (Partitioning & Semantics)

At scale, a single Kafka topic (`raw_orders`) will bottleneck. We must partition it. But *how* we partition is the most critical design decision.

### A. Partitioning Strategy: City/Geohash Locality
If we partition randomly (Round Robin), an order from Mumbai might end up on Partition 1, and an order from Delhi on Partition 2. 
**Staff Idea:** Partition by `CityID` or `Geohash Prefix`. 
* **Why?** This ensures all events (orders, driver location updates) for "Mumbai" land on exactly one partition. 
* **The Result:** We can run a Stateful Matching Engine. The Consumer assigned to the "Mumbai" partition can keep an entirely in-memory data grid of all Mumbai drivers. This bypasses the need to query Redis thousands of times a second. It just reads memory.

### B. Delivery Semantics (At-Least-Once)
If the Matching Engine crashes *while* processing an order, we cannot lose the order.
**Staff Answer:** We manually commit Kafka offsets *only* after the order's state is safely mutated in the Database/Redis to "OFFERED". If the engine crashes prior, the re-balanced consumer will pull the exact same order again.

---

## 3. The Matching Engine (Concurrency & Split-Brain)

The hardest problem in the Matching Engine is the **Concurrent Double Booking** (Split Brain). 
**Scenario:** Two users in the exact same location request a ride exactly at ms `100`. The Matching Engine fetches the closest 10 drivers. Driver `DRV_A` is the absolute best match for *both* orders. 

If we process concurrently, both threads might send trips to `DRV_A`.

### A. Distributed Locks (Pessimistic) vs Lua Scripts (Optimistic)
A Staff Engineer avoids long distributed locks (Redis Redlock) for this step because it kills throughput.

**The Lua Solution:**
1. Both threads attempt to lock `DRV_A` in Redis via an atomic Lua script:
```lua
if redis.call("GET", KEYS[1]) == "AVAILABLE" then
    redis.call("SET", KEYS[1], "OFFERED_TO_" .. ARGV[1])
    return 1 -- Success
else
    return 0 -- Failed (Already locked by another thread)
end
```
2. Thread 1 gets `1` (Success). It proceeds to push the offer to Kafka.
3. Thread 2 gets `0` (Failure). It instantly aborts `DRV_A`, pops the *second* best driver from its local list, and runs the Lua script again. 

### B. The Offer Lifecycle (State Machine)
We do not hard-assign the driver immediately. 
1. **Soft Lock:** Engine sets driver to `OFFERED` (15 second TTL).
2. **Push:** Sends offer via WebSocket.
3. **Wait:** Engine essentially parks the order status. 
4. **Resolution:** 
    - If Driver taps ACCEPT -> DB updates to `EN_ROUTE`. Redis state `BUSY`.
    - If Driver taps DECLINE -> Engine puts the Order back into the Priority Queue, penalizes the declined driver algorithmically, and repeats Step 1 with a new driver.
    - If TTL Expires (15s) -> Same as Decline. The soft-lock drops off Redis automatically, making the driver `AVAILABLE` to others again.
