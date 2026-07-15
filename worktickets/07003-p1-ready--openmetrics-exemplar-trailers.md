# Exemplars fail when an endpoint uses the Prometheus content type

## Summary

The Go scraper accepts OpenMetrics exemplar trailers when the HTTP response is
`application/openmetrics-text`, but rejects the same sample when the response
is `text/plain; version=0.0.4`. Python accepts both.

Many exporters and proxies preserve the legacy `text/plain` header while
emitting exemplar syntax. One such sample aborts the Go scrape.

## Reproduction

Return this body:

```text
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="0.5"} 5 # {trace_id="abc"} 0.3 1620000000.0
request_duration_seconds_bucket{le="+Inf"} 10
request_duration_seconds_count 10
request_duration_seconds_sum 4.2
```

Run it under both headers:

```http
Content-Type: application/openmetrics-text; version=1.0.0
```

```http
Content-Type: text/plain; version=0.0.4
```

## Expected behavior

Both responses should submit the histogram samples. The exemplar may be ignored;
semantic exemplar ingestion is not required for compatibility.

## Actual behavior

The OpenMetrics parser accepts the exemplar.

The Prometheus parser selected for `text/plain` returns:

```text
expected timestamp or new record, got "#"
```

Python ignores the exemplar trailer and submits the histogram under either
content type.

## Root cause

Parser selection is driven by the response content type. Exemplar support exists
only in the OpenMetrics parser. The Prometheus parser treats `#` after a sample
value or timestamp as an invalid token.

## Impact

Endpoints tolerated by Python can lose every metric after migration to Go if
they emit exemplars with a legacy or stale content type.

## Suggested fix

Teach the Prometheus text path to recognize and ignore a syntactically valid
exemplar trailer after the sample value or optional timestamp. Keep malformed
trailers as errors unless the broader line-recovery policy handles them.

This is narrower and cheaper than retrying the entire response with a second
parser.

## Verification

Use the same payload with both content types and assert that:

- The scrape succeeds.
- Histogram bucket, count, and sum submissions match Python.
- Exemplar labels do not become metric tags.
- Malformed exemplar syntax follows the documented error/recovery policy.
