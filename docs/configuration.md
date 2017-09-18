# Configuration

AthensDB can be configured using command-line options or using environment
variables prefixed with `ATHENSDB_`.

AthensDB aims to keep user-supplied configuration a minimum to keep complexity
to a minimum, both for the AthensDB codebase and for its users.

The 'immutable' configuration which is hard-coded is documented below.

## Configuration options

Command-line flag | Description | Default
- | - | -
`--data-directory` | The directory where data should be stored for the local node. Will be created if it does not exist. | `./data`
`--http-advertise-addr` | The host and port to advertise to peer nodes for HTTP communication | `localhost:9080`
`--http-bind-addr` | The host and port to bind to for HTTP communication | `localhost:9080`
`--peer-advertise-addr` | The host and port to advertise to peer nodes for gossip communication | `localhost:7946`
`--peer-bind-addr` | The host and port to bind to for gossip communication | `localhost:7946`
`--peers` | A list of peers to connect to to form a cluster; one peer per flag | No default
`--log-level` | Logging verbosity level; one of `debug`, `info`, `warning`, `error` or `fatal` | `info`

The same configuration options can be set as environment variables using the
`ATHENSDB_` prefix. For example, `ATHENSDB_LOG_LEVEL=debug` is equivalent to
`--log-level=debug`.

## Immutable constants

These values have been chosen as reasonable optimal values for most user
environments. If you think they should be changed, or made user-configurable,
please [raise a GitHub issue][].

Constant | Description | Value
- | - | -
Replication factor | How many copies of a time-series will stored across a cluster | 3

[raise a GitHub issue]: https://github.com/mattbostock/athensdb/issues/new
