# Configuration

AthensDB is configured using [environment variables][] which are documented below.

AthensDB aims to keep user-supplied configuration a minimum to keep complexity
to a minimum, both for the AthensDB codebase and for its users.

The 'immutable' configuration which is hard-coded is documented below.

[environment variables]: https://en.wikipedia.org/wiki/Environment_variable

## Environment variables

Variable | Description | Default
- | - | -
`ADDR` | Address and port to expose HTTP server on | `localhost:9080`

## Immutable constants

These values have been chosen as reasonable optimal values for most user
environments. If you think they should be changed, or made user-configurable,
please [raise a GitHub issue][].

Constant | Description | Value
- | - | -
Replication factor | How many copies of a time-series will stored across a cluster | 3

[raise a GitHub issue]: https://github.com/mattbostock/athensdb/issues/new
