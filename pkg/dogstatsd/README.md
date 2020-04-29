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

Dogstatsd implementation documentation (PacketsBuffer, StringInterner, ...) is available
in `docs/dogstatsd/internals.md`.

Details on existing Dogstatsd internals tuning fields are available in `docs/dogstatsd/configuration.md`.