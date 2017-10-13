# Monitoring

## Metrics

Timbala exposes metrics in the [Prometheus format][] over HTTP on `/metrics`.

You can use these metrics to define alerts for monitoring or create operational
dashboards.

[Prometheus format]: https://prometheus.io/docs/instrumenting/exposition_formats/

## Logging

Timbala logs to stdout, in JSON format.

## Tracing

Support for OpenTracing is [planned][], allowing for easier performance
debugging.

[planned]: https://github.com/mattbostock/timbala/issues/42
