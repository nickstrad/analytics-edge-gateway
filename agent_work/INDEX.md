# agent_work index

Every script produced by any agent on this project lives in agent_work/
(Go or bash) and is indexed here: name, purpose, chunk, author seat, and
what it proved or its last result. Adversarial scripts and harnesses are
kept permanently under adversarial/ and indexed in their own section.

## Orchestration scripts

| Script | Purpose | Chunk | Author seat | Last result |
|---|---|---|---|---|
| `orch/phase1-orchestrator-run.sh` | Launches the Fable orchestrator headlessly for Phase 1 (plan only) with the mission brief on stdin; writes log and exit files | Phase 1 | Twin | Ran 2026-11-21; produced PLAN.md |
| `orch/neon-route-probe.sh` | Probes both Gemini 4 Neon routes through OpenCode (direct, then OpenRouter) and logs which answer | Phase 1 | Twin | 2026-11-21: direct route live; OpenRouter route refused (credit limit) |

## Probes

Each probe is a standalone Go module (its own `go.mod`), so it never becomes part of the product build.

| Probe | Purpose | Chunk | Author seat | What it proved |
|---|---|---|---|---|
| `probes/sim-determinism/` (`go run . [-seed N]`) | Is a single-seed `*math/rand.Rand` workload stream (virtual-clock steps, Zipf keys, burst decisions) identical on replay? | Phase 1 (feeds Chunk 0) | Fable | 2026-11-21, go1.26.8 linux/amd64: 1M mixed draws hash identically on same-seed replay (seed 42 → `ed9ef8f75856e5c7`), differ across seeds; stdlib `rand.NewZipf` returns nil for s = 1.0 (needs s > 1) |
| `probes/padding-layout/` (`go run .`; `go test -bench . -cpu 2,4`) | Do `[64]byte` pads put two `atomic.Uint64` cursors on separate cache lines, and does it matter? | Phase 1 (feeds Chunk 1) | Fable | Offsets head@64 tail@128: separate 64-byte lines, NOT separate 128-byte (Apple silicon) lines. Two goroutines each hammering one counter: padded ≈ 26–27 ns/op vs unpadded ≈ 36–38 ns/op. Cross-builds for darwin/arm64 |
| `probes/swar-group/` (`go test -run . -v -bench . -benchmem`) | 16-slot SwissTable group matching: SWAR over two uint64 words vs a scalar byte loop — soundness and speed | Phase 1 (feeds Chunk 6) | Fable | 2M random groups: 0 missed matches, spurious candidate in 1,066 groups (0.053%), empty-match exact; SWAR ≈ 15–16 ns vs scalar ≈ 30–35 ns per group, 0 allocs |

Also checked in Phase 1 without a script: `perf stat -e cache-misses,cycles true` reports the events as not supported on this VM (no hardware counters), which is why PLAN.md evidences cache behaviour by proxy.

## Adversarial scripts and harnesses

(none yet — the first adversarial pass runs against Chunk 0 in Phase 2)
