# PLAN — High-Throughput Analytics & Edge Gateway (Go)

Status: Phase 1 deliverable, awaiting Nick's sign-off. Nothing in Phase 2 starts before that.
Spec: [docs/project-source.md](docs/project-source.md). State: [STATE.md](STATE.md).
This is an operational document (orchestrator-authored). Reader-facing prose is Neon's.

Contents: 1 North Star · 2 Testing architecture · 3 Repo layout · 4 Chunk plan ·
5 Concurrency map · 6 Per-chunk pipeline · 7 Key technical decisions ·
8 Spec coverage map · 9 Open questions for Nick

---

## 1. North Star

A from-scratch Go platform in two processes — an ingestion edge gateway (rate limiting,
hot-key detection, bounded-load routing, lock-free hand-off) and a sharded real-time OLAP
engine (Roaring-backed inverted index, SwissTable GROUP BY, HyperLogLog distinct counts) —
built from seven hand-written primitives with no external data-structure libraries. What a
reader or future builder gets is a codebase where **every performance and correctness claim
is proven, not asserted**: throughput, allocations per operation, p99 latency, load bounds,
key-movement ratios and sketch accuracy each trace to a named test, benchmark or seeded
simulation in the repo, an adversarial harness that tried to break it, and an architecture
write-up that states the benefit, the trade-off, and any claim that had to be narrowed. The
commit history on `main` reads as a guided tour: one landing per primitive, each message
naming what its tests prove.

---

## 2. Testing architecture (governing section)

Tests are designed here, before any chunk. Every chunk brief is written **from** this
section: it lists the named tests that define the chunk, and the code is not done until
they are green. Three ideas run through everything:

1. **Sans-IO primitives.** No primitive reads a clock, an OS random source, or a socket.
   Time enters as an `int64` nanosecond argument; randomness as an explicit `*rand.Rand` or
   seed. That is what makes every later test deterministic.
2. **One op decoder, three drivers.** Each stateful primitive defines one function that
   decodes a byte string into an operation sequence. The same decoder is driven by
   (a) property tests (bytes drawn from the seeded PRNG), (b) native fuzzing (bytes from
   the fuzz engine), and (c) regression tests (bytes from `testdata/`). A failure found by
   any driver replays in the other two.
3. **A model for every structure.** Each primitive has a trivially-correct reference model
   built from stdlib maps/slices/`math/big`; the differential runner compares them
   operation by operation.

### 2.1 The DST / workload harness (`internal/sim/`, spec Commit 0)

**Seed discipline.**
- One `int64` root seed per test. Source of the seed, in priority order: the
  `GATEWAY_SEED` environment variable; the `-sim.seed` flag (registered only in test
  binaries that import `internal/sim`, so `go test ./...` never chokes on an unknown flag —
  the env var is the form that works repo-wide); otherwise the test's pinned seed list.
- Default runs use **pinned seeds only** (zero flakiness). Exploration is opt-in:
  `GATEWAY_SEEDS=N` sweeps N fresh seeds derived from the root seed.
- `sim.New(t)` returns a `*sim.Env` holding the root `*math/rand.Rand`
  (`rand.New(rand.NewSource(seed))`, as the spec names), the virtual clock, and the event
  queue. **Named forked streams:** `env.Fork("traffic")`, `env.Fork("faults")` derive child
  `*rand.Rand`s from `mix(seed, label)`, so adding a draw in one component does not shift
  every other component's stream (this is what keeps shrinking and golden digests stable).
- **Replay line.** Every failing simulation/property test prints exactly:
  `REPLAY: GATEWAY_SEED=<seed> go test ./<pkg> -run '^<TestName>$' -count=1`
  via `t.Cleanup` when `t.Failed()`. The seed also goes into the chunk's work notes
  (`agent_work/<chunk>/seeds.txt`) with the failure and the fixing commit.
- Probe evidence (`agent_work/probes/sim-determinism`): 1M mixed draws
  (clock steps, Zipf keys, burst decisions) hash identically on same-seed replay.

**Virtual clock.**
- `type Clock interface { Now() int64 }` — nanoseconds, `int64`, no `time.Time` on hot paths.
- `sim.VirtualClock`: starts at a seed-independent epoch; moves **only** on `Advance(d)` /
  `AdvanceTo(t)` or when the event loop jumps to the next event; never goes backward
  (attempt returns an error in the harness, tested).
- `sim.RealClock` (monotonic) is the only place `time.Now` is allowed, and only `cmd/` uses it.
- Primitives take `now int64` as a parameter instead of holding a clock
  (`limiter.Allow(now, n)`), so rate-limit tests need no `time.Sleep` and no clock mock.

**Event loop.** A priority queue keyed `(timeNs, seq)` with FIFO tie-break; the clock jumps
to the next event. Closed-loop behaviour (a rejected client schedules its own retry) is
expressed as events, which is what makes a retry *storm* simulable rather than a fixed trace.

**Traffic generators** (all draw from a forked stream; all implement
`Next(*Event) bool` with zero allocations per event; `Event{TimeNs, Tenant, Key, Value, Tags}`):

| Generator | Shape | Knobs |
|---|---|---|
| `Zipf` | hot-key skew via stdlib `rand.NewZipf` (requires s > 1 — probe-verified that s = 1.0 returns nil) | s, v, key-space size, tenants |
| `Burst` / `RetryStorm` | on/off modulated arrivals; closed-loop clients that retry rejected requests after a backoff with optional synchronised (jitter-free) retries; micro-bursts of N events in one virtual nanosecond | base rate, burst factor, on/off durations, retry policy, client count |
| `HighCardinality` | keys from a bijective mix of a counter, so the exact distinct count is known without storing a set (needed for 10M-key HLL and GROUP BY truth) | distinct count, repeat factor |
| `Mix` | weighted interleave of the above by virtual time | weights |

**Differential oracle runner.** `sim.Diff(t, ops, subject, model)` applies each decoded op
to both, compares observable results after every op (and full state at checkpoints), and on
divergence: (1) minimises the op list with delta debugging (ddmin — remove chunks while the
failure reproduces), (2) prints the minimised op list in Go-literal form, the seed, and the
REPLAY line, (3) writes the minimised input under `testdata/` naming so it can be committed
as a regression. Stdlib only; ~300 lines.

**Determinism rules the code must follow** (enforced by `agent_work/gates/detlint.sh`, a
grep/`go vet`-style gate over `internal/`): no `time.Now`/`time.Sleep`/`time.After`, no
package-level `math/rand` functions, no `crypto/rand`, no map iteration feeding an
observable result, no goroutines inside sans-IO packages (ring buffer and `cmd/` excepted).

**Honest scope of the determinism claim** (decided now so it is not narrowed later):
replay is guaranteed for the same commit, Go toolchain, and GOOS/GOARCH. Golden digests are
pinned for linux/amd64. Other platforms assert run-twice equality only, because stdlib Zipf
and estimator float maths may differ across architectures (FMA fusion on arm64).

**Meta-tests (land with the harness; gate every later chunk that uses it):**

| Test | Proves |
|---|---|
| `TestReplayIdenticalStreams` | each generator, run twice with one seed, yields byte-identical event streams (SHA-256 digest); different seeds differ |
| `TestGoldenDigests` | pinned seeds → pinned digests (linux/amd64); an accidental change to draw order is visible in review |
| `TestForkIsolation` | adding draws on stream A leaves stream B's output unchanged |
| `TestClockAdvancesOnlyExplicitly` | clock is constant across arbitrary work; monotone; jump-to-next-event order is `(time, seq)` |
| `TestEventLoopDeterministic` | a closed-loop retry scenario produces an identical trace digest twice |
| `TestDiffCatchesPlantedBug` | "testing the tester": a model with a planted off-by-one is caught and shrunk to ≤ 4 ops |
| `TestReplayLinePrinted` | a forced failure prints the exact REPLAY line (subprocess test) |
| `TestZeroFlake` (gate script) | the whole `internal/sim` suite passes `-count=50` and `-race -count=10` |
| `TestGeneratorsZeroAlloc` | `Next` allocates 0 (`testing.AllocsPerRun`) |
| `TestZipfShape`, `TestBurstShape`, `TestHighCardinalityExactDistinct` | the generators actually produce the distribution they claim (rank-frequency slope, burst peak/mean ratio, exact distinct count) |

### 2.2 Property tests — invariants per component

Generator design is the same everywhere: a seeded stream produces a byte string; the
component's op decoder turns it into operations biased toward its boundaries. Failures
shrink by ddmin over the op list (stateful structures) or replay by seed (statistical and
concurrent properties, where shrinking is not meaningful). "Exact" means a deterministic
assertion; "statistical" means a bound asserted over pinned seeds with the tolerance
derived in the test's comment and the measured value recorded as evidence.

**Ring buffer (`internal/ringbuf`)**
| Invariant | Kind | How |
|---|---|---|
| Zero lost, zero duplicated items | exact | items tagged `producerID<<40 \| seq`; consumers' multiset equals producers' |
| Strict FIFO per producer | exact | per-producer sequence strictly increasing at every consumer (SPSC: globally) |
| Zero allocations on enqueue/dequeue, single and batch | exact | `testing.AllocsPerRun` == 0 |
| Sequence wrap-around | exact | test-only constructor starts cursors at `MaxUint64 − k`; full workload crosses the wrap |
| Full/empty are reported, never corrupt | exact | `TryEnqueue` on full → false and no state change; same for empty |
| Batch ≡ repeated single ops | differential | vs a `[]T` FIFO model on single-goroutine op sequences (shrinks by ddmin) |
| Backpressure stalls lose nothing | exact, under `-race` | consumer pauses of seeded length; producers spin/yield; totals reconcile |
Concurrent properties replay by seed for the *workload*; goroutine interleaving is not
seed-controlled, so these run with many iterations under `-race` and the claim is stated as
"no violation in N×M interleavings", not "deterministic".

