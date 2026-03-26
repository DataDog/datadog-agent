# pkg/util/backoff

Import path: `github.com/DataDog/datadog-agent/pkg/util/backoff`

## Purpose

Provides a pluggable backoff abstraction used whenever the agent needs to throttle retries after failures — most notably in the forwarder (blocked endpoints), the Remote Configuration client and service, and the log pipeline's HTTP destination.

The package separates **policy** (the `Policy` interface) from the **state** (a plain `int` error counter managed by the caller). This keeps the policy objects stateless and therefore safe to share across goroutines.

## Key elements

### `Policy` (interface)

```go
type Policy interface {
    GetBackoffDuration(numErrors int) time.Duration
    IncError(numErrors int) int
    DecError(numErrors int) int
}
```

Callers own an `int` error counter. They call:
- `IncError` on failure — returns the new (capped) counter value.
- `DecError` on success — returns the new (floored) counter value, stepping down at `RecoveryInterval`.
- `GetBackoffDuration` to translate the current counter into a `time.Duration` to sleep.

The interface makes it straightforward to substitute a test double or a different algorithm without changing call sites.

### `ExpBackoffPolicy` (struct)

The only built-in `Policy` implementation. Computes an exponential back-off with jitter:

| Field | Role |
|---|---|
| `BaseBackoffTime` | Controls the rate of exponential growth. The first interval starts at roughly `BaseBackoffTime / MinBackoffFactor * 2` seconds. |
| `MinBackoffFactor` | Controls the width of the jitter window. At `2`, consecutive ranges do not overlap; higher values increase overlap (approaching 50%). |
| `MaxBackoffTime` | Hard cap on the wait duration (seconds). |
| `RecoveryInterval` | Number of error-count steps removed per successful operation. |
| `MaxErrors` | Derived: the number of errors required to reach `MaxBackoffTime`. Computed automatically by `NewExpBackoffPolicy`. |

Duration formula (for `numErrors > 0`):

```
candidate = BaseBackoffTime * 2^numErrors
if candidate > MaxBackoffTime:
    wait = MaxBackoffTime
else:
    wait = random(candidate / MinBackoffFactor, candidate)
```

### `NewExpBackoffPolicy`

```go
func NewExpBackoffPolicy(
    minBackoffFactor, baseBackoffTime, maxBackoffTime float64,
    recoveryInterval int,
    recoveryReset bool,
) Policy
```

`recoveryReset = true` sets `RecoveryInterval = MaxErrors`, meaning a single success resets the counter to zero.

## Usage

### Typical call-site pattern

The caller stores an `int` alongside the policy:

```go
type myClient struct {
    policy           backoff.Policy
    backoffErrorCount int
}

// on failure:
c.backoffErrorCount = c.policy.IncError(c.backoffErrorCount)
sleep := c.policy.GetBackoffDuration(c.backoffErrorCount)
time.Sleep(sleep)

// on success:
c.backoffErrorCount = c.policy.DecError(c.backoffErrorCount)
```

### Forwarder — blocked endpoints

`comp/forwarder/defaultforwarder/blocked_endpoints.go` uses `ExpBackoffPolicy` to determine how long to block a failing intake endpoint before retrying. When an endpoint returns an error, `IncError` is called; when it succeeds, `DecError` steps the counter back toward zero.

### Remote Configuration client

`pkg/config/remote/client/client.go` adds the backoff duration to its base poll interval so that a flaky RC service automatically reduces polling pressure:

```go
interval = c.pollInterval + c.backoffPolicy.GetBackoffDuration(c.backoffErrorCount)
```

### Log HTTP destination

`pkg/logs/client/http/destination.go` uses the policy to back off after failed HTTP sends to the intake endpoint.

## Relationship to `pkg/util/retry`

`pkg/util/backoff` and [`pkg/util/retry`](retry.md) solve related but distinct problems:

| | `pkg/util/backoff` | `pkg/util/retry` |
|---|---|---|
| **Unit** | Stateless `Policy`; caller owns the error counter | Embeddable struct with lifecycle state (`Idle` → `FailWillRetry` → `PermaFail` → `OK`) |
| **Trigger** | Caller drives the sleep: `GetBackoffDuration(n)` → `time.Sleep(d)` | Caller polls `TriggerRetry()`; the retrier decides whether to try |
| **Best for** | Short-lived request loops and forwarder circuit breakers where the caller controls the event loop | Long-lived connection objects that need an initialisation-with-retry pattern |
| **Permanent failure** | No concept — caller controls when to stop retrying | `PermaFail` status after `RetryCount` exhaustion or `OneTry` |

Use `pkg/util/backoff` when the caller already owns an event loop (e.g. a worker goroutine that sends HTTP requests in a loop). Use `pkg/util/retry` when you want to embed retry logic into a struct that is initialized lazily (e.g. a client connection object that must be ready before use).

## Cross-references

| Document | Relationship |
|---|---|
| [`pkg/util/retry`](retry.md) | Higher-level retry abstraction with lifecycle states; uses a built-in doubling strategy rather than `ExpBackoffPolicy` |
| [`comp/forwarder/defaultforwarder`](../../../comp/forwarder/defaultforwarder.md) | Largest consumer; uses `ExpBackoffPolicy` in `blocked_endpoints.go` as a per-endpoint circuit breaker — `IncError` on HTTP failure, `DecError` on success |
