> **TL;DR:** Provides an embeddable, thread-safe retry mechanism with configurable one-shot, fixed-count, and exponential-backoff strategies, plus structured error types for distinguishing temporary from permanent failures.

# pkg/util/retry

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/retry`

## Purpose

`retry` provides a reusable, thread-safe retry mechanism that can be embedded
in any struct needing retry-on-error behaviour. It handles three strategies
(one-shot, fixed count, exponential backoff), enforces per-attempt delay
windows, and exposes a structured error type so callers can distinguish
temporary failures from permanent ones without string matching.

The package is intentionally non-blocking: `TriggerRetry` returns immediately
if the delay between attempts has not elapsed yet, so the caller decides when
to poll again.

## Key elements

### Key types

| Type | Description |
|------|-------------|
| `Retrier` | Embeddable struct that holds retry state. Must be initialised with `SetupRetrier` before use. |
| `Config` | Configuration passed to `SetupRetrier`. Selects a strategy and its parameters. |
| `Error` | Custom error returned by retrier methods. Carries `RetryStatus`, the originating error, and the last attempt error. |
| `Status` | Enum describing the current retrier state (`NeedSetup`, `Idle`, `OK`, `FailWillRetry`, `PermaFail`). |
| `Strategy` | Enum selecting retry behaviour (`OneTry`, `RetryCount`, `Backoff`, `JustTesting`). |

### Configuration and build flags

| Field | Strategy | Description |
|-------|----------|-------------|
| `Name` | all | Human-readable resource name used in error messages. |
| `AttemptMethod` | all | `func() error` called on each try. |
| `Strategy` | all | One of the `Strategy` constants. |
| `RetryCount` | `RetryCount` | Maximum number of attempts. |
| `RetryDelay` | `RetryCount` | Fixed delay between attempts. |
| `InitialRetryDelay` | `Backoff` | Starting delay; doubled on each failure. |
| `MaxRetryDelay` | `Backoff` | Upper bound on the computed backoff delay. |

### Key functions

| Method | Description |
|--------|-------------|
| `SetupRetrier(*Config) error` | Validates and applies configuration. Must be called once before any other method. |
| `TriggerRetry() *Error` | Calls `AttemptMethod` if the next-try window has passed. Returns `nil` on success or if status is already `OK`. |
| `RetryStatus() Status` | Returns the current status without attempting a retry. |
| `NextRetry() time.Time` | Returns when the next attempt is allowed. |
| `LastError() *Error` | Returns a wrapped version of the last attempt error. |

### Key functions (error helpers)

| Function | Description |
|----------|-------------|
| `IsRetryError(error) (bool, *Error)` | Type-asserts an `error` to `*Error`. |
| `IsErrPermaFail(error) bool` | Returns `true` when the error status is `PermaFail`. |
| `IsErrWillRetry(error) bool` | Returns `true` when the error status is `FailWillRetry`. |

## Usage

### Embedding the retrier

The typical pattern is to embed `retry.Retrier` in a struct and call
`SetupRetrier` during initialisation, passing the method that performs the
actual connection or setup work as `AttemptMethod`. Consumers then call
`TriggerRetry` on each use and inspect the returned `*Error`:

```go
// From pkg/util/containerd/containerd_util.go
containerdUtil.initRetry.SetupRetrier(&retry.Config{
    Name:              "containerdutil",
    AttemptMethod:     containerdUtil.connect,
    Strategy:          retry.Backoff,
    InitialRetryDelay: 1 * time.Second,
    MaxRetryDelay:     5 * time.Minute,
})

// On each call that needs the connection:
func (c *ContainerdUtil) CheckConnectivity() *retry.Error {
    return c.initRetry.TriggerRetry()
}
```

The same pattern is used in:
- `pkg/util/docker` — Docker daemon connection
- `pkg/util/clusteragent` — Cluster Agent client connection
- `pkg/util/cloudproviders/cloudfoundry` — Garden API connection
- `pkg/collector/corechecks/cluster/ksm` — Kubernetes State Metrics client

### Distinguishing failure types

```go
err := util.CheckConnectivity()
if err != nil {
    if retry.IsErrPermaFail(err) {
        // No point retrying; log and return.
        return fmt.Errorf("permanent failure: %w", err)
    }
    // FailWillRetry — caller can back off and try again.
    log.Debugf("not ready yet: %s", err)
}
```

### Strategies at a glance

| Strategy | Behaviour | Required fields |
|----------|-----------|-----------------|
| `OneTry` | One attempt, then `PermaFail`. | none |
| `RetryCount` | Fixed number of attempts with a constant delay. | `RetryCount`, `RetryDelay` |
| `Backoff` | Unlimited retries; delay doubles each time up to `MaxRetryDelay`. | `InitialRetryDelay`, `MaxRetryDelay` |
| `JustTesting` | Immediately sets status to `OK` without calling `AttemptMethod`. | none |

## Relationship to pkg/util/backoff

`pkg/util/retry` and [`pkg/util/backoff`](backoff.md) solve related but distinct problems:

| | `pkg/util/retry` | `pkg/util/backoff` |
|---|---|---|
| **Unit** | Embeddable struct with lifecycle state (`Idle` → `FailWillRetry` → `PermaFail` → `OK`) | Stateless `Policy` object; caller owns the error counter |
| **Trigger** | Caller polls `TriggerRetry()`; the retrier decides whether to try based on the delay window | Caller drives the sleep: `GetBackoffDuration(n)` → `time.Sleep(d)` |
| **Best for** | Long-lived connection objects that need an initialisation-with-retry pattern | Short-lived request loops and forwarder circuit breakers where the caller controls the event loop |
| **Backoff support** | Built-in via `Strategy = Backoff`; delay doubles each failure | Exponential with jitter via `ExpBackoffPolicy` |

### How the forwarder uses backoff

[`comp/forwarder/defaultforwarder`](../../../comp/forwarder/defaultforwarder.md) uses
`pkg/util/backoff.ExpBackoffPolicy` (not this package) to manage per-endpoint circuit breakers.
When a `domainForwarder` worker receives an HTTP error it calls `policy.IncError`; on success it
calls `policy.DecError`. This keeps the policy stateless and safe to share across worker
goroutines.

### How Remote Configuration uses backoff

The RC client (`pkg/config/remote/client`) adds `backoffPolicy.GetBackoffDuration(errorCount)`
to its base poll interval after a failed `ClientGetConfigs` call, reducing polling pressure on
the core agent. The RC service similarly uses an `ExpBackoffPolicy` (2–5 min range) for backend
fetch failures. See [`pkg/config/remote`](../../pkg/config/remote.md).

## Cross-references

| Topic | See also |
|-------|----------|
| Stateless exponential-backoff policy used by the forwarder and RC client | [`pkg/util/backoff`](backoff.md) |
| Forwarder per-endpoint circuit breaker built on `pkg/util/backoff` | [`comp/forwarder/defaultforwarder`](../../../comp/forwarder/defaultforwarder.md) |
| Remote Configuration poll-interval backoff and service-level backoff | [`pkg/config/remote`](../../pkg/config/remote.md) |
