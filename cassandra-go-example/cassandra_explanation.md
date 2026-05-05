# Apache Cassandra Overview

Apache Cassandra is a highly scalable, high-performance distributed NoSQL database designed to handle large amounts of data across many commodity servers, providing high availability with no single point of failure. It is a type of Wide Column Store.

Originally developed at Facebook to power their Inbox Search feature, it was later open-sourced and became a top-level Apache project. It is heavily influenced by Amazon's Dynamo (for its distributed architecture) and Google's Bigtable (for its data model).

## Key Characteristics

### 1. Distributed and Decentralized
Unlike master-slave architectures, Cassandra uses a **masterless architecture**. Every node in the cluster is identical and can perform the exact same functions. This means there is no single point of failure (SPOF) and no bottleneck. Data is distributed across the nodes in a "ring" architecture.

### 2. High Availability and Fault Tolerance
Data is automatically replicated to multiple nodes according to a Replication Factor (RF). Even if an entire data center goes down, your application can continue to run seamlessly if data is replicated across multiple data centers. Failed nodes can be replaced with no downtime.

### 3. High Scalability
Scaling is horizontal (scale-out). If you need more capacity or performance, you simply add more nodes to the cluster. Performance scales linearly.

### 4. Tunable Consistency
With distributed databases, there is a trade-off between Consistency, Availability, and Partition Tolerance (CAP Theorem). Cassandra provides **tunable consistency**, meaning you can choose on a per-query basis whether you want strong consistency (slower, requires more nodes to respond) or high availability/eventual consistency (faster, requires fewer nodes to respond). You set a Consistency Level (CL) like `ONE`, `QUORUM`, or `ALL`.

## The Data Model

Cassandra is a partitioned row store, not a relational database. It is schema-based, so you must define tables before you insert data.

- **Keyspace**: The outermost container for data in Cassandra. It is analogous to a database in an RDBMS. It contains configuration like the Replication Strategy and Replication Factor.
- **Table**: Similar to tables in relational databases, but columns can be added flexibly.
- **Primary Key**: Used to identify a row uniquely. It has two parts:
  - **Partition Key**: Determines which node stores the data. Crucial for data distribution.
  - **Clustering Key**: Determines how data is sorted within the partition on the node.

### Example Schema
```cql
CREATE TABLE users_by_city (
    city text,
    last_name text,
    first_name text,
    user_id uuid,
    PRIMARY KEY (city, last_name, first_name)
);
```
* `city` is the partition key (users in the same city are stored on the same node).
* `last_name` and `first_name` are clustering keys (users within a city are sorted by last name, then first name).

## Internal Architecture & Write Path

Cassandra is extremely optimized for writes. Here is how a write happens:

1. **Commit Log**: Data is first appended to a durable Commit Log on disk to prevent data loss in case of a crash.
2. **Memtable**: Next, data is written to a Memtable (an in-memory data structure).
3. **SSTable**: When the Memtable fills up, it is flushed to disk into an SSTable (Sorted String Table). SSTables are immutable.

Because SSTables are immutable, updates and deletes don't modify existing data in place. Instead:
- **Updates** are just new inserts with a newer timestamp.
- **Deletes** write a "Tombstone" (a marker indicating the data is deleted) with a timestamp.

During a read, Cassandra merges data from the Memtable and multiple SSTables to give the final correct result. Periodically, a background process called **Compaction** merges SSTables and removes data marked with tombstones to reclaim disk space.

## Query Language (CQL)

Cassandra uses Cassandra Query Language (CQL), which looks very similar to SQL, making it relatively easy to learn for anyone familiar with relational databases.

However, **there are no joins or complex subqueries in Cassandra**. Data modeling in Cassandra is query-driven: you must design your tables around the specific queries your application will perform, which often means denormalizing data (storing the same data in multiple tables).

## When to Use Cassandra
- Time-series data (e.g., IoT sensor data, metrics).
- High volume write-heavy workloads.
- Recommendation engines, user activity tracking.
- Applications needing multi-datacenter replication and extreme fault tolerance.

## When NOT to Use Cassandra
- Applications requiring complex ACID transactions (it only supports row-level atomicity).
- Heavy use of `JOIN`s or ad-hoc analytics queries.
- Small datasets where a traditional RDBMS like PostgreSQL would suffice.
