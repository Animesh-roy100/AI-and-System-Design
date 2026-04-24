# Coupon System Design — Deep Dives & Architecture Decisions

This document acts as an addendum to the core Swiggy Coupon System Design, extending the points specifically around caching, locks, storage patterns, and database concurrency.

---

## 1. Redis Cache: Data Structures & Lifecycles

Redis acts as the backbone for low-latency decision-making (the Eligibility Engine) and concurrency control (the Redemption Service).

Here is an *in-depth* look at exactly what data keys we store, their structures, and their TTL (Time to Live) strategies.

### A. Coupon Catalog/Details
The Eligibility Engine needs to fetch 100s of active coupons within 2-5ms. Querying PostgreSQL for every user's cart page load would crush the DB.
* **Key:** `coupon:active`
* **Data Type:** `SET` (Contains a list of `coupon_id`s that are currently active).
* **TTL:** 5 minutes (or no TTL if using event-driven cache invalidation from the Admin service).

* **Key:** `coupon:detail:{coupon_id}`
* **Data Type:** `HASH` (Stores the coupon rules: `min_cart_value`, `start_time`, `end_time`, `discount_type`, etc.).
* **TTL:** 5 minutes.
* **Why?** It ensures instantaneous bulk reads via `MGET`. Instead of joining tables in Postgres, the Eligibility engine pulls the entire rule set for 50 coupons in a single O(1) Redis network call.

### B. Usage Limit Counters (Atomic)
We must ensure users cannot bypass limits (e.g., "Use only 1 time per user" or "Only 10,000 global redemptions").
* **Key:** `coupon:usage:{coupon_id}:global`
* **Data Type:** `STRING` (containing an integer).
* **TTL:** None (lasts until the coupon expires organically).

* **Key:** `coupon:usage:{coupon_id}:user:{user_id}`
* **Data Type:** `STRING` (containing an integer).
* **TTL:** 24–48 hours (depending on promo rules structure).
* **Why?** We use Redis `INCR` to increment this value automatically. Since Redis processes commands sequentially in a single thread, `INCR` acts as a perfect atomic counter without race conditions.

### C. Session Constraints (Soft Locks)
When a user clicks "Apply Coupon" at the cart, we want to gently reserve the coupon so they aren't surprised if the global limit runs out while they type their card details.
* **Key:** `coupon:reserve:{coupon_id}:user:{user_id}`
* **Data Type:** `STRING` (value: "1").
* **TTL:** 10 minutes.
* **Why?** Acts as a session lock. If the user successfully pays, the system permanently updates the DB counter via webhook. If they abandon checkout, this key auto-expires after 10 mins, freeing the reserved coupon back to the public pool.

---

## 2. Distributed Locks: Why a 10-Second TTL?

During atomic redemption (`POST /coupons/redeem`), we acquire a Redis lock (Redlock) to ensure the user does not submit two concurrent payment webhooks and redeem the coupon twice.

```go
lockKey := fmt.Sprintf("coupon:lock:%s:user:%s", couponID, userID)
```

**Why is the TTL exactly 10 seconds?**

1. **The Maximum Execution Time:** The core DB transaction to mark a coupon as redeemed (`UPDATE user_coupon...`) takes around 10–20 milliseconds. We pad the TTL to cover the 99th percentile network latencies.
2. **Buffer for System Pauses (GC/Network):** If a Garbage Collection (GC) pause happens in the Go server, or if PostgreSQL experiences a temporary network spike, the 20ms transaction might block for 2 to 3 seconds.
3. **The Split-Brain Danger:** If we configured a tight TTL of 2 seconds, and the GC pause lasted 3 seconds, Redis would artificially release the lock at second 2. Another redundant request could then acquire a *new* lock while the first one is still technically processing, leading to the **Double Spend Problem** (the user gets the discount twice).
4. **Failure Recovery (Deadlock Prevention):** Conversely, if the Node crashes *while* holding the lock, it will never release it. With a 10s TTL, the lock naturally expires and unsticks itself after 10 seconds, allowing users to retry.

> [!CAUTION]
> **Lock Values must be UUIDs.**
> To release the lock, the process must check: `if redis.get(lockKey) == my_unique_uuid { redis.del(lockKey) }`. This ensures process A doesn’t accidentally delete process B’s lock if process A woke up late from a GC pause.

---

## 3. Storage Patterns: Cart vs. Active Coupons

### A. Where should the Cart be stored?
**Verdict:** **Redis (Primary) as a `HASH` structure, with a TTL (e.g., 7-30 days).**

* **Why Redis?** A shopping cart is aggressively read and written (user taps "+" to increase biryani quantity, "-" to reduce it). If this hits PostgreSQL every tap, it causes massive write I/O. Redis handles 100k+ ops/sec easily in memory.
* **TTL Strategy:** Usually set to 7, 14, or 30 days. When the TTL expires, abandoned carts are purged natively without requiring a cleanup cron-job. 
* **Async DB Backup (Optional):** If the business requires cart data analysis for re-marketing ("You left biryani in your cart!"), the system drops an event on Kafka on every cart update. A background worker writes the *final* daily state to PostgreSQL or Cassandra.

