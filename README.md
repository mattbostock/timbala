# Proof of concept - do not use in Production

This project is an early alpha, written for education purposes. Please don't
consider putting it into Production.

# Athens

[![Build Status](https://travis-ci.com/mattbostock/athens.svg?token=EhqoSPmXWFAXy2qpEaqr&branch=master)](https://travis-ci.com/mattbostock/athens)

Athens is a distributed, fault-tolerant time series database that supports PromQL.

It is intended to provide durable long-term storage for multi-dimensional time
series data.

## Why?

[Prometheus][] users often request durable storage that supports the PromQL
query language.

[Prometheus][] is a monitoring system that relies on time series. Designed
foremost for reliability, clustering and durable storage are explicit project
non-goals.

Athens is intended for use as a secondary, durable, data store for time series
data. It supports PromQL and the Prometheus query API, but does not depend on
Prometheus.

Data stored in Athens can be visualised using [Grafana][].

[Prometheus]: https://prometheus.io/
[Grafana]: http://grafana.org/

## High-level design goals

- Ease of operation: one server binary and all nodes are treated equally
- Failure-tolerant: no single points of failure; data is replicated across multiple nodes
- Highly available: writes are prioritised over reads and availability is valued over consistency
- Optimised for modern hardware: expects SSDs, takes advantage of multiple CPU cores and [NUMA][]

[NUMA]: https://www.kernel.org/doc/Documentation/vm/numa

## Availability/consistency tradeoffs (WIP)

- Consistency; conflicting sample values are not handled gracefully, by design
  - Each sample is expected to come from a single source of truth
  - ...this is fine for operational timeseries
- Append only?
  - Good points:
    - Should simplify availability/consistency model
    - Should be more performant
    - Well-suited to polled data (i.e. from Prometheus)
    - Easy to use varbit encoding
  - Bad points:
    - We need to back/frontfill when replicating data anyway (though ideally in blocks)
    - Limits use for historical purposes, research
    - Not good for non-realtime uses (e.g. buffered, asynchronous data)
      - Kafka queue, replaying data
  - Maybe make it dependent on write path
    - E.g. Prometheus remote write only supports append-only but another interface for writing chunks
      - clumsy, not user-friendly for non-Prometheus producers
    - Avoids question of performance, availability tradeoff

- Track duplicate/out-of-order values?
  - If append only, yes
- Conflicting values for same timestamp
  - Track which producers conflicted
  - Take first value only
- Chunk deadline (chunk closes after n minutes)
  - Avoid digging up archived queries
  - Do we repair by sample or by chunk? How do we know if a chunk is a true representation, i.e. has all known samples?
    - Merkle tree?
    - Re-encode varbit? Limit chunks by time instead of bytes so we can re-encode without affecting neighbouring chunks (i.e. overflowing)?
      - Could have overflow chunks
        - Wasteful, complex
      - Chunk by time would mean that data sampled infrequently would either have very small chunks, or wait a long time before being persisted
        - But easy to reason about persistence
    - Re-encoding could be expensive
      - Buffer repairs
      - Avoid re-encoding if all nodes agree and we can contact all nodes; overwrite whole chunk
      - Deadline for repairing individual samples?
        - Node could be down hours/days with valuable samples
- Lock-free writes possible?
  - Locks needed for varbit encoding
  - We can keep all values in memory then lock when persisting to disk
    - RocksDB: one db for log (with full values), one for long-term varbit data
    - Use deadline
    - Report conflicting samples at persist time
- Memory allocation: allocate n series max

- Options:
  - Append-only
    - Focus on realtime reporting
    - No update except for read-repair after deadline
    - Avoids expensive updates
  - In-place update
    - Greater flexibility, especially for queues
    - Can be used for read repair also
    - More wasteful with disk space and memory, or more CPU intensive
      - Indexing/hashing more important
        - Hash on hour, index on minute?
        - Can we have fixed-width encoding that's reasonably space-efficient?
          - Could use double-delta
          - More predictable memory/disk usage; easier to hash/index
            - Though we still don't know sample/ingest rate
        - Just store sample data without context and use indices for timing
          - Adjust per-series index granularity based on ingest/sample frequency using weighted average (with cap)?
            - i.e. finer-grained indices for more regular data
      - Log-structured merge might help
      - Could optimise cost of re-encoding (could write in assembly though less portable)
      - Could instrument how often blocks are re-written (different metrics for different reasons, e.g. append versus repair)

- Consistent metric UIDs/fingerprints across cluster?
  - Want to avoid local UIDs, since we'd need to rewrite when replicating (or use a lookup table, which would be overly complex and expensive)
  - Using full name is costly in terms of memory/disk, but simple
    - Cost would be:
      - Full name in each series in memory; full name on each chunk on disk (depending on what storage schema is)
    - Lossless compression on name? Probably wouldn't see much gain
  - Waiting for consensus on UIDs could mean losing writes
  - Could fingerprint, then rewrite fingerprint if collision is found when replicating
    - Means we need to deal with multiple fingerprints at any one time when answering queries
  - Use full name for now, can iterate later