**Rate limiter (`internal/ratelimit`)**
| Invariant | Kind | How |
|---|---|---|
| Over **any** virtual interval [t0,t1]: admitted ≤ burst + rate·(t1−t0) | exact | checked for every pair of admitted events against a `math/big` rational oracle (no float tolerance) |
| GCRA TAT never moves backward | exact | observed after every op, including with non-monotonic `now` inputs (clamped) |
| Full burst recovers after idle ≥ burst/rate | exact | idle then admit exactly `burst` at one instant, the next is rejected |
| Token Bucket ≡ GCRA decisions for equal (rate, burst), unit cost | differential | same op stream to both; any divergence is a bug or a documented rounding rule |
| No overflow at extreme rate/burst/now | exact | boundary generator: rates 1/hour … 10⁹/s, `now` near `MaxInt64` |
| Zero allocations per `Allow` | exact | `AllocsPerRun` |
| Retry storm → bounded downstream queue | DST | limiter feeding a queue drained at service rate μ ≥ rate: depth ≤ burst + c for the whole run, over pinned seeds |

**Consistent hashing (`internal/router`)**
| Invariant | Kind | How |
|---|---|---|
| Join: every remapped key maps to the new shard; leave: only the departed shard's keys move (plain ring) | exact | all keys, fuzzed topologies |
| Join/leave of 1 of N remaps ≈ 1/N of keys | statistical | 100k keys, N ∈ [2,64], pinned seeds; tolerance from virtual-node variance |
| Bounded load: at every assignment, chosen shard's load ≤ ⌈(1+ε)·average⌉ | exact | checked after every assignment under Zipf(s=1.1) and a single-key flood |
| Key movement with bounded loads enabled | measured | reported, not pre-claimed (theory: O(1/ε²) factor) |
| Lookup is deterministic and independent of insertion order of shards | exact | permuted construction → identical assignments |
| Zero allocations per lookup | exact | `AllocsPerRun` |
| Balance: peak/mean load vs virtual-node count | measured | table for V ∈ {16, 64, 160, 512} |

**Count-Min Sketch (`internal/sketch`)**
| Invariant | Kind | How |
|---|---|---|
| estimate(k) ≥ trueCount(k) for all keys | exact | vs `map[uint64]uint64` oracle, all keys, all pinned seeds (holds for any hash quality) |
| estimate(k) ≤ trueCount(k) + εN with probability ≥ 1−δ | statistical | fraction of violating keys ≤ δ across seeds |
| Heavy hitters: every key with true count ≥ φN is flagged | exact | follows from never-underestimates; tested on Zipf streams |
| Top-K recall on seeded Zipfian streams | statistical | recall@K vs oracle top-K, K ∈ {10, 100}; floor asserted, measured value recorded |
| Merge(A,B) ≡ sketch of concatenated stream | exact | cell-wise equality |
| Decay keeps the never-underestimates guarantee against the equally-decayed truth | exact | oracle applies the same decay |
| Bounded memory: footprint is a function of (w,d) only | exact | size constant before/after 10M adds; 0 allocs per `Add` |

**Roaring bitmaps + inverted index (`internal/index`)**
| Invariant | Kind | How |
|---|---|---|
| Add/Remove/Contains/And/Or/AndNot ≡ `map[uint32]struct{}` oracle | differential | op decoder biased to 0, 4095, 4096, 65535, 65536, MaxUint32 and to dense runs; ddmin shrinking |
| Array↔bitset promotion at 4096→4097, demotion at 4097→4096 | exact | container type asserted at the boundary, including via And/AndNot results |
| Structural invariants after every op | exact | test-only `validate()`: keys sorted/unique, no empty containers, arrays sorted ≤ 4096, cached cardinality == popcount |
| Algebra: And/Or commutative, associative, De Morgan via AndNot | exact | random triples |
| Iteration is sorted ascending and complete | exact | vs sorted oracle keys |
| Index query ≡ brute-force scan of documents | differential | random tag sets, random AND/OR/NOT query trees |
| Zero allocations: `Contains`, in-place ops within capacity, `AndCardinality` | exact | `AllocsPerRun` |

**SwissTable (`internal/hashmap`)**
| Invariant | Kind | How |
|---|---|---|
| Put/Get/Delete/Len/iteration-set ≡ built-in `map` | differential | op decoder; ddmin shrinking |
| Survives adversarial H2 collisions | differential | test-injected hasher lets the generator choose H1 and H2 bits directly (all keys one H2; all keys one group) |
| Tombstone churn does not grow the table unboundedly or lose keys | exact | insert/delete cycles at constant live size: capacity bounded, lookups correct |
| Resize preserves contents; growth-left accounting exact | exact | `validate()`: live + tombstones + empty == capacity, every key reachable by its probe sequence |
| Probe terminates at any load ≤ max | exact | full-table and all-tombstone tables |
| Zero allocations: Get, Delete, Put without growth | exact | `AllocsPerRun` |

**HyperLogLog (`internal/hll`)**
| Invariant | Kind | How |
|---|---|---|
| Merge commutative, associative, idempotent | exact | register-array equality |
| Any partition of a stream into shards, merged ≡ single-node HLL | exact | fuzzed partitions (1…64 shards, skewed splits), register-for-register |
| Standard error ≈ 0.81% (1.04/√16384) for 10k–10M distinct | statistical | n ∈ {10k, 30k, 100k, 1M, 10M}; RMSE over pinned seeds within tolerance; mean bias reported; the 40k–80k range (the classic estimator's weak spot) is sampled explicitly |
| Duplicates never change registers | exact | re-adding a stream is a no-op |
| Mismatched precision/seed merge is refused with a typed error | exact | `errors.As` |
| 16 KiB fixed footprint; 0 allocs per `Add` | exact | size + `AllocsPerRun` |

**Shared hashing (`internal/xhash`)**: official xxHash64 test vectors (unit); avalanche and
bucket-uniformity checks (statistical, measured); 0 allocs.

### 2.3 Fuzz targets (Go native, `testing.F`)

| Target | Package | Input → behaviour checked |
|---|---|---|
| `FuzzRateLimiter` *(spec)* | ratelimit | (config, [(Δt, n)…]) → interval bound vs big-rational oracle; TAT monotone; TB≡GCRA; no panic/overflow |
| `FuzzRoaringOps` *(spec)* | index | op bytes over 2–3 bitmaps → equality with map oracle + `validate()` |
| `FuzzSwissTable` *(spec)* | hashmap | op bytes + hasher mode (normal / forced-H2 / forced-group) → equality with built-in map + `validate()` |
| `FuzzRingBufferOps` | ringbuf | (capacity exponent, start sequence near wrap, batch sizes, stall pattern) → FIFO model equality single-threaded; a concurrent variant runs under `-race` |
| `FuzzRouterTopology` | router | (shard add/remove script, key bytes, ε) → exact join/leave properties, bounded-load invariant, no panic on empty ring |
| `FuzzHLLMerge` | hll | (elements, partition assignment) → merged ≡ single; algebraic laws |
| `FuzzCountMin` | sketch | (w, d, stream) → never-underestimates; merge equivalence |
| `FuzzZipfGenerator` / `FuzzEventLoop` | sim | arbitrary knobs → no panic, replay-identical |
| `FuzzWireFrame` | wire (chunk 8) | arbitrary bytes → decoder returns typed error or a frame that re-encodes identically; never panics, never over-allocates |
| `FuzzIngestLine`, `FuzzQueryRequest` | pipeline / engine (chunk 8) | arbitrary request bodies → 4xx or valid; no panic |

**Seed-corpus policy.** Boundary seeds are added in code with `f.Add` (so `go test` always
replays them). Every crasher the engine finds is committed under
`testdata/fuzz/<Target>/` **and** promoted to a named regression test carrying its input
before the chunk can land. The fuzz cache is never committed.

**Budgets.** Builder: 60 s per target before hand-off. Orchestrator landing gate: 3 min per
target (`-fuzztime=3m`), sequential, machine otherwise quiet. Final hardening (Chunk 9):
30 min per target, plus 2 h each for the three spec-named targets (≈ 11 h total, run
detached overnight with exit files). Any new crasher in hardening follows the same
regression rule and reopens the owning package for a fix.

### 2.4 Allocation and benchmark gates

**Allocation gates are tests, not observations.** Each hot path has a
`TestAllocs_<Op>` using `testing.AllocsPerRun(1000, …)` that must equal 0; the list of hot
paths per chunk is in §4. They are skipped under `-race` (instrumentation changes
allocation behaviour) via a `raceEnabled` build-tagged constant and so run in the non-race
pass. `-benchmem` output for every benchmark is saved to `agent_work/<chunk>/bench/`.

**Benchmark evidence each chunk must produce** (raw `go test -bench` output committed;
`-count=10`, compared with a small in-repo stats script — median and spread, no external
`benchstat` dependency unless Nick approves it, see §9):

| Chunk | Benchmarks | Reported |
|---|---|---|
| 1 ring buffer | SPSC and MPMC ring vs buffered `chan` at 1/2/4 producers×consumers, single and batch; padded vs unpadded layout (before/after) | ops/s, allocs/op, p50/p99 hand-off latency |
| 2 rate limiter | `Allow` for Token Bucket vs GCRA; 1 tenant and 1M tenants; contended | ns/op, allocs/op, bytes/tenant |
| 3 router | lookup vs shard count and vnode count; with/without bounded loads | ns/op, allocs/op, key-movement %, peak/mean load |
| 4 Count-Min | `Add`/`Estimate`; plain vs conservative update (before/after accuracy); vs exact `map` counter | ns/op, allocs/op, bytes, error distribution, recall@K |
| 5 Roaring | And/Or/AndNot vs sorted `[]uint32` merge and `map[uint32]struct{}` at densities 0.01%–50% | ns/op, allocs/op, bytes/element, p99 query latency |
| 6 SwissTable | GROUP BY aggregation over 1M and 10M distinct keys vs built-in `map`: insert, hit, miss, mixed, delete-churn | ns/op, allocs/op, bytes/entry, GC cycles and pause total (`runtime.MemStats`), p99 per-op |
| 7 HLL | `Add`, `Merge`, `Estimate`; vs exact `map` distinct | ns/op, allocs/op, 16 KiB vs exact-set bytes, error table |
| 8/9 integrated | end-to-end ingest throughput and query latency | events/s, p99 ingest and query latency, allocs/event on the post-parse path |

**p99 latency.** Go benchmarks do not report percentiles, so `internal/benchutil` provides
a fixed-bucket log-linear histogram (zero-alloc record) and benchmarks call
`b.ReportMetric(p99, "p99-ns")`. Latency is sampled 1-in-N to keep timer overhead out of
the throughput figure; both numbers come from separate runs where that matters.

**Honest measurement limits, stated up front.** The build machine is a 4-vCPU shared VM
with no hardware performance counters (probe: `perf stat` reports cycles/cache-misses as
unsupported). Therefore: "high contention" means ≤ 4 runnable goroutines per side; "CPU
cache efficiency" is evidenced by proxies (ns/op as tables outgrow L2, bytes/entry,
padded-vs-unpadded deltas), not miss counters; evidence benchmarks run serialised under a
machine-wide lock with builders idle. The repo ships the benchmark scripts so a reader can
re-run them on real hardware. A comparison that does not come out in our favour is
reported as measured and the write-up's claim is narrowed — notably Go's built-in `map`
has itself been a Swiss table since Go 1.24, so Chunk 6's claim is evidence-gated (§7.6).

