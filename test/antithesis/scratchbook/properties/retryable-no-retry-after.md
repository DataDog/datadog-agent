# Evidence: retryable-no-retry-after

## Summary

The Datadog Agent HTTP destination treats HTTP 429 (Too Many Requests) as a
retryable server error — the same as 5xx. It applies its own exponential backoff
and **ignores any `Retry-After` header** in the response. This property verifies
that:

1. 429 is NOT classified as a permanent drop (unlike 400/401/403/413).
2. The agent does retry after 429 responses (confirmed by test).
3. No Retry-After parsing exists anywhere in the HTTP destination path.

## Key code

**`comp/logs-library/client/http/destination.go:407-422`**:
```go
if resp.StatusCode == http.StatusForbidden && ...secrets... {
    return client.NewRetryableError(errServer)  // 403 + secrets refresh
} else if resp.StatusCode == http.StatusBadRequest ||
    resp.StatusCode == http.StatusUnauthorized ||
    resp.StatusCode == http.StatusForbidden ||
    resp.StatusCode == http.StatusRequestEntityTooLarge {
    tlmDropped.Inc()
    return errClient  // permanent drop
} else if resp.StatusCode > http.StatusBadRequest {
    return client.NewRetryableError(errServer)  // 5xx AND 429 land here
}
```

429 > 400 (StatusBadRequest) → `client.NewRetryableError(errServer)`.
429 is not listed in the permanent-drop block → retryable. ✓

**`comp/logs-library/client/http/destination_test.go:106-111`**:
```go
func TestRetries(t *testing.T) {
    retryTest(t, 500)
    retryTest(t, 429)  // 429 is tested as retryable
    retryTest(t, 404)
}
```

**No Retry-After parsing:** `grep -r "Retry-After"` across `comp/logs-library/`
returns zero results. The `resp.Header.Get("Retry-After")` call does not exist.

## The absence of Retry-After support

When an intake rate-limits with `Retry-After: 30`, the agent:
1. Applies its own backoff (base=1s, exponential, max=120s)
2. May retry in as little as 1s (first retry) or up to 120s (fully backed off)
3. If backoff is shorter than `Retry-After`, the intake will keep returning 429
4. The agent's backoff will grow on each 429, eventually exceeding `Retry-After`

This is functional but not RFC-compliant. The agent will retry more aggressively
than the intake requests during the early backoff stages, which could amplify
the rate-limiting problem.

## Why it matters

If a future code change moves 429 into the permanent-drop block (e.g., treating
it like 400), all logs would be silently dropped during rate-limiting events.
This would be a catastrophic regression.

The property tests that 429 remains in the retryable bucket and that the agent
actually retries, not permanently drops.

## Assertion design

**SUT-side (`AlwaysOrUnreachable`):** In `updateRetryState()`, when called with
an error from a 429 response, assert the error IS a `*client.RetryableError`.

**Workload-side (`Sometimes`):** Configure fakeintake to return 429 for the
first N requests, then 200. Assert that the agent eventually delivers the
payload (fakeintake receives it successfully after the 429 phase). Confirms
retry actually happens.

**Workload-side (`Always`):** During a sustained 429-response phase (fakeintake
always returns 429), assert that `DestinationLogsDropped` counter does NOT
increase for the 429 responses (since they are retryable, not permanent drops).

## Open Questions

- Is Retry-After support planned? If so, when the feature is implemented this
  property would need a new sub-property to assert Retry-After is honored.
  `(needs human input)`
- Does the absence of Retry-After support cause observable issues in production
  (repeated 429 storms due to over-aggressive retries)? `(needs human input)`

### Investigation Log

#### Is 429 classified as retryable in `destination.go` status handling? Is there any `Retry-After` parsing anywhere?

- Examined: `comp/logs-library/client/http/destination.go:407-422` (status classification), `comp/logs-library/client/http/destination_test.go:106-111` (`TestRetries`), grep for "Retry-After" across `comp/logs-library/` and `pkg/logs/`.
- Found: The status classification block at lines 414-421: 429 > 400 (`http.StatusBadRequest`) so it falls into the `resp.StatusCode > http.StatusBadRequest` branch → `client.NewRetryableError(errServer)`. 429 is explicitly tested as retryable in `TestRetries` (`retryTest(t, 429)`). Zero results for `Retry-After` header in the logs client path — no `resp.Header.Get("Retry-After")` call exists anywhere in `comp/logs-library/`.
- Not found: Any `Retry-After` header parsing, any rate-limit-aware backoff, any special 429 handling beyond treating it identically to 5xx.
- Conclusion: resolved. 429 is retryable (confirmed by code + test). No Retry-After support exists. The agent applies its own exponential backoff (base=1s, max=120s, factor=2) regardless of any `Retry-After` value in the response. The property's invariants and assertion design are correct as written. The "Is Retry-After support planned?" question cannot be answered from the codebase alone — tagged `(needs human input)`.
