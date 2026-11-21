# C4 Level 3 — Component: analytics engine, query and storage (`internal/engine`)

> **TARGET architecture — designed 2026-11-21, awaiting Nick's buy-in.**

Scope: `internal/engine` inside `cmd/analytics` (the coordinator merge runs in the gateway's query coordinator). Model id: `04-component-analytics-engine`.
Source: PLAN.md §3, §4 (Chunk 8B), §7.13.

## Mermaid

```mermaid
flowchart TD
  pipeline["cmd/gateway<br/>internal/pipeline (forwarder)"]
  coordinator["cmd/gateway<br/>Query coordinator (/query)"]
  subgraph engine["internal/engine — sans-IO shard engine"]
    columns["Append-only in-memory columns"]
    index["Inverted index over tags<br/>uses internal/index (Roaring)"]
    exec["Query execution<br/>filter tree, optional group-by, aggregates"]
    groupby["SwissTable GROUP BY<br/>COUNT / SUM / MIN / MAX<br/>uses internal/hashmap"]
    hll["Per-shard HLL<br/>uses internal/hll"]
    merge["Coordinator merge<br/>partial aggregates + HLL registers"]
  end

  pipeline -->|"append events"| columns
  pipeline -->|"index tags"| index
  coordinator -->|"query"| exec
  exec -->|"filter"| index
  exec -->|"read"| columns
  exec -->|"GROUP BY"| groupby
  exec -->|"APPROX_COUNT_DISTINCT"| hll
  exec -->|"partial aggregates, registers"| merge
  merge -->|"merged result"| coordinator
```

## Text (draw-visual)

Ingest path:

<!-- draw-visual: diagrams/04-engine-ingest.mmd -->
```text
┌─────────────────┐         ┌─────────────────────┐
│Gateway: pipeline├─append─►│  In-memory columns  │
└────────┬────────┘         └─────────────────────┘
         │
         │                  ┌─────────────────────┐
         └───────────index─►│Inverted index: index│
                            └─────────────────────┘
```

Query round trip:

<!-- draw-visual: diagrams/04-engine-query.mmd -->
```text
┌──────────────────────────┐
│Gateway: query coordinator│◄───┐
└─────────────┬────────────┘    │
              │                 │
            query               │
              ▼                 │
┌──────────────────────────┐    │
│     Query execution      │ result
└─────────────┬────────────┘    │
              │                 │
          partials              │
              ▼                 │
┌──────────────────────────┐    │
│    Coordinator merge     ├────┘
└──────────────────────────┘
```

What query execution uses:

<!-- draw-visual: diagrams/04-engine-exec.mmd -->
```text
┌───────────────┐           ┌─────────────────────┐
│Query execution├──filter──►│Inverted index: index│
└───────┬───────┘           └─────────────────────┘
        │
        │                   ┌─────────────────────┐
        ├───────────read───►│  In-memory columns  │
        │                   └─────────────────────┘
        │
        │                   ┌─────────────────────┐
        ├─────────GROUP─BY─►│  GROUP BY: hashmap  │
        │                   └─────────────────────┘
        │
        │                   ┌─────────────────────┐
        └─────────distinct─►│  Per-shard HLL: hll │
                            └─────────────────────┘
```

## Notes

- Query and storage scope is the plan's recommendation and remains open: structured HTTP/JSON queries, in-memory storage only, no persistence, replication, or SQL text parser (PLAN.md §9 Q7).
- Document ids are `uint32`, assigned monotonically per shard (PLAN.md §7.5).
