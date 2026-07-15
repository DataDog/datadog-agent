# `raw_metric_prefix` prevents `share_labels` source matching

## Summary

The Go OpenMetrics scraper does not apply `share_labels` when the configured
source metric starts with `raw_metric_prefix`.

Go strips the prefix before looking up the source metric in the `share_labels`
configuration. Python resolves `share_labels` against the original metric name
and strips the prefix later.

This changes the tags emitted after migrating a check from Python to Go.

## Minimal configuration

```yaml
namespace: diff
metrics:
  - ".+"
raw_metric_prefix: diff_lading_
share_labels:
  diff_lading_target_info:
    match:
      - service
      - region
    labels:
      - shard
```

The scrape payload contains this source metric:

```text
# TYPE diff_lading_target_info gauge
diff_lading_target_info{service="checkout",region="us-east-1",shard="control"} 1
```

It also contains target metrics with matching `service` and `region` labels,
for example:

```text
# TYPE diff_lading_queue_depth gauge
diff_lading_queue_depth{service="checkout",region="us-east-1",queue="queue-00"} 0
```

## Expected behavior

Both implementations should:

1. Find `diff_lading_target_info` as the configured `share_labels` source.
2. Copy `shard="control"` to samples matching
   `service="checkout", region="us-east-1"`.
3. Strip `diff_lading_` from submitted metric names.

The submitted queue metric should include:

```text
name: diff.queue_depth
tags: [..., shard:control]
```

## Actual behavior

Python emits the shared `shard:control` tag.

Go emits the same metric without that tag. In the full generated payload, both
implementations submit 51 metrics, but 49 normalized submissions differ because
their tag sets do not match.

Each option works correctly by itself:

- `raw_metric_prefix` without `share_labels`
- `share_labels` without `raw_metric_prefix`

The failure requires both options.

## Root cause

The Go scraper applies `raw_metric_prefix` before the shared-label prepass:

```text
diff_lading_target_info -> target_info
```

The prepass then tries to find `target_info` in a configuration keyed by
`diff_lading_target_info`, so it never collects the source labels.

The `share_labels` lookup must use the raw family name, or configuration keys
must be normalized using the same prefix rule before lookup. Prefix stripping
should still apply to submitted metric names.

## Impact

Users combining `raw_metric_prefix` and `share_labels` will silently lose
shared tags after migration from Python to Go. Metrics still arrive, so volume
checks will not detect the regression; dashboards and monitors grouped or
filtered by the shared labels may change behavior.

## Verification

Add a focused parity test using the configuration and payload above. Assert
that both Go and Python submit `diff.queue_depth` with `shard:control`.

Also verify:

- The source metric may appear before or after the target metric.
- Cached and uncached shared-label modes behave identically to Python.
- Prefix stripping changes emitted names but not `share_labels` source lookup.
- Configured source names that do not start with the prefix continue to work.
