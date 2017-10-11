# Design assumptions

## Properties of the read and write paths

The number of writes occurring will be constant and frequent. The availability
and latency of the write path is critical; if the database is unable to accept
new data, data could be lost, and if the write path is too slow, the database
will be unable to keep up with a high throughput of data samples.

The read path will be much less frequently accessed, usually by humans that
wish to view the metrics. Latency requirements are more relaxed for the
read-path; queries should be answered as quickly as possible but query
latencies can be measured in seconds rather than in milliseconds, as is the
case for writes.

The availability requirement for the read path is also more relaxed; we can
afford for a human to have to retry a query. Consistency is the most important
feature of the read path; the results returned should be the same irrespective
of the node we query.

## Data immutability

The data samples stored as raw values (not aggregated), which means that the
data samples will not change. Therefore we should expect only data samples
generated recently will be written and historic data should be considered
immutable.

The exception is data that was received late, perhaps due to delays
transporting the data across the network. We must cater for out-of-order
samples, for example when a data sample for a given time-series is received two
minutes later than a more recent data sample for the same series.

To allow for out-of-order sample ingestion, all new data samples will be held
in-memory only for a minimum period that we will call the 'staleness delta'
(using a term borrowed from Prometheus). The staleness delta is the difference
between the current time and the timestamp of the data sample to be ingested.
The tradeoff is between limiting memory used to hold the samples, limiting the
amount of data loss suffered if a single node fails while samples are held in
memory and avoiding losing late-arriving data samples.

If late-arriving samples are expected to be a significant factor in a
production environment, the operator of this database should either use
client-side buffering or a distributed queue such as Apache Kafka to buffer the
samples.

## Expected throughput and storage capacity

Data samples will be stored as 64-bit floating point numbers, compressed using
Facebook's Gorilla algorithm, and should use approximately 1.37 bytes per data
sample on average, depending on the exact data ingested.

A 5-node cluster on modern, high-specification, commodity hardware in a 'bare
metal' configuration (SSDs in RAID 10, 256GB RAM and 24 physical cores on an
amd64 architecture) should support at minimum:

- 40 million active time series
- an ingestion rate of 1 million samples per second
- 5 queries per second

## Availability

Availability for the write path, i.e. ingestion of data, is a first-class
design goal in Timbala.

Availability for the read path, i.e. querying data, is a secondary design goal.
A human can retry a query later, whereas the database must always be able to
ingest new data.
