# Requirements

## Operating system

Timbala currently supports Linux only. Other architectures or configurations
may work; contributions are welcome to support more architectures.

## Operational dependencies

Timbala has no operational dependencies; node discovery and cluster coordination is handled by Timbala internally.

You will need to use a third-party tool such as Grafana to visualise your metrics stored in Timbala.

## Cluster size

The minimum recommended size for a production cluster is five nodes in order to ensure good availability. Testing or staging clusters can be smaller.

The number of nodes in the cluster should be a prime number.

## Disk space

Timbala stores data very efficiently thanks to the excellent [tsdb][] library
used for storage, which uses [Facebook's Gorilla][] compression algorithm.

The amount of disk space required will depend on how many metrics you store and
the frequency of data points. 50GB would be a good starting point for new
users.

[tsdb]: https://github.com/prometheus/tsdb
[Facebook's Gorilla]: http://www.vldb.org/pvldb/vol8/p1816-teller.pdf

## Memory

Timbala will make use of any available memory on the system and query time will
be improved with additional memory, so the more memory the better.

Small data sets will not require much memory and a Timbala cluster easily can
be run on a laptop for testing.