**pprof.** Both entrypoints serve `net/http/pprof` on a separate `-debug-addr` listener
(loopback by default, never on the ingest/query mux). Chunk 8 has a test that the debug
endpoint answers `/debug/pprof/` and that the public mux does not.

### 2.5 Regular / unit testing conventions

- Table-driven (`tests := []struct{name string; …}`, `t.Run(tc.name, …)`); helpers call
  `t.Helper()`; no assertion libraries (stdlib `testing` only).
- Every exported constructor and every function returning `error` has an error-case table.
- **"Enough" per package:** each exported function is exercised on its happy path, each
  documented error, and each boundary named in §2.2; statement coverage is reported per
  chunk (`go test -cover`) with a floor of 85% for primitive packages — reviewed, not
  worshipped: the reviewer checks that uncovered lines are unreachable-by-design.
- **Proportionality.** Pure helpers (bit tricks, hash mixers, config validation, wire
  encode/decode of fixed fields) get unit tables only. Property tests are reserved for the
  invariants in §2.2. A chunk brief that asks for a property test must name the invariant.

### 2.6 Concurrency validation

- `go test -race ./...` is a hard landing gate. Concurrent tests also run `-race -count=5`
  and with `-cpu 1,2,4`.
- **Goroutine ownership rules** (reviewed against the code, proven by tests):
  1. Library packages start no goroutines. The ring buffer is passive: producers and
     consumers are the caller's goroutines. SPSC variants document and (in a debug build
     tag) assert single-producer/single-consumer use.
  2. Every long-lived goroutine is started by exactly one owner object in
     `internal/pipeline`, `internal/engine` or `cmd/`, via a constructor that takes a
     `context.Context`; the owner exposes `Close()`/`Wait()` that returns only after the
     goroutine has exited.
  3. Thread-per-core shape: one worker goroutine per shard ring, the only consumer of that
     ring and the only writer of that shard's state (single-writer principle; cross-shard
     reads go through snapshots or merge requests on the ring, never shared mutable state).
  4. Shutdown is signalled by context cancellation, then: stop accepting → producers stop →
     rings drain → workers flush and exit → listeners close. Drain has a deadline; items
     dropped at the deadline are counted and reported, never silent.
  5. Blocking strategy on empty/full is explicit: bounded spin → `runtime.Gosched` → park
     on a notification channel; no unbounded busy-wait.
- **Tests:** leak check in `TestMain` of every package with goroutines (goroutine count
  and stack-dump comparison before/after, stdlib `runtime` — no external leak detector);
  context-cancellation tests for every long-lived component (cancel mid-load → `Wait`
  returns within deadline, no leak, counts reconcile: accepted = delivered + reported
  dropped); `testing/synctest` for timer-driven lifecycle code so those tests need no
  real sleeps; double-`Close` and use-after-`Close` return a sentinel error, not a panic.

### 2.7 Go error-handling paradigms, per layer

| Layer | Rule | Tests assert |
|---|---|---|
| Primitive hot paths (`TryEnqueue`, `Allow`, `Add`, `Get`, `Contains`, `Lookup`) | No `error` return: `bool` / `(v, ok)` / small value results (e.g. `Allow` → `(ok bool, retryAfterNs int64)`), so the hot path cannot allocate or wrap | return values at each boundary; 0 allocs |
| Primitive constructors and cold paths | Package-level **sentinels** (`ErrCapacityNotPowerOfTwo`, `ErrInvalidRate`, `ErrEmptyRing`, `ErrClosed`, …) matched with `errors.Is` | error table per constructor: `errors.Is(err, ErrX)` for every documented failure |
| Where the caller must branch on data | **Typed errors** with fields, matched with `errors.As`: `*hll.MismatchError{Field, Want, Got}`, `*wire.FrameError{Offset, Reason}`, `*router.OverloadError{Shard, Load, Cap}`, `*engine.QueryError{Code, …}` | `errors.As` + field values |
| `internal/pipeline`, `internal/engine`, `internal/wire` | Wrap with `%w` and context naming the operation and identity (`fmt.Errorf("shard %d: decode batch: %w", id, err)`); never wrap twice with the same context; never return a bare error across a package boundary without context | chain tests: the outermost error still satisfies `errors.Is`/`As` for the innermost sentinel/type; message contains the context |
| `cmd/*` process boundary | Errors map to exit codes and HTTP statuses in one place via `errors.Is/As`: 400 malformed, 429 + `Retry-After` rate-limited, 503 backpressure/shard full, 500 internal. Each supervised goroutine has a `recover` that logs the stack, increments a counter and triggers orderly shutdown with non-zero exit (crash-only; nothing is swallowed) | status-mapping table tests; a test-only panic injection proves recover → shutdown → non-zero exit |
| **Panics** | Forbidden in library packages for any input. Allowed only for true invariant violations (corrupted internal state), behind `validate()`/debug assertions | every fuzz target doubles as a no-panic test; constructors with invalid arguments return errors, tested |
| **Retry** | **Forbidden** inside primitives, sketches, the simulator and the oracle runner (a CAS loop is algorithmic progress, not error retry; it is bounded by lock-freedom and covered by tests). **Allowed** in exactly one place: the gateway→shard forwarder re-dial, with capped exponential backoff, jitter from an injected PRNG, bounded attempts, a metric per attempt, and counted shedding when exhausted. The gateway never retries on a client's behalf | DST test: forwarder under injected connection failures makes exactly the scheduled attempts on the virtual clock; a detlint rule rejects retry loops elsewhere; sim tests fail if a result differs between two runs (so a hidden retry cannot mask non-determinism) |

### 2.8 E2E test design (built in Chunk 9, designed now)

Placement: `test/e2e/`, build tag `e2e`, run by `agent_work/gates/e2e.sh`. `TestMain`
builds `cmd/gateway` and `cmd/analytics` with `go build`, starts **real processes** (1
gateway + N analytics shards) on ephemeral ports, waits for readiness, and tears down with
SIGTERM (asserting clean exit code and drain counts). Workloads come from `internal/sim`
generators with a pinned seed, sent over real sockets. Expected answers come from an
in-test oracle (stdlib maps/sorts over the same generated events). No package of the system
is mocked. Because real processes use the wall clock, E2E assertions on time-dependent
behaviour are **bounds**; exact conformance for the same scenarios is asserted in the
in-process whole-pipeline DST (Chunk 8C), which uses the virtual clock.

