# Use cases

Timbala is intended for organisations wishing to store small to medium volumes of metrics
either on-premise or in the cloud with a minimum of operational maintenance.

We will define 'medium' as 1 million data points per second across 40 million
unique metrics, which Timbala aims to support once in beta.

Timbala is not currently intended for very large volumes of data such as SaaS
products storing data on behalf of others with multiple tenants, such users may
wish to consider [Cortex][] instead.

Timbala is not designed for accounting, billing, or financial applications
where precision and consistency is of critical importance.

[Cortex]: https://github.com/weaveworks/cortex
