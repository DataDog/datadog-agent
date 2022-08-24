## package `dogstatsd`

This package is responsible for receiving metrics from external software over
UDP or UDS. Every package has to follow the Dogstatsd format:
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

Dogstatsd implementation documentation (packets.Buffer, StringInterner, ...) is available
in `docs/dogstatsd/internals.md`.

Details on existing Dogstatsd internals tuning fields are available in `docs/dogstatsd/configuration.md`.

### [Experimental] Dogstatsd protocol 1.1

This feature is experimental for now and could change or be remove in futur release.

Starting with agent 7.25.0/6.25.0 Dogstatsd datagram can contain multiple values using the `:` delimiter.

For example, this payload contains 3 values (`1.5`, `20`, and `30`) for the metric `my_metric`:
```
my_metric:1.5:20:30|h|#tag1,tag2
```

All metric types except `set` support this, since `:` could be in the value of a
set. Sets are now being aggregated on the client side, so this is not an issue.

Most official Dogstatsd clients now support client-side aggregation for metrics
type outside histograms and distributions. This evolution in the protocol allows
clients to buffer histogram and distribution values and send them in fewer
payload to the agent (providing a behavior close to client-side aggregation for
those types).