| # | Scenario | Setup | Workload / faults | Passing assertion |
|---|---|---|---|---|
| E1 | Ingest → route → query round trip | gateway + 4 shards, limits high | 1M Zipf(s=1.1) events, seed pinned | after drain: `COUNT(*)` == events accepted (gateway's accepted counter == sum of shard row counts); per-key `SUM`/`COUNT` for the top 1,000 keys and 1,000 random keys == oracle exactly |
| E2 | Per-tenant rate limiting | 3 tenants with different (rate, burst) | each tenant offered 5× its rate for 10 s | per tenant: accepted ≤ burst + rate·elapsed (wall, measured outside); accepted ≥ 90% of that bound; every rejection is HTTP 429 with a `Retry-After`; other tenants' acceptance unaffected (± tolerance) |
| E3 | Retry-storm shedding | 1 tenant limit; shards slowed by a `-debug-slow-shard` flag | closed-loop clients retrying synchronously on 429/503 | max ring/queue depth reported by `/metrics` never exceeds capacity; shard receives ≤ burst + rate·elapsed; gateway RSS stays under a fixed ceiling; system recovers to normal acceptance within T after the storm stops |
| E4 | Hot-key detection and isolation | hot-key threshold φ configured | Zipf stream plus one injected key at 30% of traffic | the injected key appears in `/hotkeys` within W seconds; after isolation its accepted rate ≤ the hot-key limit; p99 ingest latency of other keys within X% of the no-hot-key baseline run |
| E5 | High-cardinality GROUP BY | 4 shards | 5M events over 2M distinct keys (HighCardinality generator) | `GROUP BY key` returns exactly 2M groups; every group's COUNT and SUM == oracle (full comparison, streamed) |
| E6 | Distributed APPROX_COUNT_DISTINCT | (a) 4 shards, (b) 1 shard, same events | 3M events, 1M distinct users | coordinator's merged registers from (a) are byte-identical to the single shard's registers in (b); estimate within 3σ (≈ 2.4%) of 1M |
| E7 | Roaring-backed tag queries | 4 shards | events with seeded tag sets (10 tags, mixed selectivity) | for 200 seeded boolean queries (AND/OR/NOT): returned row count and a checksum of matching row ids == brute-force oracle |
| E8 | Shard join/leave | 4 → 5 → 4 shards during ingest | steady Zipf stream | fraction of keys whose owning shard changed is within the Chunk 3 measured band; no shard's in-flight load exceeds ⌈(1+ε)·avg⌉ as reported per assignment; zero events lost (accepted == stored) |
| E9 | Graceful shutdown and errors | — | SIGTERM under load; malformed bodies; oversized frames; unknown tenant | exit code 0; accepted == stored + reported-dropped; malformed → 400 with a typed error body; process survives every bad input |
| E10 | pprof live | both binaries | — | `/debug/pprof/heap` answers on the debug address and 404s on the public address |

### 2.9 Landing gates per chunk (ordered; all green before merge)

Run by the orchestrator with `agent_work/gates/gate.sh <chunk>`; output saved to
`agent_work/<chunk>/gate-results.txt`.

1. `gofmt -l` empty; `go vet ./...`; `go build ./...`.
2. Cross-build: `GOOS=darwin GOARCH=arm64` and `amd64` — `go build ./...`, `go vet ./...`,
   and test-binary compile (`go test -run '^$' -exec /bin/true ./...`).
3. Determinism lint (`detlint.sh`) and dependency check (`go.mod` has no `require` beyond
   what Nick approved).
4. Unit + property tests: `go test -count=1 ./...`.
5. Allocation gates: `go test -run 'TestAllocs_' ./...` (non-race).
6. Race: `go test -race -count=1 ./...`; concurrent packages `-race -count=5 -cpu 1,2,4`.
7. Fuzz-short: every fuzz target in the chunk's packages, 3 min each; no new crashers;
   every earlier crasher present as a regression test.
8. DST seeds (chunks that use the simulator): pinned seed list plus a 200-seed sweep; any
   failing seed recorded and fixed, never skipped.
9. Sim meta-tests still green (replay digests unchanged, or changed with a reviewed reason).
10. Benchmark evidence recorded (`agent_work/<chunk>/bench/`, serialised run) and summarised.
11. Adversarial suite run by the orchestrator: held/broken counts recorded; every BROKEN is
    either repaired and re-run HELD, or the claim is explicitly narrowed in the evidence pack.
12. Neon review verdict PASS (or NITS with nits resolved or recorded).
13. Neon prose pass done: comments + write-up + knowledge entry, authored by Neon in git;
    the **comment-only check** proves Neon's edits to `.go` files changed no code (AST with
    comments stripped is identical before/after); gates 1–6 re-run after the prose commit.
14. Prose provenance: `git blame` over reader-facing files and Go comment lines shows only
    the Neon author; self-containment grep (no `/root/`, briefs, host names) is clean.
15. Fable acceptance note written (`agent_work/<chunk>/acceptance.md`): claims ↔ evidence table.

After merge: push, Pages deploy verified by fetching the write-up URL (HTTP 200 and a
content marker; a 404 is not "published"), CI run green, STATE.md, both kb stores and
`agent_work/INDEX.md` updated.

---

## 3. Repo layout

| Path | Purpose |
|---|---|
| `cmd/gateway/` | Ingestion edge proxy + rate-limiting router entrypoint; also the query coordinator (see §9 Q6) |
| `cmd/analytics/` | Sharded OLAP query worker node entrypoint |
| `internal/sim/` | Deterministic simulation harness: seeded PRNG streams, virtual clock, event loop, traffic generators, differential oracle runner with ddmin |
| `internal/ringbuf/` | Lock-free SPSC and MPMC ring buffers with cache-line padding |
| `internal/ratelimit/` | Token Bucket and GCRA limiters, per-tenant state table |
| `internal/router/` | Consistent-hash ring with virtual nodes and bounded loads |
| `internal/sketch/` | Count-Min Sketch and top-K hot-key detector |
| `internal/index/` | Roaring bitmap containers and the inverted index |
| `internal/hashmap/` | SwissTable-style open-addressed hash map |
| `internal/hll/` | HyperLogLog (p=14) with register merge |
| `internal/xhash/` | *(addition)* deterministic, seedable 64-bit hashing (xxHash64 + integer mixer) shared by five primitives — stdlib `hash/maphash` cannot be used because its seed is per-process and cannot be fixed, which would break replay and cross-process agreement |
| `internal/benchutil/` | *(addition)* zero-alloc latency histogram and benchmark reporting helpers (p99) |
| `internal/pad/` | *(addition)* cache-line size constant per architecture (see §7.1) |
| `internal/wire/` | *(addition, chunk 8)* gateway→shard binary frame format and query messages |
| `internal/pipeline/` | *(addition, chunk 8)* sans-IO gateway core: parse → limit → hot-key → route → ring |
| `internal/engine/` | *(addition, chunk 8)* sans-IO shard engine: column store, index, GROUP BY, HLL, query execution, coordinator merge |
| `test/e2e/` | End-to-end suite against the real binaries (build tag `e2e`) |
| `docs/architecture/<slug>.md` | Neon's architecture write-ups, one per primitive/feature |
| `docs/` (`index.md`, `_config.yml`) | Pages site source (Jekyll); the write-up list is generated from front matter, so no chunk edits a shared index file |
| `docs/knowledge/` | In-repo knowledge store entries, one per landed chunk, indexed by the vendored kb |
| `docs/project-source.md` | Nick's spec, verbatim |
| `.github/workflows/pages.yml` | Builds `docs/` and deploys to GitHub Pages on every push to `main` |
| `.github/workflows/ci.yml` | Build, vet, test, race on Linux; build + test on macOS |
| `kb/` | Vendored copy of the kb tool (whole repo at a recorded SHA; its own Go module, so `./...` skips it) |
| `agent_work/INDEX.md` | Index of every script: name, purpose, chunk, author seat, last result |
| `agent_work/orch/` | Orchestration scripts: launch, wait, stage chains, land, Neon/Astra runners, kb sync |
| `agent_work/gates/` | Gate scripts: `gate.sh`, `detlint.sh`, cross-build, comment-only check, prose provenance, self-containment, Pages verify, bench runner + stats |
| `agent_work/probes/` | Feasibility probes (three from Phase 1 already here) |
| `agent_work/adversarial/<chunk>/` | Astra's adversarial harnesses (Go tests behind the `adversarial` build tag, bash drivers), `run-all.sh`, results per run — permanent |
| `agent_work/<chunk>/` | Per-chunk work notes: brief copy, build notes, reviews, seeds, gate results, bench raw output, evidence pack, acceptance |
| `PLAN.md`, `STATE.md` | This plan; current state and resume instructions |

Product code policy: pure Go, no cgo, no assembly, no `unsafe` (the SWAR loads compile to
single instructions through `encoding/binary`). `go.mod` has zero `require` lines unless
Nick approves one (§9 Q2).

---

## 4. Chunk plan

Sizes are product + test Go lines (rough) and expected pipeline wall-clock on this machine.
Every chunk's Opus brief lists the tests below as its definition of done; Opus commits the
tests (red) before the implementation and records the red run in its build notes.
"Astra attacks" are the claims Astra is asked to break; it may add its own.

### Chunk 0 — Foundation + DST/workload simulator (spec Commit 0)
- **Deliverable:** `go.mod` (module path per §9 Q1, Go pin per §7.12); `internal/sim`
  (§2.1 complete); `internal/xhash`; `internal/benchutil`; `internal/pad`; gate scripts
  (`agent_work/gates/*`), orchestration scripts (`agent_work/orch/*`); kb vendored into
  `kb/` with `agent_work/orch/kb-sync.sh` and the recorded SHA, `docs/knowledge/` seeded;
  Pages scaffold (`docs/_config.yml`, `docs/index.md`, `.github/workflows/pages.yml`) and
  `ci.yml`; README stub (Neon) with the live site URL.
- **Tests first:** all §2.1 meta-tests; `xhash` vectors + avalanche; histogram accuracy
  tests (`benchutil` p99 within one bucket of an exact sort); `FuzzZipfGenerator`,
  `FuzzEventLoop`; gate-script self-tests (detlint catches a planted `time.Now`; the
  comment-only check catches a planted code change; provenance check catches a planted
  non-Neon comment).
- **Astra attacks:** find a seed/knob combination where replay differs; make a forked
  stream leak into another; break clock monotonicity through the public API; get the event
  loop to reorder equal-time events; find a planted-bug class ddmin fails to shrink or
  mis-reports; defeat detlint (aliased imports, method values); xhash collisions/bias on
  structured keys (sequential ints, common prefixes); histogram p99 error on bimodal data.
- **Write-ups:** `deterministic-simulation-harness` (+ site landing page).
- **Size:** ~2,500 lines; 1–1.5 days. **Lands alone, first; freezes the `sim`, `xhash`,
  `benchutil`, `pad` APIs.** Pages must serve this chunk's write-up before any other chunk lands.

### Chunk 1 — Lock-free ring buffers (spec Commit 1)
- **Deliverable:** `ringbuf.SPSC[T]` and `ringbuf.MPMC[T]`: fixed power-of-two capacity,
  `sync/atomic` sequence counters, padded cursors, single and batch Try-enqueue/dequeue,
  blocking helpers with the spin→yield→park strategy.
