# Design assumptions

## Properties of the read and write paths

The number of writes occurring in a long-term metrics store are constant and
frequent. The availability and latency of the write path is critical; if the
database is unable to accept new data, data could be lost, and if the write
path is too slow, the database will be unable to keep up with a high throughput
of data samples.

The read path is likely much less frequently accessed, usually by humans that
wish to view the metrics. Latency requirements are more relaxed for the
read-path; queries should be answered as quickly as possible but query
latencies can be measured in seconds rather than in milliseconds, as is the
case for writes.

The availability requirement for the read path is also more relaxed; we can
afford for a human to have to retry a query.

## Data immutability

The data samples are stored as raw values (not aggregated), which means that
the data samples will not change. Therefore we should expect only data samples
generated recently will be written and historic data should be considered
immutable.

The exception is data that was received late, perhaps due to delays
transporting the data across the network. We must cater for out-of-order
samples, for example when a data sample for a given time-series is received two
minutes later than a more recent data sample for the same series.

If late-arriving samples are expected to be a significant factor in a
production environment, the operator of this database should either use
client-side buffering or a distributed queue such as Apache Kafka to buffer the
samples.

## Expected throughput and storage capacity

Data samples will be stored as 64-bit floating point numbers, compressed using
[Facebook's Gorilla][] algorithm, and should use approximately 1.37 bytes per data
sample on average, depending on the exact data ingested.

A 5-node cluster on modern, high-specification, commodity hardware in a 'bare
metal' configuration (SSDs in RAID 10, 256GB RAM and 24 physical cores on an
amd64 architecture) should support at minimum:

- 40 million active time series
- an ingestion rate of 1 million samples per second
- 5 queries per second

[Facebook's Gorilla]: www.vldb.org/pvldb/vol8/p1816-teller.pdf

## Availability

Availability for the write path, i.e. ingestion of data, is a first-class
design goal in Timbala.

Availability for the read path, i.e. querying data, is a secondary design goal.
A human can retry a query later, whereas the database must always be able to
ingest new data.
