## package `forwarder`

This package is responsible for sending payloads to the backend. Payloads can
come from different sources in different format, the forwarder will not inspect
them.

The forwarder can receive multiple domains with a list of API keys for each of
them. Every payload will be sent to every domain/API keys couple, this became a
`Transaction`. Transactions will be retried on error. The newest transactions
will be retried first. Transactions are consumed by `Workers` asynchronously.

Usage example:
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