- **Tests first:** unit tables (constructor errors, full/empty, capacity 1 and 2); all
  ring-buffer properties in §2.2; `FuzzRingBufferOps`; `TestAllocs_Enqueue/Dequeue/Batch`;
  race suite `-cpu 1,2,4`; cancellation test for the blocking helpers; benchmarks vs
  buffered channels and padded-vs-unpadded.
- **Astra attacks:** lost/duplicated item under producer/consumer count and batch-size
  combinations not in the suite; wrap-around at start sequences the suite did not pick;
  ABA-style reordering in MPMC; starvation/livelock with a stalled consumer; SPSC misuse
  detection; a workload where the channel beats the ring (to bound the benchmark claim);
  hidden allocation via escape of `T` with pointer fields.
- **Write-up:** `lock-free-ring-buffers`. **Size:** ~1,800 lines; ~1 day.

### Chunk 2 — Token Bucket / GCRA rate limiter (spec Commit 2)
- **Deliverable:** `ratelimit.TokenBucket`, `ratelimit.GCRA` (integer arithmetic, `now`
  passed in), and a per-tenant limiter table with idle eviction.
- **Tests first:** unit tables (config errors; burst tolerance; smooth pacing — admitted
  spacing equals the emission interval once the burst is spent; overload shedding);
  rate-limiter properties in §2.2; `FuzzRateLimiter`; DST retry-storm and micro-burst
  scenarios with the bounded-queue assertion; `TestAllocs_Allow`; contended race test;
  benchmarks.
- **Astra attacks:** an interval that violates the bound (rounding accumulation over
  10¹² ops, rate/burst coprime pairs); overflow with extreme `now`, rate, burst or cost n;
  backward and repeated timestamps; TB/GCRA divergence; concurrent `Allow` over-admission;
  eviction resurrecting a full burst early; tenant-table memory growth under a
  high-cardinality tenant flood.
- **Write-up:** `token-bucket-and-gcra`. **Size:** ~1,400 lines; ~1 day.

### Chunk 3 — Consistent hashing with bounded loads (spec Commit 3)
- **Deliverable:** `router.Ring`: virtual-node ring (sorted points, binary search),
  immutable snapshots swapped atomically on membership change, bounded-load assignment with
  per-shard load counters, `Assign`/`Release`.
- **Tests first:** unit tables (empty ring, one shard, duplicate shard, remove unknown);
  router properties in §2.2; `FuzzRouterTopology`; DST under Zipf and single-key flood;
  `TestAllocs_Lookup`; concurrent lookup-during-membership-change race test; benchmarks
  and the movement/balance tables.
- **Astra attacks:** topology + key set that moves far more than 1/N; a load pattern that
  exceeds the cap (concurrent assigns racing the counter; release/assign interleavings);
  ε near 0 and huge ε; vnode hash collisions between shards; lookup during snapshot swap
  returning a removed shard; skew that makes every key cascade (latency blow-up).
- **Write-up:** `consistent-hashing-bounded-loads`. **Size:** ~1,300 lines; ~1 day.

### Chunk 4 — Count-Min Sketch hot-key detection (spec Commit 4)
- **Deliverable:** `sketch.CountMin` (w×d counters, double hashing), merge, windowed decay,
  and `sketch.TopK` heavy-hitter tracker with a threshold query.
- **Tests first:** unit tables (dimension errors, saturation at counter max); sketch
  properties in §2.2; `FuzzCountMin`; DST on Zipf streams with a hot key appearing
  mid-stream (detection latency in virtual time); `TestAllocs_Add/Estimate`; benchmarks
  including plain vs conservative update.
- **Astra attacks:** an underestimate (saturation, decay rounding, merge of different
  seeds); keys crafted against the double-hashing scheme to inflate one victim key's
  estimate (false hot-key flag); top-K eviction thrash that drops a true heavy hitter;
  recall collapse at low skew; memory growth in the top-K structure.
- **Write-up:** `count-min-sketch-hot-keys`. **Size:** ~1,100 lines; ~0.75 day.

### Chunk 5 — Inverted index + Roaring bitmaps (spec Commit 5)
- **Deliverable:** `index.Bitmap` (16-bit chunking, array and bitset containers,
  Add/Remove/Contains/And/Or/AndNot/cardinality/iteration, in-place and allocating forms)
  and `index.Inverted` (term → posting bitmap, boolean query evaluation).
- **Tests first:** unit tables per container type and per container-pair operation
  (array×array, array×bitset, bitset×bitset); Roaring and index properties in §2.2;
  `FuzzRoaringOps`; `TestAllocs_*`; benchmarks vs sorted slices and maps across densities
  with bytes/element.
- **Astra attacks:** op sequences around 4096/4097 and chunk edges that desynchronise the
  cached cardinality or leave an empty container; aliasing bugs (`a.And(a)`, in-place op
  with shared containers after a copy); AndNot producing a wrong container type; a density
  where the naive slice wins (bound the benchmark claim); query trees with deep NOT nesting.
- **Write-up:** `inverted-index-roaring-bitmaps`. **Size:** ~2,600 lines; 1.5 days (largest primitive).

### Chunk 6 — SwissTable hash map (spec Commit 6)
- **Deliverable:** `hashmap.Map[K, V]` with an explicit hasher: flat control-byte and slot
  arrays, 7-bit H2, 16-slot groups matched with SWAR over two 64-bit words, triangular
  group probing, tombstones with in-place rehash, doubling resize; plus a GROUP BY
  aggregation harness used by the benchmarks.
- **Tests first:** unit tables for the SWAR primitives (exhaustive per byte position; the
  known false-positive pattern is covered and shown harmless); SwissTable properties in
  §2.2; `FuzzSwissTable`; `TestAllocs_Get/Put/Delete`; benchmarks vs built-in map at 1M
  and 10M distinct keys with GC metrics.
- **Astra attacks:** op sequence diverging from the built-in map (delete-during-probe
  chains, reinsertion over tombstones, resize at exact load boundaries); a hasher/key set
  that makes probing non-terminating or quadratic; tombstone accumulation that triggers
  unbounded growth; iteration set mismatch after rehash; workloads where the built-in map
  wins (bound the claim); zero-value and NaN-like keys.
- **Write-up:** `swisstable-hash-map`. **Size:** ~2,000 lines; ~1.25 days.

### Chunk 7 — HyperLogLog (spec Commit 7)
- **Deliverable:** `hll.Sketch` (p=14, 16,384 one-byte registers), `Add`, `Merge`
  (element-wise max), `Estimate` with small/large-range correction, classic estimator kept
  for the before/after comparison, binary encode/decode of registers.
- **Tests first:** unit tables (register index/rank extraction at bit boundaries, decode
  errors, mismatch errors); HLL properties in §2.2; `FuzzHLLMerge`; accuracy table test;
  `TestAllocs_Add`; benchmarks and memory comparison.
- **Astra attacks:** a partition/merge order that breaks register equality; cardinalities
  where error exceeds the claimed band (transition region, tiny n, n near 10M and beyond);
  structured inputs (sequential ints, low-entropy strings) that bias the estimate —
  really an attack on `xhash`; corrupted/truncated encoded registers; merging a sketch into
  itself.
- **Write-up:** `hyperloglog`. **Size:** ~1,100 lines; ~0.75 day.

### Chunk 8 — Integration (three landings)
Split because the gateway side and the analytics side depend on disjoint primitives and can
be built concurrently; the thin joining chunk is sequential.
- **8A — Gateway pipeline** (needs 1–4): `internal/wire`, `internal/pipeline`
  (zero-alloc line parse → per-tenant GCRA → Count-Min hot-key check and isolation →
  bounded-load route → per-shard ring → batching forwarder). Tests first: pipeline DST on
  the virtual clock (retry storm → bounded queue; hot-key isolation; shard join/leave),
  `FuzzWireFrame`, `FuzzIngestLine`, lifecycle/cancellation/leak tests, error-mapping
  tables, allocs/event gate on the post-parse path. Write-up: `gateway-pipeline`. ~2,200 lines; 1.25 days.
- **8B — Analytics engine** (needs 5–7): `internal/engine` (append-only in-memory columns,
  inverted index over tags, SwissTable GROUP BY with COUNT/SUM/MIN/MAX, per-shard HLL,
  query execution, coordinator merge of partial aggregates and HLL registers). Tests
  first: engine-vs-brute-force differential on seeded workloads, merged-vs-single-shard
  equivalence for every aggregate, `FuzzQueryRequest`, single-writer lifecycle tests.
  Write-up: `analytics-engine`. ~2,400 lines; 1.25 days.
- **8C — Entrypoints + whole-pipeline DST** (needs 8A, 8B): `cmd/gateway`,
  `cmd/analytics` (flags, listeners, graceful shutdown, pprof on the debug address,
  `/metrics`), and the in-process whole-pipeline simulation (gateway core + N engines on
  one virtual clock) asserting the exact forms of E1–E8. Tests first: whole-pipeline DST
  scenarios, process-boundary panic/exit tests, pprof placement test, a smoke E2E (E1 at
  small scale) to prove the binaries work. Write-up: `integrated-platform`. ~1,500 lines; 1 day.
- **Astra attacks (across 8A–8C):** malformed/oversized/slow-loris ingest; frames that
  desynchronise the decoder; tenant and key floods aimed at memory; forcing loss between
  accepted and stored (kill a shard connection mid-batch); shutdown under load losing or
  double-counting events; queries that make coordinator and single-shard answers differ;
  a hot key engineered to evade detection (rotating keys); goroutine leaks on every error path.

### Chunk 9 — E2E suite + fuzz hardening + full benchmark suite
- **Deliverable:** `test/e2e` scenarios E1–E10 (§2.8) against the real binaries; the
  long fuzz campaign (§2.3 budgets) with every crasher fixed and regression-pinned; the
  full benchmark suite re-run on the final tree (serialised) with a consolidated results
  table; `agent_work/gates/e2e.sh`, `fuzz-campaign.sh`, `bench-all.sh`.
