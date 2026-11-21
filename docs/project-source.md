# Project 2: High-Throughput Analytics & Edge Gateway (Go)

Faithful copy of Nick's Google Doc (2026-11-20):
https://docs.google.com/document/d/1mpALejYepx9INw9M1RIKUb9k4nKTDD4J_7-5_p1loPE/edit

Setup flow applied: "Setting Up the Multi-Agent Build Flow From Scratch"
https://docs.google.com/document/d/1NYNXmDEVz6aKmw1ebZEottdY3yZROLrAKQ14aC5lO-U/edit

---

Technical specification document for the implementation of a zero-allocation, high-throughput distributed systems primitive engine.
Overview
Goal: Build a high-throughput event ingestion edge gateway and real-time OLAP analytics engine (Mini-ClickHouse + Envoy) in Go from scratch, structured as a readable commit history where each commit introduces a CPU-, memory-, or routing-focused distributed systems primitive.
Language & Runtime: Go (latest stable release; emphasize zero-allocation hot paths, cache-line padding, bit manipulation, and Go profiling via pprof).
Primitives Covered (7 of 20):
Lock-Free Ring Buffers
Token Bucket / Leaky Bucket (and GCRA)
Consistent Hashing (with Bounded Loads)
Count-Min Sketch
Inverted Indexes & Roaring Bitmaps
Custom Cache-Friendly Hash Maps (SwissTable)
HyperLogLog (HLL)
Go Engineering & Testing Guidelines
Implement all 7 core data structures and algorithms from scratch without relying on external libraries (e.g., avoid importing roaring or hashbrown wrappers).
Optimize for Go mechanical sympathy: minimize heap allocations on hot paths (verify with go test -bench . -benchmem), use cache-line padding ([64]byte) to prevent false sharing, and leverage sync/atomic.
Provide profiling hooks (net/http/pprof) and benchmark comparisons showing allocation counts, throughput (ops/sec), and p99 latency before and after each optimization commit.
Suggested Go Repository Structure
internal/sim/: First-class deterministic simulation harness (seeded PRNG, virtual nanosecond Clock, Zipfian/burst traffic generator, and differential oracle runner)
cmd/gateway/: Ingestion edge proxy and rate-limiting router entrypoint
cmd/analytics/: Sharded OLAP query worker node entrypoint
internal/ringbuf/: Lock-free SPSC/MPMC ring buffer with cache-line padding
internal/ratelimit/: Token Bucket and GCRA rate limiters
internal/router/: Consistent Hashing ring with Bounded Loads
internal/sketch/: Count-Min Sketch heavy-hitter / hot-key detector
internal/index/: Inverted Index and Roaring Bitmap container implementation
internal/hashmap/: Open-addressed SwissTable-style cache-friendly hash map
internal/hll/: HyperLogLog cardinality estimator with register merging
Commit-by-Commit Roadmap
Commit 0: feat(sim): build first-class deterministic simulation, virtual clock, and workload fuzzing harness
Primitive: Deterministic Simulation Testing (DST) & Workload Generation Infrastructure
Scope: Build the core testing primitives before any gateway or analytics code: a single-seed PRNG (*math/rand.Rand), an injectable nanosecond virtual Clock for deterministic rate-limiting tests without time.Sleep, a seeded traffic generator producing Zipfian (hot-key), bursty retry-storm, and high-cardinality workloads, and a differential testing harness.
Verification: Meta-test verifying identical seeded workload streams, deterministic clock advancement, and zero-flakiness replay.
Commit 1: feat(core): add thread-per-core Lock-Free Ring Buffers
Primitive: Lock-Free Ring Buffers (Disruptor-style)
Scope: Implement a fixed-size, power-of-two circular buffer using sync/atomic sequence counters and cache-line padding to pass event batches between producer and worker goroutines without mutexes or channel lock contention.
Verification: Throughput and tail-latency benchmarks comparing the lock-free ring buffer against Go buffered channels under high contention, plus go test -race validation.
Commit 2: feat(edge): add Token Bucket / GCRA rate limiter
Primitive: Token Bucket / Leaky Bucket (and GCRA)
Scope: Implement both a classic Token Bucket and a Generic Cell Rate Algorithm (GCRA) rate limiter at the ingestion gateway to enforce per-tenant burst and sustained rate limits.
Verification: Time-simulated unit tests (injectable clock) validating burst tolerance, smooth pacing, and overload shedding during simulated retry storms.
Commit 3: feat(routing): route ingestion streams with Consistent Hashing
Primitive: Consistent Hashing (Ring & Bounded Loads)
Scope: Route incoming tenant/key event streams across worker shards using a virtual-node Consistent Hashing ring with Bounded Loads so adding or removing shards minimizes key movement while preventing shard overload.
Verification: Distribution tests measuring key movement ratio (~K/N) on node join/leave and load balance variance under skewed traffic.
Commit 4: feat(protection): detect hot keys in real time with Count-Min Sketch
Primitive: Count-Min Sketch
Scope: Build a fixed-memory 2D counter matrix with independent hash functions to estimate key frequencies in real time and flag heavy hitters (hot keys) for automatic isolation or adaptive rate limiting.
Verification: Zipfian traffic distribution tests verifying top-K heavy-hitter recall and bounded memory footprint.
Commit 5: feat(index): build Inverted Index compressed with Roaring Bitmaps
Primitive: Inverted Indexes & Roaring Bitmaps
Scope: Index incoming event tags/attributes into an Inverted Index mapping attribute values to sorted document IDs, compressed into Roaring Bitmap 16-bit chunks (array containers for sparse chunks, bitset containers for dense chunks) supporting fast AND/OR intersections.
Verification: Query correctness and memory/latency benchmarks comparing Roaring Bitmap intersections against naive sorted slices and Go maps.
Commit 6: perf(agg): replace stdlib map with cache-friendly SwissTable Hash Map
Primitive: Custom Cache-Friendly Hash Maps (SwissTable)
Scope: Implement an open-addressed SwissTable-inspired hash map using flat contiguous storage, 7-bit H2 metadata control bytes, and group probing (16-slot groups using bitmasks/SWAR in pure Go) to accelerate high-cardinality GROUP BY aggregations.
Verification: Benchmarks comparing Go's built-in map against the custom hash map on high-cardinality aggregations (millions of distinct keys), measuring CPU cache efficiency and GC pressure.
Commit 7: feat(agg): add distributed APPROX_COUNT_DISTINCT via HyperLogLog
Primitive: HyperLogLog (HLL)
Scope: Implement a HyperLogLog sketch (14-bit precision, bias correction for small/large ranges) on each analytics shard and a coordinator merge step combining shard registers via element-wise MAX to answer distributed distinct-count queries.
Verification: Accuracy tests across 10k to 10M unique elements verifying ~0.8% standard error and exact equivalence between single-node HLL and merged multi-shard HLL.

