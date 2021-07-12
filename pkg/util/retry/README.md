## package `retry`

This package implements a configurable retry mechanism that can
be embedded in any class needing a retry-on-error system.

It's flexible enough to support any process that exposes a
`func() error` method, and can be extended for other retry
strategies than we default ones.

### Supported strategies:

- **OneTry** (default): don't retry, fail on the first error
- **RetryCount**: retry for a set number of attempts when `TriggerRetry`
is called (returning a `FailWillRetry` error), then fail with a `PermaFail`
- **Backoff**: retry with a duration between two consecutive retries that
double at each new try up to a maximum

### How to embed the Retrier

Your class needs to:

- provide a function returning an `error` (`nil` on success)
- embed a `Retrier` object as an anonymous struct field (like a `sync.Mutex`)
- call `self.SetupRetrier()` with a valid `Config` struct

Have a look at `retrier_test.go` for an example.

### How to use a class embedding Retrier

Assuming the class is properly initialised, you can use any of the public
methods from `retrier.go` on that class:

- `RetryStatus()` will return the current status (defined in `types.go`)
- `TriggerRetry()` will either return `nil` if everything is OK, or an
`error` (real type `Retry.Error`) if the attempt was unsuccessful.
- passing the error to `Retry.IsErrWillRetry()` and `Retry.IsErrPermaFail()`
will tell you whether it's necessary to retry again, or just give up
initialising this object
- retry throtling is implemented in case several users try to use a class.
Calling `NextRetry()` will tell you when the next retry is possible. Before
that time, all calls to `TriggerRetry()` will return a `FailWillRetry` error.
**The retry will not automatically run when that time is reached, you have
to schedule a call to `TriggerRetry`.**