### B. Where should Active Coupons be stored?
**Verdict:** **PostgreSQL is the Source of Truth; Redis acts purely as a Read-Aside Cache.**

* **Why not just Redis?** A coupon is a financial instrument. If Redis crashes and loses data, we cannot just "forget" that a coupon existed or what its limits were. 
* Coupons are created/edited by Admins in PostgreSQL (`coupon` table). An asynchronous trigger (or application code) syncs the `ACTIVE` status to Redis immediately upon creation.

---

## 4. Deep Dive: Optimistic vs. Pessimistic Locking

When limiting global coupon limits (e.g., "First 10,000 users only"), we must lock the database to ensure we don't accidentally give out 10,001 coupons.

### Why Pessimistic Locking fails at scale

```sql
-- Pessimistic Lock (DO NOT DO THIS AT HIGH SCALE)
BEGIN;
SELECT available_count FROM coupon WHERE id = 'SWIGGY50' FOR UPDATE;
-- Thread pauses, waits for application logic --
UPDATE coupon SET available_count = available_count - 1 WHERE id = 'SWIGGY50';
COMMIT;
```

**The problem with `FOR UPDATE` (Pessimistic):** It physically commands the database to hold a row lock. If thousands of users try to check out at 8:00 PM on Friday to use `SWIGGY50`, 9,999 of them are shoved into an internal database queue. This causes:
1. **Connection Pool Exhaustion:** Database connections are tied up waiting for locks.
2. **High Latencies:** The user sees "Processing..." indefinitely.
3. **Deadlocks:** If multiple resources are locked out of order across various services.

### Why Optimistic Locking is the Gold Standard

```sql
-- Optimistic Lock (DO THIS)
-- 1. Read current version and count (No DB Blocks!)
SELECT available_count, version FROM coupon WHERE id = 'SWIGGY50'; 
-- (Assume returns count=5000, version=100)

-- 2. Update ONLY if version hasn't changed
UPDATE coupon 
SET available_count = available_count - 1, version = version + 1 
WHERE id = 'SWIGGY50' AND version = 100;
```

**The benefits of Optimistic Locking:**
1. **No Physical Locks:** PostgreSQL's MVCC (Multi-Version Concurrency Control) naturally ensures that only ONE transaction can successfully run the `UPDATE` query where `version = 100`.
2. **Conflict Resolution:** If the `UPDATE` returns `RowsAffected == 0`, it means another thread beat us to it and incremented the version to 101. 
3. **High Throughput Application Retries:** Our Go application can intelligently catch the `0 rows updated` signal and cleanly retry step 1 and 2 in a `for` loop. This completely offloads the queuing/locking logic away from the Database layer and into the highly-scalable Application Server layer.

This makes Optimistic Locking vastly superior for high-velocity, concurrent financial operations like coupon redemption.

---

## 5. Architectural Edge Case: The "Location-Context Mismatch"

**Scenario:** A user creates a cart full of items in Delhi. They fly to Mumbai, open the app, and view their cart. The cart still exists, but the original Delhi restaurant obviously cannot deliver to Mumbai. How is this handled?

### A. The Validation Layer (Serviceability Check)
When the user opens the cart in Mumbai, the mobile app sends a `GET /cart` request. This request explicitly includes the user's **Current GPS Coordinates** or **Selected Delivery Address**.

Before returning the cart, the Cart Service performs a mandatory check:
1. It reads the cart from Redis (recognizing the Delhi `restaurant_id`).
2. It makes a fast RPC/gRPC call to the **Logistics/Restaurant Service**: *"Is `rest-delhi-123` deliverable to this Mumbai Lat-Lng?"*
3. The Logistics Service calculates the geofence boundary and returns `UNSERVICEABLE`.

### B. Graceful Degradation (UI Response)
Instead of aggressively destroying the user's cart (which destroys user intent—they might be trying to order food for their parents back in Delhi), the backend returns the cart data flagged with a specific status.

```json
{
  "cart_status": "UNSERVICEABLE_LOCATION",
  "restaurant": {
     "id": "rest-delhi-123",
     "name": "Bikanervala"
  },
  "items": [...],
  "message": "This restaurant doesn't deliver to your current location."
}
```

**User Experience:** 
The cart renders, but the checkout button is disabled. The app shows a banner: *"Restaurant doesn't deliver to your current location. [Clear Cart] or [Change Address]"*. If the user changes the address back to their Delhi home, the cart instantly becomes serviceable again.

### C. Compute Optimization (Short-Circuiting)
In relation to the Coupon System, if the Cart Service detects an `UNSERVICEABLE_LOCATION`, it **short-circuits** the flow. It deliberately skips calling the Eligibility Engine. There is no point in calculating complex coupon limits and placing load on Redis and Postgres if the user isn't physically permitted to check out.
