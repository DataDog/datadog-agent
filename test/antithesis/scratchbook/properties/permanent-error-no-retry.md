# Evidence: permanent-error-no-retry

## Summary

The HTTP destination classifies intake responses as either retryable (5xx,
network errors, 429) or permanent (400, 401, 403, 413). Permanent errors
must NOT be retried — the payload is dropped, `DestinationLogsDropped` is
incremented, and the auditor offset still advances (the payload goes to
`output <- payload` regardless). This property tests that permanent errors
are handled correctly: dropped exactly once with no retry loop.

## Key code

**`comp/logs-library/client/http/destination.go:407-424`** — response classification:
```go
if resp.StatusCode == http.StatusForbidden &&
    d.secrets.IsValueFromSecret(d.endpoint.GetAPIKey()) &&
    d.secrets.Refresh() {
    return client.NewRetryableError(errServer)  // 403 + secrets → retry once
} else if resp.StatusCode == http.StatusBadRequest ||
    resp.StatusCode == http.StatusUnauthorized ||
    resp.StatusCode == http.StatusForbidden ||
    resp.StatusCode == http.StatusRequestEntityTooLarge {
    tlmDropped.Inc()
    return errClient  // NOT a RetryableError → permanent drop
} else if resp.StatusCode > http.StatusBadRequest {
    return client.NewRetryableError(errServer)  // 5xx, 429 → retry
}
```

**`comp/logs-library/client/http/destination.go:296-319`** — `sendAndRetry()`:
```go
if d.shouldRetry {
    if d.updateRetryState(err, isRetrying) {
        continue  // only RetryableError gets here
    }
}
if err != nil {
    // Permanent error, increment the logs dropped metric
    metrics.DestinationLogsDropped.Add(d.host, payload.Count())
    metrics.TlmLogsDropped.Add(...)
} else {
    metrics.LogsSent.Add(payload.Count())
    ...
}
output <- payload  // sent to auditor in BOTH cases
```

Note: `output <- payload` is executed for **both permanent errors and success**.
This means the auditor advances the offset even for permanently dropped payloads.
This is intentional (permanent errors = non-recoverable, no point re-reading)
but means "at least once delivery" does NOT apply to 4xx errors.

## 429 treatment

429 (Too Many Requests) is treated as `> StatusBadRequest` (which is 400), so:
`resp.StatusCode > http.StatusBadRequest` = `StatusCode > 400`. 429 > 400 → yes,
falls into `client.NewRetryableError(errServer)`. This is confirmed by the test
at `destination_test.go:109`: `retryTest(t, 429)`.

**There is no `Retry-After` header support.** The agent ignores any
`Retry-After` header in 429 responses and applies its own exponential backoff
(base=1s, max=120s, factor=2). This means the agent may retry faster than the
intake requests, potentially triggering more 429s.

## The 403 + secrets exception

A 403 response from an endpoint whose API key was sourced from a secrets backend
triggers a secrets refresh and retries once (line 410-413). This is the *only*
retry for a 4xx response. After the refresh+retry, if still 403, it becomes
permanent.

## Why it matters

Misclassifying a permanent error as retryable would cause the agent to loop
indefinitely on a bad payload, blocking the pipeline. The test
`TestNoRetries` confirms 400/401/403/413 are not retried, but under fault
injection, a proxy/chaos layer might return unusual status codes that need
correct classification.

## Assertion design

**SUT-side (`AlwaysOrUnreachable`):** In `sendAndRetry()`, at the point where
`d.shouldRetry` is false and `err != nil` (permanent drop path), assert that
the error is NOT a `*client.RetryableError`. Asserts the classification is
correct whenever this path runs.

**Workload-side (`Always`):** When fakeintake returns 400/401/403/413 for a
payload, assert that the payload is NOT retried (fakeintake should receive it
exactly once, not in a retry loop). Observable via fakeintake request count
for the same payload.

**Workload-side (`Sometimes`):** Confirm at least one permanent-drop scenario
is observed during the run (fakeintake receives a 4xx trigger and the agent
does not loop).

## Open Questions

