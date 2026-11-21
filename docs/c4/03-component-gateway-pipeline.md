# C4 Level 3 — Component: gateway ingest pipeline (`internal/pipeline`)

> **TARGET architecture — designed 2026-11-21, awaiting Nick's buy-in.**

Scope: `internal/pipeline` inside `cmd/gateway`. Model id: `03-component-gateway-pipeline`.
Source: PLAN.md §3, §4 (Chunk 8A), §2.7, §7.3.

## Mermaid

```mermaid
flowchart TD
  producer(["Event producers"])
  subgraph pipeline["internal/pipeline — sans-IO gateway core"]
    parse["Line parse<br/>zero-alloc, pooled buffers"]
    limit["Per-tenant GCRA limit<br/>uses internal/ratelimit"]
    hotkey["Hot-key check and isolation<br/>uses internal/sketch (Count-Min)"]
    route["Bounded-load route<br/>uses internal/router"]
    ring["Per-shard ring<br/>uses internal/ringbuf"]
    forward["Batching forwarder<br/>uses internal/wire; re-dial with capped backoff"]
  end
  engine["cmd/analytics shard<br/>internal/engine"]

  producer -->|"events (HTTP ingest)"| parse
  parse -->|"event"| limit
  limit -->|"admitted"| hotkey
  limit -.->|"rejected: 429 + Retry-After"| producer
  hotkey -->|"not isolated"| route
  route -->|"assigned shard"| ring
  ring -->|"dequeue batch"| forward
  forward -->|"length-prefixed binary batches (TCP)"| engine
  engine -.->|"acknowledgement → Release"| route
```

## Text (draw-visual)

<!-- draw-visual: diagrams/03-component-gateway-pipeline.mmd -->
```text
┌──────────────────────────┐
│     Event producers      │◄──┐
└─────────────┬────────────┘   │
              │                │
           events              │
              ▼                │
┌──────────────────────────┐  429
│        Line parse        │   │
└─────────────┬────────────┘   │
              ▼                │
┌──────────────────────────┐   │
│  GCRA limit: ratelimit   ├───┘
└─────────────┬────────────┘
              │
          admitted
              ▼
┌──────────────────────────┐
│  Hot-key check: sketch   │
└─────────────┬────────────┘
              ▼
┌──────────────────────────┐
│Bounded-load route: router│◄──┐
└─────────────┬────────────┘   │
              ▼                │
┌──────────────────────────┐   │
│ Per-shard ring: ringbuf  │   │
└─────────────┬────────────┘   │
              ▼                │
┌──────────────────────────┐  ack
│ Batching forwarder: wire │   │
└─────────────┬────────────┘   │
              │                │
         TCP batches           │
              ▼                │
┌──────────────────────────┐   │
│ Analytics shard: engine  ├───┘
└──────────────────────────┘
```

## Notes

- "Load" for bounded loads is in-flight events per shard — assign on route, release on shard acknowledgement — as recommended and still open (PLAN.md §9 Q3).
- The forwarder re-dial is the only place retry is allowed (PLAN.md §2.7).
