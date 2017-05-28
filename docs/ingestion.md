# Ingestion

Ingestion, or writes, are supported using the write HTTP API, which uses
Protobufs and Snappy compression. HTTP/2 is also supported.

The write API is also compatible with the Prometheus '[remote write][]'
specification.

[remote write]: https://prometheus.io/docs/operating/configuration/#<remote_write>
