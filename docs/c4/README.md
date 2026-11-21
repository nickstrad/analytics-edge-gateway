# C4 diagrams — analytics-edge-gateway

> **TARGET architecture — designed 2026-11-21, awaiting Nick's buy-in.**

These diagrams describe the architecture PLAN.md targets. No product code exists yet. As chunks land, each affected diagram is updated to the as-built architecture.

## Diagrams

| # | Level | Diagram | Shows |
|---|---|---|---|
| 01 | 1 — System Context | [System Context](01-system-context.md) | The platform, event producers, query consumers, operators, and the two test systems (`internal/sim`, `test/e2e`). |
| 02 | 2 — Container | [Container](02-container.md) | `cmd/gateway` (ingestion edge proxy + rate-limiting router, query coordinator) and `cmd/analytics` (sharded OLAP query worker node, ×N). |
| 03 | 3 — Component | [Gateway ingest pipeline](03-component-gateway-pipeline.md) | `internal/pipeline`: parse → limit → hot-key → route → ring → forwarder. |
| 04 | 3 — Component | [Analytics engine (query and storage)](04-component-analytics-engine.md) | `internal/engine`: column store, index, GROUP BY, HLL, query execution, coordinator merge. |
| 05 | 3 — Component | [Ring buffers](05-component-ringbuf.md) | `internal/ringbuf`: lock-free SPSC and MPMC ring buffers with cache-line padding. |
| 06 | 3 — Component | [Rate limiter](06-component-ratelimit.md) | `internal/ratelimit`: Token Bucket and GCRA limiters, per-tenant state table. |
| 07 | 3 — Component | [Router](07-component-router.md) | `internal/router`: consistent-hash ring with virtual nodes and bounded loads. |
| 08 | 3 — Component | [Sketch](08-component-sketch.md) | `internal/sketch`: Count-Min Sketch and top-K hot-key detector. |
| 09 | 3 — Component | [Index](09-component-index.md) | `internal/index`: Roaring bitmap containers and the inverted index. |
| 10 | 3 — Component | [Hash map](10-component-hashmap.md) | `internal/hashmap`: SwissTable-style open-addressed hash map. |
| 11 | 3 — Component | [HyperLogLog](11-component-hll.md) | `internal/hll`: HyperLogLog (p=14) with register merge. |

## Files

- `model.json` — the architecture as a c4-sdk bundle (`schemaVersion: 1`): elements, relationships, and the eleven views above, with drill-down links between them. It is the source for the c4-sdk viewer in the Pages C4 section, which the Chunk 0 Pages workflow adds.
- `NN-*.md` — one file per view: a Mermaid rendering (GitHub/Pages) and draw-visual Unicode embeds (terminal/glow) of the same view.
- `diagrams/*.mmd` — the draw-visual sources for the Unicode embeds. The Unicode text is generated from them and never edited by hand. Views 02, 04 and 05 use more than one source, because one text diagram of the whole view does not fit 80 columns.
- `validate.mjs` — decodes `model.json` with c4-sdk and builds every scene.

## Status of this set (2026-11-21)

- **Tool: c4-sdk.** The ruling first named "c4-ask"; Nick clarified on 2026-11-21 that he meant c4-sdk (`/root/Software/c4-sdk` on vm-1, npm `c4-sdk@0.1.0`). There is no separate c4-ask tool.
- **`model.json` is validated.** `decodeArchitecture` accepts it with 0 diagnostics (no errors, no warnings): 60 elements, 76 relationships, 11 diagrams. `buildScene` succeeds for every diagram. Checked against both the published `c4-sdk@0.1.0` package and a bundle of the c4-sdk source at `bb12c7b`.
- **Unicode embeds are rendered.** All 15 embeds pass `render.py --check` at 80 columns.
- **Mermaid blocks are checked.** All 11 render with mermaid-cli (`mmdc`) without errors. They have not been viewed on GitHub itself.
- **Pages.** The Pages workflow lands in Chunk 0. Stock Jekyll does not render Mermaid blocks, so the C4 section needs the c4-sdk viewer (or a Mermaid step) in that workflow.

## Checks to rerun after any change

```sh
# 1. The bundle (any directory with c4-sdk installed; Node 18 or newer)
mkdir -p /tmp/c4-validate && cd /tmp/c4-validate && npm init -y >/dev/null && npm install c4-sdk@0.1.0
node /root/Software/analytics-edge-gateway/docs/c4/validate.mjs

# 2. The Unicode embeds (from the repo root)
R=~/.claude/skills/draw-visual/scripts/render.py
for f in docs/c4/[0-9]*.md; do "$R" --update "$f"; done
for f in docs/c4/[0-9]*.md; do DRAW_VISUAL_MAX_WIDTH=80 "$R" --check "$f"; done
```

## How the model fits c4-sdk's rules

c4-sdk enforces strict C4 nesting: a component belongs to a container, and a component view is scoped to a container. The bundle therefore differs from the pictures in four ways.

- **Go packages are groups, not scopes.** Each Level 3 view is scoped to the process that runs the package (`gateway` or `analytics`) and draws the package as a group (`internal/ringbuf`, …) around its parts. The package elements (`ringbuf`, `router`, …) stay in the model, tagged `package`, but appear in no view.
- **The container view shows the two processes only.** c4-sdk does not allow components in a container view, so view 02 in the bundle has `cmd/gateway` and `cmd/analytics` with process-level relationships. The Mermaid rendering of 02 also draws the main package inside each process.
- **`external` means "outside this view's scope".** c4-sdk requires that tag on a component shown in another container's view. `pipeline`, `coordinator`, `engine` and `en-merge` carry it; each appears only in views of the other process. `en-merge` is `internal/engine` code that runs in the gateway's query coordinator, so it belongs to `gateway` and is drawn inside the `internal/engine` group.
- **`internal/xhash` is two elements**, `xhash-gateway` and `xhash-analytics`: one package linked into both processes.

Every element and relationship has status `planned`. In-process relationships carry the technology `Go call (in-process)`.

## Open questions that shape the diagrams

The diagrams draw the plan's recommended answers. If Nick rules otherwise, these change:

- PLAN.md §9 Q6 — query coordinator in `cmd/gateway` (drawn) or in `cmd/analytics`: diagrams 02, 04.
- PLAN.md §9 Q7 — query and storage scope; in-memory only: diagrams 01, 04.
- PLAN.md §9 Q3 — meaning of "load" for bounded loads: diagrams 03, 07.
- PLAN.md §9 Q8 — cache-line pad size per architecture: diagram 05.
- PLAN.md §9 Q10 — no separate leaky-bucket shaper: diagram 06.
- PLAN.md §9 Q11 — HLL primary estimator: diagram 11.