- The 403 + secrets refresh path adds one retry before going permanent. Is the
  retry on the *same payload* or a reconstructed payload? If the API key in the
  payload header changes after refresh, does the payload need to be re-encoded?
- Does `updateRetryState(errClient, isRetrying)` signal `isRetrying <- false`?
  If yes, the worker's `cancelSendChan` is triggered, which may unblock a pending
  `Send()` call that was waiting for retry recovery.
- Is 413 (payload too large) the right behavior for "advance the auditor"? The
  individual messages in a too-large batch are valid — only the batching was
  wrong. A better behavior might be to split the batch and retry. The current
  code drops the batch silently. `(needs human input)`

### Investigation Log

#### Does `output <- payload` in the permanent-error path block? If the auditor's input channel (cap=100) is full, this blocks the destination goroutine.

- Examined: `comp/logs-library/client/http/destination.go:228-319` (`run` and `sendAndRetry`), `comp/logs-library/client/http/worker_pool.go` (`workerPool`), `comp/logs-library/sender/worker.go:108` (`buildDestinationSenders`), `comp/logs-library/sender/sender.go:113-158` (`NewSender`), `comp/logs/auditor/impl/auditor.go:89` (`messageChannelSize`), `pkg/config/setup/common_settings.go:1870` (`message_channel_size` default = 100).
- Found: `sendAndRetry` runs inside a goroutine dispatched by `workerPool.run()`. The `output` channel passed to `sendAndRetry` is `reliableOutputChan` (the auditor `inputChan`, cap 100 by default). The `output <- payload` at line 318 is an **unguarded blocking send** — it will block the `sendAndRetry` goroutine if the auditor channel is full. However, this goroutine is separate from the `destination.run()` loop (which reads from the `input` channel). The `run()` loop's `d.wg.Wait()` at shutdown means if workers are blocked on `output <- payload`, the destination's `Stop()` path will also block. Under a sustained 4xx storm: each successful (permanent-drop) send completes with `output <- payload`. If the auditor is slow to drain (e.g., registry write latency), the worker goroutines pile up blocked on the auditor channel. This IS a secondary backpressure source but it is bounded — the workerPool limits concurrent goroutines, and the auditor drains continuously.
- Not found: any `select` with timeout or `default` case around `output <- payload`; it is always a plain blocking send.
- Conclusion: resolved. `output <- payload` CAN block if the auditor channel (cap 100) is full. Under sustained 4xx storms this stalls `sendAndRetry` goroutines but does not cause indefinite deadlock because the auditor drains at registry-write speed. It is a bounded secondary backpressure source, not an unbounded stall. The property's assertion design is not affected, but test workloads should monitor auditor channel depth under sustained 4xx injection.

## Merged-in evidence (from permanent-errors-not-retried)

The secondary file was the dual of `retry-no-data-loss-on-partition` and added
the following not present in the canonical:

**Hypothesis: liveness violation from infinite retry (regression guard):** if a
future code change accidentally wraps a 4xx in `client.NewRetryableError()`, the
agent would retry indefinitely on a bad API key or malformed payload. The pipeline
would appear stalled; `RetryCount` would increment unboundedly.

**Auditor divergence from intake:** the complementary liveness failure — if a 4xx
does NOT advance the auditor, the pipeline will repeatedly re-read and re-send the
same lines indefinitely (infinite retry through the file, not the retry loop).

**`DestinationLogsDropped` as the only signal** (`destination.go:311-312`):
```go
metrics.DestinationLogsDropped.Add(d.host, payload.Count())
metrics.TlmLogsDropped.Add(float64(payload.Count()), d.host)
```
This is the only observable signal that permanent drops occurred. The workload
must monitor this counter to distinguish drops from delivery.

**Additional workload assertion (from secondary):** configure fakeintake to return
400 Bad Request for payloads containing a specific marker. Then assert:
1. `DestinationLogsDropped` counter increments exactly once for the dropped payload.
2. `RetryCount` does NOT increment for the dropped payload.
3. The pipeline continues delivering subsequent log lines (the drop doesn't jam
   the pipeline).
4. Auditor offset advances past the dropped payload's lines.
