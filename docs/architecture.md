# Architecture

## Clustering

Membership of the cluster will be determined using a gossip protocol. Timbala
will use Hashicorp's Memberlist library, which uses a modified implementation
of the Scalable Weakly-consistent Infection-style Process Group Membership
(SWIM) protocol. Cluster membership data is the only shared state in the
cluster.

Time-series data will be partitioned across multiple nodes. The mapping between
time-series and nodes will be determined by a consistent hashing function that
partitions the data across a number of virtual nodes (vnodes) in a hash ring.
Data will be replicated across a number of virtual nodes, as determined by the
replication factor.

## Ingestion

Timbala will implement the 'remote write' server API used by Prometheus.

To ingest metrics into the database, clients must make HTTP requests to the
'remote write' server API. A client can connect to any node to write data into
the database. The node will use consistent hashing to determine which vnodes in
the hashring are responsible for storing the data and will make multiple
requests in parallel to each of the responsible nodes to send them the new
sample.

The node receiving the data will only confirm acceptance of the sample(s) once
a at least one of the nodes that store those samples have confirmed acceptance.
If no nodes are can be reached, the samples are rejected.

Timbala will reuse the same remote write API as used by clients for internal
writes between nodes in the cluster. The node receiving the data samples will
keep persistent connections to the nodes responsible for storing the
time-series being ingested to ensure consistent throughput for frequent writes.

Time-series will be append-only in the general case with the important
exception that out-of-order data samples will be accepted for specified grace
period to allow for recovery following a failure or network partition. The
duration of the grace period should be short enough to ensure that the data
block containing those samples can be rewritten in-memory and that the data
block containing those samples has not yet been persisted to disk.

Append-only ingestion allows for high compression of data samples, minimises
expensive data rewrites, avoids complicated consensus decisions across the
cluster, and should improve the predictability of system performance.

## Node-local storage

For node-local storage Timbala will reuse as much of the Prometheus storage
engine as is feasible. The Prometheus development branch for version 2.0 uses a
new library 'tsdb' for the storage engine, written by one of the Prometheus
developers, Fabian Reinartz. Although the new library is still very new and is
alpha-quality, the API for the new library is much simpler than the previous
one, and should be more amenable to managing data across multiple nodes.

The tsdb library compresses floating point values to reduce the memory and disk
footprint of time-series data.

## Indexing

Individual time series will be mapped to nodes using a hashring as described in
the 'Clustering' section above.

For mapping label names and label values to time series, Timbala will use the
indexes provided by the Prometheus tsdb library.

The indexes will be decentralised and local to the node storing the data that
they index. The design of the read and write paths negates the need for a
centralised index.

## Querying

Timbala will re-use the existing PromQL library used by Prometheus for the
query engine to avoid having to re-implement it.

When a client sends a request to the query API, the query will be forwarded to
all nodes in the cluster; no centralised indexes will be used. Nodes that do
not store the data requested will respond to say as much and all nodes that
store the requested data will retrieve it and send it back to the node that
proxied the query. That node will then compare the responses from the different
nodes and return the most complete and most recent response back to the client.
