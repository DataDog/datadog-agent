## Current verification after Ali branch update

Still open, but narrowed to HTTP content-type-driven parser selection.

Ali's streaming **OpenMetrics parser** accepts and validates exemplar trailers.
The scraper selects that parser when the endpoint responds with an OpenMetrics
content type such as:

```http
Content-Type: application/openmetrics-text; version=1.0.0
```

The same payload is routed through the **Prometheus text parser** when the
endpoint responds with the widespread legacy content type:

```http
Content-Type: text/plain; version=0.0.4
```

The differential payload server uses the latter. The Prometheus parser sees the
`#` after the sample value as an invalid post-value token, so the focused case
still reports:

```text
go_rejected_py_accepted  go=0 py=4
scrape: expected timestamp or new record, got "#"
```

Strictly, an exporter that advertises Prometheus 0.0.4 while emitting exemplars
is mixing formats. Operationally, however, Python tolerates this combination,
and real exporters or proxies may preserve a stale `text/plain` header while
emitting newer syntax. Rejecting it would therefore be a Python-to-Go migration
regression, and one exemplar currently aborts the entire scrape.

### Recommended resolution

Teach the Prometheus text path to recognize and ignore a syntactically valid
OpenMetrics exemplar trailer after the sample value or optional timestamp. This
is a narrow compatibility extension:

- It preserves Python behavior for mislabeled endpoints.
- It does not require semantic exemplar ingestion.
- It avoids retrying or reparsing the entire response.
- Malformed trailers can retain the existing error behavior unless 07001's
  broader per-line recovery policy supersedes it.

Alternative resolutions are to enforce the declared format strictly and
document the migration break, or retry with the OpenMetrics parser after this
specific failure. The strict option is standards-correct but operationally
riskier; parser retry adds more complexity than the narrow compatibility rule.

Verification should cover the same exemplar payload under both content types:

| Response `Content-Type` | Expected Go behavior |
|---|---|
| `application/openmetrics-text; version=1.0.0` | Existing OpenMetrics parser accepts the sample |
| `text/plain; version=0.0.4` | Prometheus parser accepts the sample and ignores the trailer |

---

## Summary

The Go scraper aborts the entire scrape with
`scrape: expected timestamp or new record, got "#"` when a sample line
includes an OpenMetrics exemplar trailer. Python ignores the exemplar
section and accepts the sample.

Exemplars are an OpenMetrics 1.0.0 feature widely used to attach trace
context (trace_id / span_id) to histogram and counter samples. Any modern
endpoint emitting exemplars for trace correlation will trigger this.

## Context

OpenMetrics exemplar syntax (from the OpenMetrics 1.0.0 spec):

```
<metric_name>{<labels>} <value> # {<exemplar_labels>} <exemplar_value> [<timestamp>]
```

Example from the Prometheus blog post on exemplars:

```
http_request_duration_seconds_bucket{le="0.5"} 5 # {trace_id="abc"} 0.3 1620000000.0
http_request_duration_seconds_bucket{le="1.0"} 10 # {trace_id="def"} 0.7 1620000001.0
```

Python parses these with `prometheus_client` which knows the format;
Go's tokenizer hits the `#` after the value and errors expecting a
timestamp or newline.

## Repro

Already in the adversarial catalog:

```bash
go test -tags openmetrics_differential -v \
    -run 'TestOpenMetricsAdversarial/format/openmetrics_exemplar' \
    ./pkg/collector/corechecks/openmetrics/differential/
```

Current output:

```
go_err: scrape: expected timestamp or new record, got "#"
    ("INVALID") while parsing: "req_dur_bucket{le=\"0.5\"} 5 #"
```

Python returns the 4 expected submissions (3 buckets + count + sum).

The minimal repro payload (10 lines of the adversarial case):

```
# TYPE req_dur histogram
req_dur_bucket{le="0.5"} 5 # {trace_id="abc"} 0.3 1620000000.0
req_dur_bucket{le="+Inf"} 10
req_dur_count 10
req_dur_sum 4.2
```

## Suggested fix

The narrowest fix: after parsing a value (and optional timestamp), if
the next non-whitespace character is `#`, treat the rest of the line as
an exemplar trailer and **skip it** without parsing semantically.

A richer fix would parse exemplars and submit them via a new Datadog
APM/correlation surface — but that's a separate feature. For now,
"skip and don't error" is exactly what Python does and is enough to
unblock production rollout.

Location: wherever the post-value token parsing happens in the scraper.
Likely the same code path that handles the optional timestamp.

## Severity rationale

P1 because:
- Modern OpenMetrics endpoints emit exemplars routinely (Prometheus
  client libs add them automatically when traces are present in
  request context).
- Failure is total (zero metrics) — this is exactly the production
  regression the P0 ticket describes, but with a specific narrow fix.
- The fix is one conditional in the tokenizer.

## Verification

After fix, this passes:

```
TestOpenMetricsAdversarial/format/openmetrics_exemplar
```

And the `openmetrics_exemplar` entry in `known_divergences.go` can
be removed.
