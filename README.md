[![Build Status](https://travis-ci.org/mattbostock/timbala.svg?branch=master)](https://travis-ci.org/mattbostock/timbala)


## Project status

Timbala is in a very early stage of development and is not yet production-ready. **Please do not use it yet for any data that you care about.**

There are several known issues that prevent any serious use of Timbala at this stage; please see the [MVP milestone](https://github.com/mattbostock/timbala/milestone/2) for details.

* * *

<img src="docs/images/timbala_logo_horizontal.svg" alt="Timbala logo" width="50%" style="margin: 2em 0;">

Timbala is a distributed, fault-tolerant time-series database intended to
provide durable long-term storage for multi-dimensional metrics.

It is designed to integrate easily with [Prometheus][], supports PromQL and is
API-compatible with Prometheus, but can be used standalone.

Data stored in Timbala can be visualised using [Grafana][] by
configuring a Prometheus data source pointing to Timbala.

[Prometheus]: https://prometheus.io/
[Grafana]: http://grafana.org/

## Design goals

### Ease of operation

- one server binary
- no external dependencies
- all nodes have the same role
- all nodes can serve read and write requests

### Fault-tolerant

- no single points of failure
- data is replicated and sharded across multiple nodes
- planned features for read repair and active anti-entropy

### Highly available

- high write throughput and availability