- **Tests first:** the E2E scenarios themselves are the tests; each is written with its
  oracle and assertion before any fix-up of product code they expose.
- **Astra attacks:** scenario-level — seeds/scales where an E2E assertion fails; long-run
  (30 min) soak for memory growth and goroutine leaks; chaos during E8 (kill -9 a shard,
  restart) to map what the platform does and does not promise.
- **Write-ups:** `end-to-end-verification` and `benchmark-results`. **Size:** ~2,000 lines + overnight campaign; 2 days.

### Chunk 10 — Publishing finish
- **Deliverable:** final README (Neon); site complete (landing page, reading order, every
  write-up cross-linked to its tests and benchmark files); self-containment sweep across
  README, `docs/`, the site and Go comments; prose-provenance report for the whole repo;
  final STATE.md; both kb stores complete.
- **Tests first:** the sweep scripts are the tests — link checker over the built site
  (every internal link 200), self-containment grep, provenance check, every write-up's
  quoted numbers matched against the committed benchmark files by a script.
- **Astra attacks:** find a claim in any write-up or the README that the repo's evidence
  does not support (number mismatch, un-narrowed claim, dead link, a test named in prose
  that does not exist).
- **Write-up:** site index + README. **Size:** prose only; 0.5–1 day.

**Totals:** ≈ 22k lines of Go; 13 landings; roughly 8–10 working days of pipeline time with
the concurrency below, dominated by review/adversarial/prose stages rather than building.

---

## 5. Concurrency map

```
Chunk 0 (alone; freezes sim / xhash / benchutil / pad APIs and the gate scripts)
   │
   ├── 1 ringbuf ─────┐
   ├── 2 ratelimit ───┤
   ├── 3 router ──────┼──► 8A gateway pipeline ──┐
   ├── 4 sketch ──────┘                          │
   ├── 5 index ───────┐                          ├──► 8C entrypoints + DST ──► 9 ──► 10
   ├── 6 hashmap ─────┼──► 8B analytics engine ──┘
   └── 7 hll ─────────┘
```

- **Independent after Chunk 0:** chunks 1–7 touch disjoint packages and import only
  `sim`, `xhash`, `benchutil`, `pad`. All seven *can* run in parallel.
- **Practical cap: 4 chunk pipelines active at once**, rolling (a slot frees when a chunk
  lands or parks). Reasons: 4 vCPUs (race and fuzz runs contend); usage pools (seven
  simultaneous Opus builders plus Neon and Astra stages would drain the weekly limits —
  gauges are checked before each dispatch per the standing protocol); Neon spend cap.
  **Start order:** 5, 6, 1, 2 first (the longest poles), then 3, 4, 7 as slots free.
- **Frozen-interface rule:** a primitive chunk that needs a change to `sim`/`xhash`/
  `benchutil`/`pad` does not edit them in its branch. It files the request in its build
  notes; the orchestrator dispatches a small foundation patch (own mini-pipeline), lands
  it, and the waiting chunks rebase.
- **8A may start as soon as 1–4 have landed, 8B as soon as 5–7 have** — they need not wait
  for each other. 8C waits for both. 9 waits for 8C. 10 waits for 9.
- **Worktrees and branches:** one worktree per chunk at
  `<repo>-work/chunk-NN-<slug>` on branch `chunk/NN-<slug>`, created from current `main`.
  Review, repair, adversarial and prose agents for a chunk are each fresh agents working
  in that chunk's worktree, strictly one at a time. Only the orchestrator touches the main
  checkout. Worktrees are removed after landing.
- **Shared files:** `STATE.md`, `agent_work/INDEX.md`, `PLAN.md`, `README.md` and `go.mod`
  are edited only by the orchestrator on `main` (README text itself comes from a Neon
  run). The site's write-up list is generated from front matter, so parallel chunks never
  conflict on an index page.
- **Benchmarks under concurrency:** builders run short sanity benchmarks only. Evidence
  benchmarks are run by the orchestrator under `flock /root/runs/gateway-bench.lock` with
  no fuzz/race job running, so numbers from parallel chunks are comparable.
- **Landing sequence when chunks finish out of order:** landings are serialised by the
  orchestrator: rebase the branch on `main` → re-run gates 1–6 on the rebased tree → merge
  `--no-ff` → push → verify Pages and CI. Preferred order on `main` is spec order (1…7) so
  `git log --first-parent` reads as the tour. A finished chunk therefore **parks**
  (accepted, rebased, waiting) while lower-numbered chunks are in their normal pipeline.
  Parking is bounded: if the blocking chunk is in arbitration or more than 12 hours
  behind, the parked chunk lands out of order and STATE.md records it. Parking never
  delays downstream *building* — 8A/8B may branch from a parked chunk's accepted tip.
- **Commit shape per landing:** the chunk branch carries a short curated series —
  tests+code (Opus), repair if any (Opus), adversarial harness (Astra), comments +
  write-up + knowledge entry (Neon) — under one merge commit titled with the spec's
  subject line (e.g. `feat(core): add thread-per-core Lock-Free Ring Buffers`) whose body
  states what the tests prove (invariants held, benchmark headline, adversarial
  held/broken, narrowed claims). Keeping the series, not squashing, is what lets the Neon
  authorship of prose be verified in history.

---

## 6. Per-chunk pipeline

Each stage is a fresh headless agent with a brief file (`/root/briefs/gateway-<chunk>-<stage>.md`,
copied into `agent_work/<chunk>/` at landing), a detached run with log/pid/exit files under
`/root/runs/`, a launch check at ~15 s (log exists, process alive, cwd is the chunk
worktree), a deadline, and one re-dispatch with the failure context on a nonzero or
missing exit. A second failure stops that chunk's chain and is reported. Other chunks continue.

| # | Stage | Seat | Input → output |
|---|---|---|---|
| 1 | Tests-first brief | Fable | §2 + §4 for the chunk → brief naming every test, the API surface, hot paths with alloc gates, benchmarks, error contract, the no-comments rule, acceptance commands |
| 2 | Build | Opus (fresh, worktree) | brief → tests committed red, then implementation; **code logic only**: no comments except compiler directives, no markdown except `agent_work/<chunk>/build-notes.md` (operational: decisions, red→green record, seeds, hints for the prose pass) |
| 3 | Review | Neon | diff + brief + build notes → `review-1.md`: PASS / NITS / FAIL with numbered findings; reviews logic, tests' adequacy against §2, error contract, concurrency rules |
| 4 | Repair (one round) | Opus (fresh) | review findings → fixes; skipped on PASS |
| 5 | Re-review | Neon (fresh) | repair diff → `review-2.md`. A second FAIL or a standoff → arbitration |
| 6 | Adversarial | Astra (`gpt-6-astra`, fresh) | claims list + public API → harnesses in `agent_work/adversarial/<chunk>/` with `run-all.sh`; **the orchestrator runs them** and records HELD/BROKEN per attack in `results-run<N>.txt` and INDEX.md |
| 7 | Full gate suite | Fable | §2.9 gates 1–11 on the post-adversarial tree |
| 8 | Prose (once, late) | Neon (fresh) | evidence pack (final API, test names, gate results, benchmark numbers, adversarial results, **every narrowed claim**) → all Go comments in the chunk's packages, `docs/architecture/<slug>.md`, `docs/knowledge/<entry>.md`; committed with the Neon author identity |
| 9 | Post-prose gates | Fable | comment-only check, provenance, self-containment, gates 1–6 again |
| 10 | Acceptance | Fable | `acceptance.md`: claim ↔ evidence table, held/broken, narrowed claims, open risks |
| 11 | Land | Fable | rebase, quick gates, `--no-ff` merge, push |
| 12 | Publish + verify | Fable | Pages workflow green; write-up URL returns 200 with the content marker; CI green |
| 13 | Upkeep | Fable | STATE.md, `agent_work/INDEX.md`, in-repo kb index, vm-1 kb entry, progress file; worktree removed |

**Handling a BROKEN adversarial result.** The mission's stage order puts the adversarial
pass after the single repair round. So a BROKEN is classified by the orchestrator:
(a) *claim overreach* (e.g. a benchmark advantage that does not hold for some workload):
the claim is narrowed in the evidence pack and the attack stays in the suite marked
`BROKEN→NARROWED`; (b) *real defect in a stated invariant*: this is treated as the chunk's
second FAIL and goes to arbitration, where the arbiter rules "targeted fix" (a fresh Opus
fix limited to that defect, Neon confirm-review, adversarial re-run) or "narrow". Nothing
BROKEN lands unaddressed. See §9 Q4 for a proposed reorder that would make this cheaper.

**Arbitration seat.** Builder is Anthropic (Opus), reviewer is Google (Neon); the third
family's top seat is OpenAI's Astra, run as a fresh agent with the diff and both verdicts.
(Confirm in §9 Q5, since Astra is also the adversarial seat.)

**Neon mechanics.** Before the first Neon stage of each chunk: route probe (direct
`google/gemini-4-neon` first, OpenRouter fallback), with `opencode` on PATH and both
secrets files sourced; the live route is recorded in `agent_work/<chunk>/neon-route.txt`.
A 429/quota on one route switches routes; it is not a pipeline failure. Every Neon brief
ends with a done-marker the runner checks, because a budget cut-off can exit 0 silently.

**Enforcing "Opus writes no prose, Neon changes no code".** Gate scripts from Chunk 0:
`no-comments-check` (Opus commits contain no non-directive comments), `comment-only-check`
(Neon commits leave the comment-stripped AST of every `.go` file unchanged),
`prose-provenance` (blame of every comment line and reader-facing markdown line is the
Neon identity). When a later chunk's code change invalidates an earlier comment, Opus
deletes the stale comment line and lists it in build notes; that chunk's Neon pass rewrites it.

