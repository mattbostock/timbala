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
