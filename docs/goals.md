# Goals

## Durable storage

'Durable' in this context means that data retention can be measured in years
and data can survive the loss of a single server.

## Fault tolerance

By 'fault tolerance', it is meant that a single component failure (hard disk,
RAM module) or failure of the operating system (kernel, filesystem) should not
affect the availability of the time-series database. A combination of failures
may affect the availability of the database for reads (querying) or writes
(ingestion).

Fault tolerance, and by extension 'high availability', therefore demands that
the data must be duplicated across a cluster of physical (or logical, but
isolated) machines to help prevent the loss of any single machine, however
transiently, from affecting availability from the user's point of view.

## API compatibility with Prometheus

API compatibility with Prometheus ensures
that existing popular graphing tools such as Grafana can be used without
modification to query data.  Support for PromQL The PromQL language is
well-featured, supporting arithmetic and basic forecasting functions, as well
as being succinct for time-series queries which are its sole use case. The
features of PromQL include aggregations, quantiles, basic arithmetic (involving
two or more time series) and linear regression.

Supporting PromQL means that users can run the same queries that they run
against Prometheus without modifying their query. This avoids users having to
learn a separate query language for querying long-term metrics versus metrics
used for operational monitoring stored in Prometheus.

## Easy to deploy and operate, without reliance on external services

'Easy to operate' means that the database should be simple to install,
configure, and maintain while in operation.

Many existing durable and highly-available time-series databases rely on
external services to operate. The result is that the operator of these existing
databases needs to understand how to operate many moving parts, many of which
are generalised solutions that are not specific to time-series storage and
therefore have a large number of configuration options. For example, OpenTSDB
requires Apache HBase, which requires HDFS, which requires Zookeeper.

To reduce the operational burden, Timbala:

- keeps operator-specified configuration options to a minimum

- prefers libraries to external services

- allows for symmetrical deployments; each node in the cluster performs the
  same duties