---

## 7. Key technical decisions (researched; recommended)

**7.1 Ring buffer: memory ordering and padding.** Go exposes only sequentially-consistent
atomics, so there is no relaxed/acquire/release choice to make; the design work is in
*minimising* atomic operations and shared-line traffic. Use typed atomics
(`atomic.Uint64`: alignment guaranteed, no accidental non-atomic access). **SPSC:**
Lamport ring with each side caching the other side's cursor (the producer re-reads the
consumer's cursor only when its cached copy says "full"), so the steady state is one
atomic store per operation and almost no cross-core line bouncing. **MPMC:** Vyukov-style
bounded queue with a per-slot sequence number (the Disruptor's sequence idea), CAS on the
cursor, monotonically increasing `uint64` sequences masked for indexing; wrap-around is
safe by unsigned arithmetic and is tested by starting cursors near `MaxUint64`. Batch
operations claim a range with one CAS. **Padding:** cursors separated by full cache-line
pads on both sides, hot read-mostly fields (mask, buffer slice header) on their own line.
Probe result (`agent_work/probes/padding-layout`): with 64-byte pads the two cursors sit
on different 64-byte lines and two hammering goroutines run ~40% faster than unpadded
(≈26 ns vs ≈37 ns/op); **but 64 bytes does not separate them on Apple-silicon 128-byte
lines.** Recommendation: `internal/pad` defines the pad size per GOARCH — 64 on amd64, 128
on arm64 — rather than a literal `[64]byte` (§9 Q8). "Thread-per-core" in Go means one
worker goroutine per shard with single-writer ownership; no OS-thread pinning by default
(§9 Q9).

**7.2 GCRA vs Token Bucket state per tenant.** GCRA keeps **one `int64`** per tenant (the
Theoretical Arrival Time), updated with a single CAS — lock-free, 8 bytes, and an idle
tenant's state is indistinguishable from a fresh one once `TAT ≤ now`, so eviction is
always safe. Token Bucket keeps `(tokens, last)` — two words, updated under single-writer
ownership (or a small lock in the contended benchmark variant). Both use **integer
arithmetic only**: time in ns, rate as a rational `(limit, periodNs)`, token/TAT values
scaled so the emission interval is exact; no floats, so the §2.2 bound is exact against a
`math/big` oracle. Non-monotonic `now` is clamped to the last seen value. The gateway uses
GCRA by default (smaller, lock-free); Token Bucket is implemented in full, tested to be
decision-equivalent, and benchmarked beside it. The spec title also says "Leaky Bucket":
GCRA *is* the leaky-bucket-as-meter algorithm; a separate leaky-bucket-as-queue shaper is
not built (§9 Q10). Tenant table: stdlib `map` behind single-writer shards in Chunk 2
(swapping in the Chunk 6 SwissTable is evaluated in 8A with a benchmark, not assumed).

**7.3 Consistent hashing: hash function, virtual nodes, bounded loads.** Hash: `xhash`
(xxHash64, seedable, deterministic across processes; verified against the published test
vectors). Virtual-node point = `xhash(shardID ‖ replicaIndex)`. **Default 160 vnodes per
shard**, configurable; the balance table for V ∈ {16, 64, 160, 512} is produced as
evidence (theory: load spread ≈ 1/√V, so ~8% at 160) — with bounded loads enforcing the
hard cap, the vnode count is about smoothness, not safety. Ring = sorted `[]uint64` points
+ parallel shard-index slice, binary search, immutable snapshot swapped via
`atomic.Pointer` on membership change (lookups never lock). **Bounded loads**
(Mirrokni–Thorup–Zadimoghaddam): cap = ⌈(1+ε)·(total+1)/n⌉, default ε = 0.25; walk
clockwise from the key's point to the first shard under the cap. **What "load" means here
is a real choice (§9 Q3):** recommended is *in-flight events per shard* (assign on route,
release on shard acknowledgement), which makes the spec's invariant hold exactly per
assignment and lets a single hot key spill to the next shard instead of overloading one.
The consequence — a key's events may land on more than one shard during overload — is
safe for this platform because every aggregate the engine serves is mergeable (sums,
counts, min/max, HLL registers).

**7.4 Count-Min: hash independence and sizing.** One 64-bit `xhash` per key, split into
two 32-bit halves, with row i using `h1 + i·h2` (Kirsch–Mitzenmacher double hashing, `h2`
forced odd) — one hash computation per update instead of d. The never-underestimates
guarantee does not depend on hash quality; the εN error bound does, so the bound is
measured across seeds and Astra attacks the scheme with crafted keys. Width rounded up to
a power of two (mask, not modulo). Defaults: ε = 0.001, δ = 0.02 → w = 4096 (≥ e/ε =
2,719), d = 4, `uint32` saturating counters → 64 KiB. Conservative update is implemented
as an option and reported as the "after" in the accuracy benchmark (it gives up
mergeability, so the per-shard merged sketch stays plain). Heavy hitters: a size-K
min-heap with a small index (stdlib map) updated on `Add`. "Real time" means windowed:
counters are halved at each window boundary on the virtual clock, which preserves the
guarantee against the equally-decayed truth.

**7.5 Roaring container thresholds.** The canonical layout: high 16 bits select a
container (sorted `[]uint16` keys + parallel containers, binary search); **array container**
(sorted `[]uint16`) up to and including 4,096 elements; **bitset container**
(`[1024]uint64`, 8 KiB) from 4,097. 4,096 is the break-even where 2 bytes × n equals the
fixed 8 KiB. No hysteresis (matching the reference design and the spec's 4095/4096
boundary tests); promotion/demotion is exact and tested through every operation that can
change cardinality. Array∩array uses a linear merge, switching to galloping search when
sizes differ by ≥ 64×; bitset ops are word loops with `math/bits.OnesCount64`. **Run
containers are not built** — the spec names only array and bitset containers. Document
ids are `uint32`, assigned monotonically per shard.

**7.6 SwissTable group probing in pure Go.** **SWAR over two `uint64` words per 16-slot
group** rather than a scalar loop or assembly. Probe result
(`agent_work/probes/swar-group`, 2M random groups): SWAR H2 matching never misses a true
match, reports a spurious candidate in 0.05% of groups (harmless — the key comparison
rejects it), matches empties exactly, and runs ~2× faster than a 16-byte scalar loop
(≈15.6 vs ≈32 ns per group including mask packing; 0 allocs). Control bytes: empty
`0x80`, deleted `0xFE`, full = 7-bit H2; H1 (upper 57 bits) selects the group; groups are
aligned (no mirrored tail bytes); triangular probing over a power-of-two number of
groups; max load 7/8; delete writes *empty* when the group still has an empty slot (no
probe chain can pass through it) and a tombstone otherwise; rehash in place when
tombstones dominate, double otherwise. Hasher is an explicit constructor argument with a
per-table seed (deterministic under simulation; lets tests inject H1/H2-controlling
hashers). **Expectation management:** Go's built-in map is already a Swiss table with
runtime-internal advantages, and this VM has no cache-miss counters. The plan does not
pre-claim a win on lookups; the likely defensible claims are fewer allocations and lower
GC cost for pointer-free aggregate slots, pre-sized bulk aggregation, and bytes/entry.
Whatever the benchmarks show is what the write-up says.

**7.7 HLL precision and bias correction.** p = 14 per spec: 16,384 registers, one byte each
(16 KiB; 6-bit packing would save 4 KiB at the cost of shifts on the hot path — not
taken), standard error 1.04/√m = 0.8125%. 64-bit `xhash`: top 14 bits index, rank = leading
zeros of the remaining 50 bits + 1. **Estimator:** recommended primary is Ertl's improved
estimator (2017), which corrects the small and large ranges analytically from the register
histogram, needs no empirical bias tables, and has no bump at the linear-counting hand-off.
The classic Flajolet estimator with linear counting for the small range (the 32-bit
large-range correction is unnecessary with a 64-bit hash) is implemented alongside as the
"before" and its known error bump around 2.5m–5m (≈ 40k–80k) is shown in the accuracy
table. This reading of the spec's "bias correction for small/large ranges" is flagged in
§9 Q11. Merge = element-wise max; sketches carry (precision, hash seed) and refuse
mismatched merges with a typed error. No sparse representation (not in spec).

**7.8 Property-testing approach.** **Stdlib only**, via the `internal/sim` helpers: seeded
byte-string generation, op decoders shared with fuzz targets, ddmin shrinking, REPLAY
lines. `testing/quick` is not used (frozen, no shrinking, no seed replay). The named
alternative is `pgregory.net/rapid` (integrated shrinking, state-machine tests): better
shrinking for free, but it is a dependency in a repo whose point is from-scratch
construction, and its replay format would sit beside ours. Recommendation: no dependency
(§9 Q2).

**7.9 Fuzz budget policy.** As §2.3: 60 s builder / 3 min gate / 30 min–2 h hardening per
target; seeds in code, crashers committed and promoted to named regressions. Fuzzing runs
with the default worker count on a quiet machine; never concurrently with evidence benchmarks.

**7.10 Pages build tooling.** GitHub's own Jekyll build, no local toolchain:
`actions/checkout` → `actions/configure-pages` → `actions/jekyll-build-pages` (source
`docs/`) → `actions/upload-pages-artifact` → `actions/deploy-pages`, triggered on push to
`main` and manually. Stock theme, `jekyll-relative-links` so the same markdown links work
on GitHub and on the site, write-up list generated from front matter (`order:`). Pages is
already enabled with `build_type=workflow` (verified via the API); the workflow file lands
in Chunk 0. Verification after every landing: workflow run concluded `success`, then fetch
the write-up URL with retries and require HTTP 200 plus a content marker.

**7.11 macOS verification.** Gate on darwin **cross-build** per chunk (arm64 and amd64:
build, vet, compile test binaries) — cheap and catches build tags and arch constants. In
addition `ci.yml` runs `go test ./...` on a `macos-latest` runner after each push, which
actually executes the suite on Apple silicon (including the 128-byte padding path); a red
macOS job is a fix-forward item that blocks the next landing. Golden simulation digests
are asserted on linux/amd64 only (§2.1).

**7.12 Go version pin.** `go 1.26` with `toolchain go1.26.8` in `go.mod` (the installed
latest stable); CI reads the version from `go.mod`. Features relied on: typed atomics,
generics, `testing.B.Loop`, `testing/synctest`, native fuzzing.

**7.13 Ingest, wire and query surface (integration; mostly open — §9 Q6/Q7).** Recommended:
HTTP ingest on the gateway with a compact line format parsed without allocation from
pooled buffers; length-prefixed binary batches over TCP from gateway to shards; queries as
a small structured HTTP/JSON request (filter tree over tags, optional group-by,
aggregates including `APPROX_COUNT_DISTINCT`) sent to the gateway, which fans out to
shards and merges; in-memory storage only.

**7.14 kb vendoring and knowledge flow.** Chunk 0 copies `/root/Software/kb` at HEAD into
`kb/` (`git archive | tar`), records the SHA in `agent_work/kb-vendor.sha`, and adds
`agent_work/orch/kb-sync.sh` (`vendor` / `index` / `status`), following the
sqlite-internals pattern; the built binary and index live in a git-ignored `_tools/`. Each
landing adds one `docs/knowledge/gateway-chNN-<slug>.md` (durable findings: measured
numbers, corrected assumptions, narrowed claims), written by Neon in the prose pass,
re-indexes the in-repo store, and adds the same entry to the vm-1 store with `kb add`.
(The mission's item 6 governs here over the global "project knowledge never in the kb CLI"
rule.) Planning each chunk's brief starts with `kb search`; Phase 1's searches found no
prior entries on the seven primitives themselves; the useful prior art was the
sqlite-internals kb-vendoring pattern and the DST technique ladder in
`/root/Software/dst-research` (its rungs 1–7, 12–16, 20 and 36 are what §2.1 adopts).

---

## 8. Spec coverage map (nothing dropped silently)

| Spec item | Where |
|---|---|
| Overview: Go, zero-alloc hot paths, padding, bit manipulation, pprof | §2.4, §2.6, §7.1, §7.6; pprof in 8C |
| Guideline: all 7 from scratch, no external libraries | §3 policy; gate 3 |
| Guideline: `-benchmem`, `[64]byte` padding, `sync/atomic` | §2.4, §7.1 (padding size refined per arch — §9 Q8) |
| Guideline: pprof hooks; before/after benchmarks with allocs, ops/s, p99 | §2.4; "before/after" pairs: padded/unpadded, channel/ring, plain/conservative CMS, slice-map/Roaring, built-in/SwissTable, classic/improved HLL |
| Repository structure (10 paths) | §3, all kept; six internal packages added, each with a reason |
| Commit 0 (sim) + its meta-test | Chunk 0; §2.1 |
| Commit 1 (ring buffers) + channel benchmark + race | Chunk 1 |
| Commit 2 (Token Bucket / GCRA) + time-simulated tests | Chunk 2 |
| Commit 3 (consistent hashing) + movement/variance tests | Chunk 3 |
| Commit 4 (Count-Min) + Zipf recall + bounded memory | Chunk 4 |
| Commit 5 (inverted index + Roaring) + comparison benchmarks | Chunk 5 |
| Commit 6 (SwissTable) + built-in map benchmarks, cache efficiency, GC pressure | Chunk 6 (cache efficiency by proxy — no PMU on this VM, §2.4) |
| Commit 7 (HLL) + accuracy + single/merged equivalence | Chunk 7 |
| Core DST & fuzzing architecture (3 bullets) | §2.1, §2.3, §2.4, §2.6 |
| Ring buffer suite (invariants + fuzz dimensions) | §2.2 ring table; `FuzzRingBufferOps` |
| `FuzzRateLimiter` + traffic DST | §2.2 limiter table; §2.3 |
| Consistent hashing simulation (minimal disruption, bounded load) | §2.2 router table |
| `FuzzRoaringOps` | §2.2 Roaring table; §2.3 |
| `FuzzSwissTable` | §2.2 SwissTable table; §2.3 |
| Count-Min + HLL statistical properties | §2.2 |
| cmd/gateway, cmd/analytics (named in structure, no commit of their own) | Chunks 8A–8C (added, per the mission) |

**Cut or merged, with reasons:**
- *Leaky-bucket-as-queue shaper* — not built; GCRA is the leaky-bucket meter and the spec's
  scope line names only Token Bucket and GCRA (§9 Q10).
- *Roaring run containers, serialisation* — not in spec.
- *HLL sparse mode, HLL++ empirical bias tables* — not in spec; tables would be copied data.
- *Persistence, replication, SQL text parser* — spec is silent; proposed as non-goals (§9 Q7).
- *Hardware cache-miss counters* — unavailable on the build VM; replaced by proxies.
- *Chunk 8 split into 8A/8B/8C* — to build the two halves concurrently.

---

## 9. Open questions for Nick

No default below has been applied to anything that changes scope; each has a recommendation.

1. **Repo name / module path.** `analytics-edge-gateway` /
   `github.com/nickstrad/analytics-edge-gateway` (used by the scaffold). Confirm or rename
   before Chunk 0 creates `go.mod`.
2. **Dependencies.** Recommended: **zero** third-party modules — property testing and
   shrinking via our own `internal/sim` helpers, benchmark comparison via a small in-repo
   script. Alternatives you may prefer: `pgregory.net/rapid` (better shrinking) and/or
   `golang.org/x/perf/cmd/benchstat` as a dev-only tool (statistically sounder benchmark
   comparison; not imported by product code). Approve none, one, or both.
3. **Meaning of "load" for bounded loads.** Recommended: in-flight events per shard, with
   overflow of a hot key spilling to the next shard (correct because all served aggregates
   merge). Alternative: sticky key→shard assignment counted in keys, which keeps per-key
   affinity but cannot bound a single hot key's load. This shapes Chunks 3, 8A and E8.
4. **Pipeline order tweak.** The mission's order runs Astra after the single repair round,
   so a real defect found by Astra must go to arbitration. Proposal: let Astra write and
   run its harness **in parallel with Neon's first review** (both against the same build
   commit), so the one repair round fixes review findings and broken attacks together,
   and the adversarial suite is re-run after re-review. Same seats, same stages, less
   arbitration, shorter wall-clock. Approve, or keep the order as written (the plan
   currently follows it as written).
