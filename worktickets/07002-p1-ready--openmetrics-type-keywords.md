# OpenMetrics-only `TYPE` values are accepted but not submitted

## Summary

The Go OpenMetrics parser recognizes `stateset`, `gaugehistogram`, and `info`,
but the native metric transformer only handles `counter`, `gauge`,
`histogram`, and `summary`. A configured native metric with an OpenMetrics-only
type can therefore be parsed and then silently omitted.

For responses parsed as Prometheus text, `stateset` and `gaugehistogram` are
rejected as invalid types and can fail the scrape.

## Reproduction

```text
# TYPE feature_enabled stateset
feature_enabled{feature_enabled="dark_mode"} 1
# EOF
```

Configure the metric for native collection:

```yaml
metrics:
  - feature_enabled
```

Repeat with `info` and `gaugehistogram` families, and with both
`application/openmetrics-text` and `text/plain` response content types.

## Expected behavior

Go should match Python's compatibility behavior and submit these families.
At minimum, unsupported OpenMetrics types should degrade to gauges rather than
abort or silently disappear.

Recommended mappings:

| `TYPE` | Compatibility submission |
|---|---|
| `stateset` | Gauge samples with state labels |
| `info` | Gauge value `1` with info labels |
| `gaugehistogram` | Gauge-histogram buckets, sum, and count |
| Unknown | Gauge/untyped fallback, or an explicit unsupported-config fallback |

## Actual behavior

The parser and transformer support different type sets. OpenMetrics parsing can
succeed while native transformation returns no submission function. The
Prometheus text parser rejects `stateset` and `gaugehistogram` before
transformation.

## Root cause

`validOpenMetricsType` accepts the OpenMetrics 1.0 type set, while
`nativeTransformer` and `skipNativeMetric` only support the four Prometheus
families. `validPrometheusType` has a third, different support set.

## Impact

Modern exporters can emit these types. During migration, affected metrics may
vanish silently or cause the entire endpoint scrape to fail depending on the
response content type.

## Suggested fix

Define one explicit compatibility policy for every parser/transformer path.
Either implement native handling for the OpenMetrics-only families or normalize
them to supported submission types before transformer selection.

Do not allow a parsed, configured family to disappear silently.

## Verification

For each type and both response content types, assert that:

- The scrape succeeds.
- The configured family produces submissions.
- Metric names, values, and tags match Python.
- An unsupported type follows a documented fallback path rather than silently
  disappearing.
