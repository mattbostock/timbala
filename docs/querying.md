# Querying

AthensDB does not provide a user interface for querying; you are expected to
use a third-party tool such as [Grafana][] to view your data.

## Grafana

You can use AthensDB as a datasource in [Grafana][] by adding a new
'Prometheus' datasource that points to AthensDB.

Follow the [Prometheus documentation for Grafana][].

[Grafana]: https://grafana.com/grafana
[Prometheus documentation for Grafana]: https://prometheus.io/docs/visualization/grafana/#creating-a-prometheus-data-source

## Query language

AthensDB supports PromQL, as used by Prometheus. Please see the [Prometheus
query documentation][] for details.

AthensDB makes no changes or additions to PromQL, so all queries that work in
Prometheus will work in AthensDB and vice-versa.

[Prometheus query documentation]: https://prometheus.io/docs/querying/basics/

## HTTP API

AthensDB has a HTTP API that is compatible with the [Prometheus API][]. See the
[Prometheus API][] documentation to learn how to use it.

[Prometheus API]: https://prometheus.io/docs/querying/api/

## 'Remote read' integration with Prometheus

Support is [planned][] for a '[remote read][]' endpoint, allowing Prometheus to
use AthensDB as a storage backend for queries.

[planned]: https://github.com/mattbostock/athensdb/issues/43
[remote read]: https://prometheus.io/docs/operating/configuration/#<remote_read>
