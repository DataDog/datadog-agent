# No standalone `share_labels` compatibility bug

## Status

Closed. Correctly configured `share_labels` behavior matches between the Go and
Python OpenMetrics scrapers.

## Correct semantics

`share_labels` is keyed by the source metric. `match` lists labels used to join
source and target samples. `labels` lists values copied from the source.

```yaml
share_labels:
  kube_pod_info:
    match:
      - namespace
      - pod
    labels:
      - node
      - pod_ip
```

For a target sample with the same `namespace` and `pod`, both implementations
copy `node` and `pod_ip` from `kube_pod_info`.

## Verification

The following behaviors agree:

- Conditional joins using one or more matching labels.
- Copying selected labels or all source labels.
- Source families appearing before target families.
- Cached mode where a source appearing after a target affects the next scrape.
- Uncached mode where the prepass collects source labels for the current scrape.
- Source value filters.
- `target_info` label propagation.

## Related limitation

`share_labels` combined with `raw_metric_prefix` is tracked separately because
prefix normalization changes source lookup behavior. That interaction does not
indicate a defect in standalone `share_labels` processing.

## No further action

Keep focused parity tests for the behaviors above. Reopen only with a minimal,
correctly keyed source metric and a demonstrated Go/Python output difference.
