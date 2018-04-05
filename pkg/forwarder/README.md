## package `forwarder`

This package is responsible for sending payloads to the backend. Payloads can
come from different sources in different format, the forwarder will not inspect
them.

The forwarder can receive multiple domains with a list of API keys for each of
them. Every payload will be sent to every domain/API keys couple, this became a
`Transaction`. Transactions will be retried on error. The newest transactions
will be retried first. Transactions are consumed by `Workers` asynchronously.

### Usage
```go

KeysPerDomains := map[string][]string{
	"http://api.datadog.com": {"my_secret_key_1", "my_secret_key_2"},
	"http://debug.api.com":   {"secret_api"},
}

forwarder := forwarder.NewForwarder(KeysPerDomains)
forwarder.NumberOfWorkers = 1 // default 4
forwarder.Start()

// ...

payload := []byte("some payload")
forwarder.SubmitTimeseries(&payload)

// ...

forwarder.Stop()
```

### Configuration

There are several settings that influence the behavior of the forwarder.

#### Exponential backoff and circuit breaker settings

- `forwarder_backoff_factor` - This controls the overlap between consecutive
retry interval ranges. When set to `2`, there is a guarantee that there will
be no overlap. The overlap will asymptotically approach 50% the higher the
value is set. Values less then `2` are verboten as there will be range gaps.
Default: `2`
- `forwarder_backoff_base` - This controls the rate of exponential growth. Also,
you can calculate the start of the very first retry interval range by evaluating
the following expression: `forwarder_backoff_base / forwarder_backoff_factor * 2`.
Default: `2`
- `forwarder_backoff_max` - This is the maximum number of seconds to wait for
a retry. Default: `64`
- `forwarder_recovery_interval` - This controls how many retry interval ranges to
step down for an endpoint upon success. Increasing this should only be considered
when `forwarder_backoff_max` is particularly high. Default: `1`
- `forwarder_recovery_reset` - Whether or not a successful request should completely
clear an endpoint's error count. Default: `false`
