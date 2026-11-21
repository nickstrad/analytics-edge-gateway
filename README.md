# High-Throughput Analytics & Edge Gateway

A high-throughput event ingestion edge gateway and real-time OLAP
analytics engine (Mini-ClickHouse + Envoy) in Go from scratch, built
around seven CPU-, memory-, and routing-focused primitives:
lock-free ring buffers, Token Bucket / GCRA rate limiting, consistent
hashing with bounded loads, Count-Min Sketch, inverted indexes with
Roaring Bitmaps, a SwissTable-style hash map, and HyperLogLog —
designed testing-first, with deterministic simulation testing,
property tests, native fuzzing, allocation and race gates guiding
the build.

Status: planning phase. The full plan (PLAN.md) is being prepared; the
architecture write-ups will be published on GitHub Pages as each
primitive lands. Spec: docs/project-source.md.
