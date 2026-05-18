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
