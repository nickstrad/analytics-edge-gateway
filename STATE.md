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

Status: Phase 1 COMPLETE — PLAN.md written and pushed 2026-11-21; initial
C4 diagram set designed with c4-sdk, validated, rendered and landed at
a1b2948 (2026-11-21). Waiting for Nick's sign-off on the plan AND buy-in on
the diagrams (Twin relays his ruling in a brief file).
HARD GATE: no product building, no Opus dispatch, until that ruling.

Seats (Nick, 2026-11-20, this project only): Fable orchestrates; Opus
writes Go code (logic only, no comments), concurrently per chunk where
the dependency graph allows; Gemini 4 Neon reviews Opus and writes ALL
reader-facing prose (README, docs, Pages site, code comments) once,
late, per chunk; Astra (gpt-6-astra) adversarially tests each chunk.
Full ruling: /root/briefs/gateway-seat-ruling.md; mission:
/root/briefs/gateway-orchestrator-mission.md.

Done:
- 2026-11-20: Twin saved the faithful spec (docs/project-source.md) and
  pushed the scaffold (22f717a). Repo is public; GitHub Pages enabled
  with build_type=workflow (no workflow file yet — lands in Chunk 0).
- 2026-11-21: Fable wrote PLAN.md (testing architecture first, repo
  layout, 13-landing chunk plan, concurrency map, per-chunk pipeline,
  technical decisions, spec coverage map, 16 open questions) and three
  feasibility probes under agent_work/probes/ (seeded replay
  determinism; cache-line padding layout; SWAR group matching), all
  indexed in agent_work/INDEX.md. Completion marker:
  /root/runs/gateway-plan-done.txt.
- 2026-11-21: Fable designed the initial C4 set from PLAN.md's target
  architecture with c4-sdk and landed it at a1b2948 in docs/c4/: 11
  diagrams (01 System Context, 02 Container, 03-11 Component: gateway
  pipeline, analytics engine, ringbuf, ratelimit, router, sketch, index,
  hashmap, hll), each labelled "TARGET architecture — designed
  2026-11-21, awaiting Nick's buy-in". docs/c4/model.json is a c4-sdk
  schemaVersion-1 bundle: decodeArchitecture (c4-sdk@0.1.0) accepts it
  with 0 diagnostics (60 elements, 76 relationships, 11 diagrams). The
  15 draw-visual Unicode embeds pass render.py --check at 80 columns;
  the 11 Mermaid blocks render with mermaid-cli. Index, checks to rerun
  and the model-to-c4-sdk mapping: docs/c4/README.md. Completion marker:
  /root/runs/gateway-c4-done.txt.

In progress:
- No agents or background runs are alive for this project.

Next steps (only after Nick's ruling arrives):
1. Apply Nick's edits/answers to PLAN.md section 9 (open questions).
2. Check all usage gauges; write the Chunk 0 tests-first brief
   (/root/briefs/gateway-ch00-build.md) from PLAN.md sections 2.1 and 4.
3. Run Chunk 0 alone through the PLAN.md section 6 pipeline (foundation,
   internal/sim, xhash, gates, kb vendoring, Pages workflow); verify the
   site serves before any other chunk lands.
4. Then chunks 1-7 with at most 4 pipelines at once (start 5, 6, 1, 2),
   8A/8B, 8C, 9, 10 per the concurrency map.

Open decisions / blockers:
- C4 RULING (Nick, 2026-11-21, via Twin): this project keeps a C4
  diagrams section on its GitHub Pages site, up to date per landing,
  designed with Nick's c4-sdk (he first said "c4-ask" and clarified
  2026-11-21 that he meant c4-sdk; /root/Software/c4-sdk, npm
  c4-sdk@0.1.0); designing the initial diagrams is part of kickoff and
  they go to Nick for buy-in together with the plan sign-off. Full
  ruling: /root/briefs/gateway-c4-ruling-2026-11-21.md. The initial set
  is in docs/c4/ (index: docs/c4/README.md), labelled TARGET
  architecture. Owner Nick: buy-in on the diagrams.
- Pages C4 section: stock Jekyll does not render Mermaid blocks; the
  Chunk 0 Pages workflow needs the c4-sdk viewer over docs/c4/model.json
  (or a Mermaid step).
- BOOK RULING (Nick, 2026-11-20, via Twin): in addition to the per-chunk
  Neon write-ups, Neon assembles a concise Leanpub-style BOOK as the
  project's canonical learning artifact - per-chunk write-ups become book
  chapter drafts, Chunk 10 assembles the book, Pages presents the book.
  Full ruling: /root/briefs/gateway-book-ruling-2026-11-20.md. Applies
  at Phase 2 mission time, together with the plan sign-off.
- Owner Nick: sign-off on PLAN.md, and the 16 open questions in its
  section 9 (module path, dependencies, bounded-load semantics,
  pipeline order tweak, arbitration seat, query/storage scope, padding
  size, HLL estimator, CI workflow, and others).
- vm-1 Claude sign-in ends sometime 2026-11-25 to 2026-11-28; keep this
  file resumable at all times.

Resume: read this file, then PLAN.md and the mission/ruling briefs
above, then look in /root/briefs/ for a newer gateway-* brief carrying
Nick's ruling. `git log --first-parent main` shows what has landed.
Work happens on branch main for plan/state files only; chunk work will
use worktrees at ../analytics-edge-gateway-work/chunk-NN-<slug>.
Orchestrator progress log: /root/runs/gateway-orchestrator-progress.txt.
Site (not yet serving): https://nickstrad.github.io/analytics-edge-gateway/
