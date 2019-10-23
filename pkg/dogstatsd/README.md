## package `dogstatsd`

This package is responsible for receiving metrics from external software over
UDP. Every package has to follow the Dogstatsd format:
http://docs.datadoghq.com/guides/dogstatsd/.

Metrics will be sent to the aggregator just like regular metrics from checks.
This mean that aggregator and forwarder configuration will also inpact
Dogstatsd.

Usage example:
```go
// you must first initialize the aggregator, see aggregator.InitAggregator

// This will return an already running statd server ready to receive metrics
statsd, err := dogstatsd.NewServer(aggregatorInstance.GetBufferedChannels())

// ...

statsd.Stop()
```

### Packet

`Packet` is a statsd packet that might contain several statsd messages in it's
`Contents` field. If origin detection is supported and enabled, the `Origin`
field will hold the container id ready for tag resolution. If not, the field holds
an empty `string`.

### StatsdListener

`StatsdListener` is the common interface, currently implemented by:

- `UDPListener`: handles the historical UDP protocol,
- `UDSListener`: handles the host-local UDS protocol with optional origin detection,
see [https://github.com/DataDog/datadog-agent/wiki/Unix-Domain-Sockets-support](the wiki)
for more info.

### Origin Detection is Linux only

As our client implementations rely on Unix Credentials being added automatically
by the Linux kernel, this feature is Linux only for now. If needed, server and
client side could be updated and tested with other unices.
