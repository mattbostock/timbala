# Security

## Dependencies

Timbala has no operational dependencies.

Timbala uses the [Go programming language](https://golang.org/).

## Encryption and authentication

It is assumed that Timbala will run in a trusted environment.

Communications between nodes and from nodes to clients is unauthenticated and
unencrypted. Please see [this GitHub
issue](https://github.com/mattbostock/timbala/issues/44) for more details.

You should use a reverse HTTP proxy if you wish to add [Transport Layer
Encryption][] or add authentication to Timbala's HTTP APIs. One way to do so would be to use
a service mesh such as [Istio][].

[Istio]: https://istio.io/
[Transport Layer Encryption]: https://en.wikipedia.org/wiki/Transport_Layer_Security

## Multi-user or multi-tenant support

Timbala has no concept of users or tenants; a request to the Timbala API can read and
write to all data.

See the [GitHub issue for multi-tenant support](https://github.com/mattbostock/timbala/issues/45).
