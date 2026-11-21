# C4 Level 1 — System Context

> **TARGET architecture — designed 2026-11-21, awaiting Nick's buy-in.**

Scope: the Analytics Edge Gateway platform (gateway + analytics engine). Model id: `01-system-context`.
Source: PLAN.md §1, §2.1, §2.8, §7.13.

## Mermaid

```mermaid
flowchart TD
  producer(["Event producers<br/>(tenant clients)"])
  consumer(["Query consumers"])
  operator(["Operators"])
  platform["Analytics Edge Gateway platform<br/>ingestion edge gateway + sharded real-time OLAP engine<br/>(two Go processes, in-memory storage only)"]
  sim["Workload simulation harness<br/>internal/sim (test system)"]
  e2e["E2E suite<br/>test/e2e (test system)"]

  producer -->|"sends events<br/>HTTP ingest"| platform
  consumer -->|"sends queries<br/>HTTP/JSON"| platform
  operator -->|"/metrics, /hotkeys,<br/>pprof on debug address"| platform
  sim -->|"drives seeded traffic in-process<br/>on the virtual clock (DST)"| platform
  e2e -->|"starts real processes, sends workloads,<br/>checks answers against an oracle"| platform
  e2e -->|"uses generators<br/>(pinned seed)"| sim

  classDef test stroke-dasharray: 5 5
  class sim,e2e test
```

## Text (draw-visual)

<!-- draw-visual: diagrams/01-system-context.mmd -->
```text
┌─────────────────────────┐                ┌────────────────────────────┐
│     Event producers     ├────events─────►│Gateway + analytics platform│
└─────────────────────────┘                └────────────────────────────┘
                                                          ▲
┌─────────────────────────┐                               │
│     Query consumers     ├────────────queries────────────┤
└─────────────────────────┘                               │
                                                          │
┌─────────────────────────┐                               │
│        Operators        ├─────────metrics/pprof─────────┤
└─────────────────────────┘                               │
                                                          │
┌─────────────────────────┐                               │
│Sim harness: internal/sim├────────────seeded─────────────┤
└─────────────────────────┘                               │
             ▲                                            │
        generators                                        │
             │                                            │
┌────────────┴────────────┐                               │
│   E2E suite: test/e2e   ├───────────processes───────────┘
└─────────────────────────┘
```

## Notes

- No upstream or downstream software systems: the plan proposes in-memory storage only, with no persistence or replication (PLAN.md §9 Q7).
- Ingest and query protocols are the plan's recommendations and remain open (PLAN.md §7.13, §9 Q6/Q7).
