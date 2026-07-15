# One malformed metric line aborts the entire scrape

## Summary

The Go OpenMetrics scraper returns an error and stops processing the response
when one payload line cannot be parsed. The Python scraper logs the malformed
line, skips it, and continues submitting valid metrics from the same response.

## Reproduction

Serve a payload containing valid metrics around one malformed sample:

```text
# TYPE before gauge
before 1
broken{label="unterminated} 2
# TYPE after gauge
after 3
```

## Expected behavior

The scraper should submit `before` and `after`, report the malformed line, and
complete the scrape with partial data.

## Actual behavior

Python submits the valid metrics and skips `broken`.

Go returns a parse error. Later valid metrics are not processed; buffered paths
may also produce no submissions from the response.

## Root cause

Payload parsing propagates line-level syntax errors out of the scrape loop.
There is no recovery boundary around an individual sample or metric family.

## Impact

One bad exporter sample can suppress every metric from that endpoint until the
payload becomes valid. This is a migration regression for endpoints tolerated
by the Python check and creates a large failure domain for an observability
collector.

## Suggested fix

Recover at the smallest safe boundary:

- Sample parse errors: log and skip the sample line.
- Metadata errors in `HELP`, `TYPE`, or `UNIT`: skip the affected family and
  resume at the next family.
- Transport-level failures such as invalid compression or unreadable response
  bodies: continue to fail the scrape.

Track skipped lines with telemetry so malformed exporters remain observable.

## Verification

Add a scrape-level test with valid samples before and after each malformed
line class. Assert that:

- Both valid samples are submitted.
- The malformed sample is not submitted.
- The scrape does not return a payload parse error.
- Skipped-line telemetry increments.
- Transport failures still fail the scrape.
