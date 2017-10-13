# Architecture

## Clustering

Membership of the cluster is determined using a gossip protocol. Timbala
uses Hashicorp's [Memberlist][] library, which uses a modified implementation
of the [Scalable Weakly-consistent Infection-style Process Group Membership][]
(SWIM) protocol.

Time-series data is partitioned across multiple nodes. The mapping between
time-series and nodes is determined by the [Jump consistent hashing algorithm][] that
partitions the data across nodes in the cluster.

Data is replicated across multiple distinct nodes as determined by the
[replication factor](configuration.md#immutable-constants).

[Scalable Weakly-consistent Infection-style Process Group Membership]: http://www.cs.cornell.edu/~asdas/research/dsn02-SWIM.pdf
[Memberlist]: https://godoc.org/github.com/hashicorp/memberlist
[Jump consistent hashing algorithm]: https://arxiv.org/abs/1406.2294

## Ingestion

Timbala implements the 'remote write' server API used by Prometheus.

To ingest metrics into the database, clients must make HTTP requests to the
'remote write' server API. A client can connect to any node to write data into
the database. The node uses consistent hashing to determine which nodes in
the hashring are responsible for storing the data and will make multiple
requests in parallel to each of the responsible nodes to send them the new
sample.

Timbala reuses the same remote write API as used by clients for internal
writes between nodes in the cluster. The node receiving the data samples
keeps persistent connections to the nodes responsible for storing the
time-series being ingested to ensure consistent throughput for frequent writes.

Metrics are append-only in the general case with the important
exception that out-of-order data samples will be accepted for specified grace
period to allow for recovery following a failure or network partition. The
duration of the grace period should be short enough to ensure that the data
block containing those samples can be rewritten in-memory and that the data
block containing those samples has not yet been persisted to disk.

Append-only ingestion allows for high compression of data samples, minimises
expensive data rewrites, avoids complicated consensus decisions across the
cluster, and should improve the predictability of system performance.

## Node-local storage

For node-local storage Timbala reuses as much of the Prometheus storage
engine as is feasible. The Prometheus development branch for version 2.0 uses a
new library 'tsdb' for the storage engine, written by one of the Prometheus
developers, Fabian Reinartz.

The tsdb library compresses floating point values to reduce the memory and disk
footprint of time-series data.

## Indexing

Individual time series are mapped to nodes using a hashring as described in
the 'Clustering' section above.

For mapping label names and label values to time series, Timbala uses the
indexes provided by the Prometheus tsdb library.

The indexes are decentralised and local to the node storing the data that
they index.

## Querying

Timbala re-uses the existing PromQL library used by Prometheus for the
query engine to avoid having to re-implement it.

When a client sends a request to the query API, the query is forwarded to
all nodes in the cluster; no centralised indexes will be used. Nodes that do
not store the data requested would respond to say as much and all nodes that
store the requested data will retrieve it and send it back to the node that
proxied the query. That node would then compare the responses from the different
nodes and return the most complete and most recent response back to the client.
