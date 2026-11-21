# C4 Level 3 — Component: inverted index + Roaring bitmaps (`internal/index`)

> **TARGET architecture — designed 2026-11-21, awaiting Nick's buy-in.**

Scope: `internal/index`. Model id: `09-component-index`.
Source: PLAN.md §3, §4 (Chunk 5), §7.5.

## Mermaid

```mermaid
flowchart TD
  caller["internal/engine<br/>inverted index over tags"]
  subgraph index["internal/index"]
    inverted["Inverted<br/>term → posting bitmap,<br/>boolean query evaluation (AND / OR / NOT)"]
    bitmap["Bitmap<br/>high 16 bits select a container;<br/>Add / Remove / Contains / And / Or / AndNot"]
    array["Array container<br/>sorted []uint16, ≤ 4,096 elements"]
    bitset["Bitset container<br/>[1024]uint64, ≥ 4,097 elements"]
  end

  caller -->|"index tags, query"| inverted
  inverted -->|"posting bitmap per term"| bitmap
  bitmap -->|"sparse chunk"| array
  bitmap -->|"dense chunk"| bitset
  array -->|"promote at 4,097 / demote at 4,096"| bitset
```

## Text (draw-visual)

<!-- draw-visual: diagrams/09-component-index.mmd -->
```text
┌────────────────────────┐
│   Engine: tag index    │
└────────────┬───────────┘
             ▼
┌────────────────────────┐
│Inverted: term to bitmap│
└────────────┬───────────┘
             ▼
┌────────────────────────┐
│ Bitmap: 16-bit chunks  ├──────────dense───────────┐
└────────────┬───────────┘                          │
             │                                      │
          sparse                                    │
             ▼                                      ▼
┌────────────────────────┐                 ┌────────────────┐
│    Array container     ├─promote/demote─►│Bitset container│
└────────────────────────┘                 └────────────────┘
```

## Notes

- Run containers and serialisation are not planned (PLAN.md §7.5, §8).
