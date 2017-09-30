# Glossary

## Replication factor

How many copies of a time-series will be stored across a cluster.

For example, if the replication factor is `3`, each data point for each
time-series should be stored three times, in case one of the copies becomes
corrupt or is lost due to a node failure.

## Time-series

A time-series, or a 'metric', measures the value of one singular thing over time.

Specifically, a time-series is the unique combination of a metric name plus a
label name plus a label value.

For example, each of the below would be considered as three distinct
time-series:

    http_requests_total{code="404"}
    http_requests_total{code="200"}
    timbala_build_info{version="0.0.1"}
