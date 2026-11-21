# STATE — analytics-edge-gateway

Goal: build Nick's "High-Throughput Analytics & Edge Gateway"
(spec: docs/project-source.md) — a high-throughput event ingestion
edge gateway and real-time OLAP analytics engine (Mini-ClickHouse +
Envoy) in Go from scratch, structured as a readable commit history
where each commit introduces a CPU-, memory-, or routing-focused
primitive: lock-free ring buffers, Token Bucket / GCRA, consistent
hashing with bounded loads, Count-Min Sketch, inverted index with
Roaring Bitmaps, SwissTable-style hash map, and HyperLogLog —
designed testing-first: DST, property tests, native fuzzing,
allocation gates, race validation, and E2E tests guide the build.

Status: Phase 1 (planning) — Fable orchestrator commissioned 2026-11-20.
HARD GATE: no product building until Nick signs off on PLAN.md.

Seats (Nick, 2026-11-20, this project only, per the setup guide
"Setting Up the Multi-Agent Build Flow From Scratch"): Fable
orchestrates; Opus writes Go code (logic only), concurrently per
chunk where the dependency graph allows; Gemini Neon reviews Opus
and writes ALL reader-facing prose (README, docs, Pages site, code
comments) once, late, per chunk; Astra (gpt-6-astra) adversarially
tests each chunk. Full ruling: the orchestrator mission at
/root/briefs/gateway-orchestrator-mission.md.

Done:
- 2026-11-20: Twin read Nick's Google Doc
  (1mpALejYepx9INw9M1RIKUb9k4nKTDD4J_7-5_p1loPE) and saved the faithful
  spec to docs/project-source.md; repo created PUBLIC at
  github.com/nickstrad/analytics-edge-gateway with this scaffold.

In progress:
- Phase 1: Fable orchestrator producing PLAN.md — testing architecture
  first, then repo layout, chunk plan with a concurrency map, per-chunk
  pipeline, technical decisions, and open questions for Nick.

Next steps:
- Nick signs off on PLAN.md (Twin relays), then Phase 2 build per the
  plan: foundation/DST chunk first, primitive chunks with Neon
  write-ups publishing to GitHub Pages as each lands, E2E phase,
  publishing finish.

Open decisions / blockers:
- All Phase 2 work gated on Nick's plan sign-off.
- Repo name / module path: analytics-edge-gateway /
  github.com/nickstrad/analytics-edge-gateway — used for the scaffold;
  Nick to confirm or rename at plan sign-off.
- GitHub Pages: enabled via the Actions workflow that lands with the
  foundation chunk; site URL will be
  https://nickstrad.github.io/analytics-edge-gateway/
- vm-1 Claude sign-in ends sometime 2026-11-25 to 2026-11-28; keep
  STATE.md resumable at all times.

Resume: read this file, then PLAN.md and
/root/briefs/gateway-orchestrator-mission.md; git log for what has
landed. Orchestrator progress: /root/runs/gateway-orchestrator-progress.txt.