First-Class Deterministic Simulation, Property Tests & Fuzzing
Core DST & Fuzzing Architecture
Deterministic Time & Traffic: Eliminate real-time wall-clock flakiness in rate limiters and hot-key detectors by driving all time and traffic distributions from seeded virtual primitives.
Differential Oracles & Go Native Fuzzing (testing.F): Verify complex low-level structures (Roaring Bitmaps, SwissTable) by fuzzing identical operation sequences against simple, provably-correct standard library maps/slices.
Allocation & Concurrency Gates: Treat zero-allocation hot paths (testing.AllocsPerRun) and race-detector cleanliness (go test -race) as hard invariants inside automated property tests.
Component-Specific Simulation, Property & Fuzz Suites
Lock-Free Ring Buffer Concurrency & Wrap-Around Property Tests:
Invariants: Assert zero lost items, zero duplicates, strict FIFO ordering per producer, and zero heap allocations on enqueue/dequeue hot paths.
Fuzzing: Fuzz buffer sizes (powers of two), producer/consumer batch sizes, backpressure stalls, and uint64 sequence-number wrap-around boundaries under go test -race.
Token Bucket & GCRA Rate Limiter Property Tests (FuzzRateLimiter):
Invariants: Over any virtual time interval [t0, t1], total admitted requests never exceed burst + rate * (t1 - t0). Assert GCRA Theoretical Arrival Time (TAT) never drifts backward and full burst capacity recovers cleanly after idle periods.
Traffic DST: Replay simulated retry storms and micro-bursts against the virtual clock to verify bounded downstream queue depth.
Consistent Hashing with Bounded Loads Simulation:
Minimal Disruption Property: Adding or removing 1 of N shards remaps approximately 1/N of keys within statistical bounds across fuzzed keyspaces and shard topologies.
Bounded Load Invariant: Under heavily skewed Zipfian traffic, assert no shard's assigned load ever exceeds ceil((1 + epsilon) * averageLoad).
Inverted Index & Roaring Bitmap Differential Fuzzing (FuzzRoaringOps):
Oracle Verification: Fuzz random sequences of Add, Remove, And, Or, and AndNot operations across sparse and dense 16-bit chunk boundaries (0, 4095, 4096, 65535, 65536, MaxUint32), asserting exact equality against a map[uint32]struct{} oracle and verifying Array-to-Bitset container promotion/demotion.
SwissTable Custom Hash Map Differential Fuzzing (FuzzSwissTable):
Oracle Verification: Drive fuzzed Put, Get, Delete, and table-resize sequences—including adversarial keys engineered to force 7-bit H2 metadata collisions and heavy tombstone churn—against Go's built-in map, asserting identical lookups, lengths, and iteration sets.
Count-Min Sketch & HyperLogLog Statistical Property Tests:
Count-Min Sketch Invariant: Assert the never-underestimates guarantee (estimate(k) >= trueCount(k) for all keys) and verify top-K heavy-hitter recall on seeded Zipfian streams.
HyperLogLog Algebraic Properties: Fuzz arbitrary shard partitions and assert Merge commutativity (Merge(A, B) == Merge(B, A)), associativity, and exact register equivalence between a single-node HLL and merged multi-shard HLLs, while validating ~0.8% standard error across 10k–10M distinct elements.
