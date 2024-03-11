## package `forwarder`

This package is responsible for sending payloads to the backend. Payloads can
come from different sources in different formats, the forwarder will not inspect
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
forwarder.NumberOfWorkers = 1 // default: config.GetInt("forwarder_num_workers")
forwarder.Start()

// ...

payload1 := []byte("some payload")
payload2 := []byte("another payload")
forwarder.SubmitSeries(Payloads{&payload1, &payload2}, ...)

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
step down for an endpoint upon success. Default: `2`
- `forwarder_recovery_reset` - Whether or not a successful request should completely
clear an endpoint's error count. Default: `false`

### Internal

The forwarder is composed of multiple parts:

#### DefaultForwarder

`DefaultForwarder` it the default implementation of the `Forwarder` interface
(and the only one for now). This class is in charge of receiving payloads,
creating the HTTP transactions and distributing them among every
`domainForwarder`.

#### domainForwarder

The agent can be configured to send the same payload to multiple destinations.
Each destination (or domain) can be configured with 1 or more API keys. Every
payload will be sent to each domain/API key pair.

A `domainForwarder` is in charge of sending payloads to one domain. This avoids
slowing down every domain when one is down/slow. Each `domainForwarder` will
have a number of dedicated `Worker` to process `Transaction`. We process new
transactions first and then (when the workers have time) we retry the erroneous
ones (newest transactions are retried first).

We start dropping transactions (oldest first) when the sum
of all the payload sizes is bigger than `forwarder_retry_queue_payloads_max_size` 
(see the agent configuration).

Disclaimer: using multiple API keys with the **Datadog** backend will multiply
your billing ! Most customers will only use one API key.

#### Worker

A `Worker` processes transactions coming from 2 queues: `HighPrio` and `LowPrio`.
New transactions are sent to the `HighPrio` queue and the ones to retry are
sent to `LowPrio`. A `Worker` is dedicated to on domain (ie: domainForwarder).

#### blockedEndpoints (or exponential backoff)

When a transaction fails to be sent to a backend we blacklist that particular
endpoints for some time to avoid flooding an unavailable endpoint (the
transactions will be retried later). A blacklist is specific to one endpoint on
one domain (ie: "http(s)://<domain>/<endpoint>"). The blacklist time will grow,
up to a maximum, as more and more errors are encountered for that endpoint and
is gradually cleared when a transaction is successful. The blacklist is shared
by all workers.

#### Transaction

A `HTTPTransaction` contains every information about a payload and how/where to
send it. On failure a transaction will be retried later (see blockedEndpoints).
