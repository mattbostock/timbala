# Security

## Dependencies

AthensDB has no external dependencies.

AthensDB uses the [Go programming language](https://golang.org/), which has a
good security record.

## Encryption and authentication

It is assumed that AthensDB will run in a trusted environment.

Communications between nodes and from nodes to clients is unauthenticated and
unencrypted. Please see [this GitHub
issue](https://github.com/mattbostock/athensdb/issues/44) for more details.

You should use a reverse HTTP proxy if you wish to add [Transport Layer
Encryption][] or add authentication to AthensDB's HTTP APIs.

[Transport Layer Encryption]: https://en.wikipedia.org/wiki/Transport_Layer_Security

## Multi-user or multi-tenant support

AthensDB has no concept of users or tenants; a request to the AthensDB API can read and
write to all data.

See the [GitHub issue for multi-tenant
support](https://github.com/mattbostock/athensdb/issues/45).
