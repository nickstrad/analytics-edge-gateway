# C4 Level 3 — Component: HyperLogLog (`internal/hll`)

> **TARGET architecture — designed 2026-11-21, awaiting Nick's buy-in.**

Scope: `internal/hll`. Model id: `11-component-hll`.
Source: PLAN.md §3, §4 (Chunk 7), §7.7.

## Mermaid

```mermaid
flowchart TD
  shard["internal/engine<br/>per-shard HLL"]
  merge["internal/engine<br/>coordinator merge"]
  subgraph hll["internal/hll — p = 14"]
    sketch["Sketch<br/>16,384 one-byte registers;<br/>Add, Merge (element-wise max),<br/>mismatch refused with *hll.MismatchError"]
    estimate["Estimate<br/>small/large-range correction;<br/>classic estimator kept for comparison"]
    codec["Register encode/decode<br/>binary"]
  end
  xhash["internal/xhash<br/>seedable xxHash64"]

  shard -->|"Add"| sketch
  merge -->|"Merge"| sketch
  sketch -->|"registers"| estimate
  sketch -->|"registers"| codec
  sketch -->|"64-bit hash: top 14 bits index, rank of remaining 50"| xhash
```

## Text (draw-visual)

<!-- draw-visual: diagrams/11-component-hll.mmd -->
```text
┌───────────────────────┐        ┌─────────────────────────┐
│ Engine: per-shard HLL │        │Engine: coordinator merge│
└───────────┬───────────┘        └────────────┬────────────┘
            │                                 │
           Add                                │
            ▼                                 │
┌───────────────────────┐                     │
│Sketch: 16384 registers│◄───────Merge────────┘
└───────────┬───────────┘
            │
            ├─────────────────────────────────┬─────────────────────────┐
            │                                 │                         │
            │                                 │                       hash
            │                                 │                         │
            ▼                                 ▼                         ▼
┌───────────────────────┐        ┌─────────────────────────┐    ┌──────────────┐
│        Estimate       │        │      Register codec     │    │internal/xhash│
└───────────────────────┘        └─────────────────────────┘    └──────────────┘
```

## Notes

- Primary estimator (Ertl's improved estimator, recommended) is open (PLAN.md §9 Q11).
- No sparse representation is planned (PLAN.md §7.7).
