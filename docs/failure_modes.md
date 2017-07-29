# Failure modes

AthensDB is designed to tolerate network and node failures. The availability of
the write path takes priority, potentially at the risk of losing some writes.

## Write guarantee

AthensDB uses a replication factor of 3, which is not user-configurable. This
means that under optimal operation, all data will be stored on three separate
nodes in the cluster.

Writes must be persisted to the write-ahead log (WAL) on disk on three distinct
nodes before returning a HTTP 200 'OK' status to the client sending the data.

If one node in the replica set (i.e., a node that is usually responsible for
storing a copy of the data being written) is not available, another node in the
cluster that does not already store a replica of that data can accept a copy of
the write on behalf of the unhealthy node and store it until the original node
becomes available again. This process is called 'Hinted handoff'.

The implication here is that any production cluster must have at least five
nodes to ensure good availability for the write path.

## Read guarantee

Read queries are sent to multiple nodes. A read will only be considered
successful if at least two nodes were able to return a result. 

If a node returns less data than other nodes in the cluster, or encounters an
error in the data, a read-repair process will be triggered to repair that copy
of the data.

## Network reliability

FIXME: memberlist settings

## Network partitions

AthensDB uses a gossip protocol based on SWIM to manage cluster membership.
