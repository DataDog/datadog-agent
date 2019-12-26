## package `listeners`

This package handles the network transmission for statsd protocols and returns
packets to be processed by the `dogstatsd` package.

### Packet

`Packet` is a statsd packet that might contain several statsd messages in it's
`Contents` field. If origin detection is supported and enabled, the `Origin`
field will hold the container id ready for tag resolution. If not, the field holds
an empty `string`.

### StatsdListener

`StatsdListener` is the common interface, currently implemented by:

- `UDPListener`: handles the historical UDP protocol,
- `UDSListener`: handles the host-local UDS protocol with optional origin detection,
see [the wiki](https://github.com/DataDog/datadog-agent/wiki/Unix-Domain-Sockets-support)
for more info.

### Origin Detection is Linux only

As our client implementations rely on Unix Credentials being added automatically
by the Linux kernel, this feature is Linux only for now. If needed, server and
client side could be updated and tested with other unices.