5. **Arbitration seat.** Recommended: Astra (fresh agent) as the third family's top seat.
   Astra is also this project's adversary; if you want the arbiter to be uninvolved,
   name Muse Wonder Full instead.
6. **Query coordinator placement.** Recommended: the gateway hosts `/query`, fans out to
   shards and merges. Alternative: a coordinator role inside `cmd/analytics`.
7. **Query and storage scope.** Recommended: structured HTTP/JSON queries (filters,
   GROUP BY, COUNT/SUM/MIN/MAX, `APPROX_COUNT_DISTINCT`, top-K hot keys); in-memory
   storage only; no persistence, replication, or SQL text parser. Say if any of these
   should be in scope (a minimal SQL subset parser would add roughly a chunk).
8. **Cache-line padding size.** The spec says `[64]byte`. The probe shows 64 bytes does
   not prevent false sharing on Apple silicon (128-byte lines), and you may run this on a
   Mac. Recommended: 64 on amd64, 128 on arm64 via `internal/pad`. Alternative: literal
   `[64]byte` everywhere, as written.
9. **OS-thread pinning for "thread-per-core".** Recommended: no pinning — one worker
   goroutine per shard, single-writer. CPU affinity needs raw syscalls or `x/sys` and is
   Linux-only. Optional `runtime.LockOSThread` behind a flag can be benchmarked in Chunk 9
   if you want the comparison.
10. **Leaky bucket.** Recommended: no separate queue-style leaky bucket; the write-up
    explains GCRA as the leaky-bucket meter. Say if you want the shaper variant built too.
11. **HLL estimator.** The spec says "bias correction for small/large ranges".
    Recommended: Ertl's improved estimator as primary, with the classic
    Flajolet + linear-counting estimator kept as the measured baseline. Alternative:
    classic only (simpler, with a known error bump near 40k–80k that may exceed the 0.8%
    target there and would then be reported as a narrowed claim).
12. **Statistical-test floors.** Property tests on sketches assert numeric floors
    (e.g. HLL RMSE ≤ 1.0% per cardinality over pinned seeds; top-10 recall ≥ 0.9 at
    s = 1.1). Proposed policy: floors are set from theory before the build and the
    measured values are published; if a floor is missed the claim is narrowed in the
    open, never the test loosened quietly. Confirm the policy.
13. **CI on GitHub Actions.** Recommended: add `ci.yml` (Linux build/vet/test/race; macOS
    build/test) alongside the Pages workflow. It uses the public repo's free minutes.
    Decline if you want Pages to be the only workflow.
14. **Concurrency cap and landing order.** Recommended: at most 4 chunk pipelines at once
    and spec-order landings with bounded parking (§5). A higher cap is possible but will
    drain usage pools faster and make benchmark scheduling tighter.
15. **Commit subjects.** Recommended: merge commits use the spec's conventional-commit
    subjects (`feat(core): …`) for the tour, rather than the global `<area>: <summary>`
    style. Confirm.
16. **`docs/project-source.md` and prose provenance.** It is your spec verbatim, committed
    by Twin, and lives under `docs/`. Recommended: exempt it (and PLAN/STATE/agent_work)
    from the Neon-only rule and keep it out of the published site navigation.
