# C4 Level 3 — Component: SwissTable hash map (`internal/hashmap`)

> **TARGET architecture — designed 2026-11-21, awaiting Nick's buy-in.**

Scope: `internal/hashmap`. Model id: `10-component-hashmap`.
Source: PLAN.md §3, §4 (Chunk 6), §7.6.

## Mermaid

```mermaid
flowchart TD
  caller["internal/engine<br/>SwissTable GROUP BY"]
  subgraph hashmap["internal/hashmap"]
    harness["GROUP BY aggregation harness<br/>(used by the benchmarks)"]
    map["Map[K, V]<br/>Put / Get / Delete, triangular group probing,<br/>tombstones + in-place rehash, doubling resize"]
    hasher["Explicit hasher<br/>per-table seed; H1 selects group, 7-bit H2"]
    swar["SWAR group match<br/>two uint64 words per 16-slot group"]
    ctrl["Control bytes<br/>empty 0x80, deleted 0xFE, full = H2"]
    slots["Slot array<br/>flat contiguous key/value slots"]
  end

  caller -->|"Put / Get"| map
  harness -->|"aggregates into"| map
  map -->|"H1, H2"| hasher
  map -->|"probe group"| swar
  swar -->|"match H2 / empty"| ctrl
  map -->|"compare key, read/write value"| slots
```

## Text (draw-visual)

<!-- draw-visual: diagrams/10-component-hashmap.mmd -->
```text
┌────────────────┐    ┌───────────────────┐
│Engine: GROUP BY│    │Aggregation harness│
└────────┬───────┘    └─────────┬─────────┘
         │                      │
      Put/Get                   │
         ▼                      │
┌────────────────┐              │
│    Map K,V     │◄─────────────┘
└────────┬───────┘
         │
         ├──────────────────────┬────────────────────┐
         │                      │                    │
       probe                  H1/H2              key/value
         │                      │                    │
         ▼                      ▼                    ▼
┌────────────────┐    ┌───────────────────┐    ┌──────────┐
│SWAR group match│    │  Explicit hasher  │    │Slot array│
└────────┬───────┘    └───────────────────┘    └──────────┘
         │
       match
         ▼
┌────────────────┐
│ Control bytes  │
└────────────────┘
```

## Notes

- Max load 7/8; pure Go, no assembly, no `unsafe` (PLAN.md §3, §7.6).
