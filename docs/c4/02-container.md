# C4 Level 2 — Container

> **TARGET architecture — designed 2026-11-21, awaiting Nick's buy-in.**

Scope: inside the Analytics Edge Gateway platform. Model id: `02-container`.
Source: PLAN.md §1, §2.4 (pprof), §2.6, §3, §4 (Chunk 8), §7.13.

Containers are the two processes. The boxes inside each are Go packages in that process, not separate deployables.

## Mermaid

```mermaid
flowchart TD
  producer(["Event producers"])
  consumer(["Query consumers"])
  operator(["Operators"])

  subgraph gateway["cmd/gateway — ingestion edge proxy + rate-limiting router (Go process)"]
    pipeline["internal/pipeline<br/>sans-IO gateway core:<br/>parse → GCRA limit → Count-Min hot-key →<br/>bounded-load route → lock-free ring → forwarder"]
    coordinator["Query coordinator (/query)<br/>fan-out to shards, merge of partial<br/>aggregates and HLL registers"]
  end

  subgraph analytics["cmd/analytics — sharded OLAP query worker node (Go process, ×N shards)"]
    engine["internal/engine<br/>sans-IO shard engine:<br/>in-memory columns, Roaring inverted index,<br/>SwissTable GROUP BY, HyperLogLog, query execution"]
  end

  producer -->|"events — HTTP ingest<br/>(400 / 429 + Retry-After / 503)"| pipeline
  consumer -->|"queries — HTTP/JSON"| coordinator
  pipeline -->|"length-prefixed binary batches over TCP<br/>(internal/wire); shard acknowledges"| engine
  coordinator -->|"query messages (internal/wire);<br/>partial aggregates + HLL registers back"| engine
  operator -->|"/metrics, /hotkeys,<br/>pprof on -debug-addr"| gateway
  operator -->|"/metrics,<br/>pprof on -debug-addr"| analytics
```

## Text (draw-visual)

Data path (events and queries):

<!-- draw-visual: diagrams/02-container-data.mmd -->
```text
┌─────────────────────────────┐                ┌──────────────────────────┐
│       Event producers       │                │     Query consumers      │
└──────────────┬──────────────┘                └─────────────┬────────────┘
               │                                             │
          HTTP ingest                                    HTTP/JSON
               ▼                                             ▼
┌─────────────────────────────┐                ┌──────────────────────────┐
│  gateway: internal/pipeline │                │gateway: query coordinator│
└──────────────┬──────────────┘                └─────────────┬────────────┘
               │                                             │
          TCP batches                                        │
               ▼                                             │
┌─────────────────────────────┐                              │
│analytics xN: internal/engine│◄───────fan-out/merge─────────┘
└─────────────────────────────┘
```

Operator access:

<!-- draw-visual: diagrams/02-container-ops.mmd -->
```text
┌─────────┐                        ┌────────────────┐
│Operators├─metrics/hotkeys/pprof─►│  cmd/gateway   │
└────┬────┘                        └────────────────┘
     │
     │                             ┌────────────────┐
     └──────────metrics/pprof─────►│cmd/analytics xN│
                                   └────────────────┘
```

## Notes

- In `model.json` this view holds the two processes only, because c4-sdk does not allow components in a container view; the package boxes above are drawn in views 03 and 04 (see README, "How the model fits c4-sdk's rules").
- Hosting `/query` in the gateway is the plan's recommendation; the alternative is a coordinator role inside `cmd/analytics` (PLAN.md §9 Q6).
- Thread-per-core shape: one worker goroutine per shard ring, the only consumer of that ring and the only writer of that shard's state (PLAN.md §2.6).
- `net/http/pprof` is served on a separate `-debug-addr` listener in both processes, never on the ingest/query mux (PLAN.md §2.4).
