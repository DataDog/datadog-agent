# OpenMetrics Python contrast SMP cases

These cases provide a small, reviewable proof that the OpenMetrics scale problem
is not caused by Python checks being inherently slow.

The suite runs four scenarios at three target counts, using the same 962-sample payload:

1. `noop`: a custom Python check dispatches and submits one gauge.
2. `io_read`: a custom Python check performs HTTP IO and reads an
   OpenMetrics-shaped response body.
3. `parse_submit`: a custom Python check reads/parses the same body and submits
   one gauge per parsed sample.
4. `real`: the real OpenMetrics v2 integration scrapes the same endpoint and
   payload shape.

Cases use `target_count in (200, 500, 1500)`, `DD_CHECK_RUNNERS=4`,
`min_collection_interval=15s`, and lading response delay
100ms. The expected story is that `noop` and `io_read` stay near
scheduler throughput, `parse_submit` shows the controlled cost of Python
parse/submit work, and `real` shows the OpenMetrics-specific degradation.

## Important lading note

The cases use lading's official `body_variant.openmetrics` generator syntax. At
the time this PR was written that generator exists in a custom lading build from
`DataDog/lading#1895` and is being upstreamed. `test/regression/config.yaml`
pins `ghcr.io/datadog/lading:sha-87e2bc917be922a0594bc4a7131cb88516c1e9e6`
so SMP runs use a lading build that includes the generator.

## Cases

* `openmetrics_python_contrast_noop_samples0962_t0200_r04` — noop, 200 targets, 962 OpenMetrics samples
* `openmetrics_python_contrast_io_read_samples0962_t0200_r04` — io_read, 200 targets, 962 OpenMetrics samples
* `openmetrics_python_contrast_parse_submit_samples0962_t0200_r04` — parse_submit, 200 targets, 962 OpenMetrics samples
* `openmetrics_python_contrast_real_samples0962_t0200_r04` — real, 200 targets, 962 OpenMetrics samples
* `openmetrics_python_contrast_noop_samples0962_t0500_r04` — noop, 500 targets, 962 OpenMetrics samples
* `openmetrics_python_contrast_io_read_samples0962_t0500_r04` — io_read, 500 targets, 962 OpenMetrics samples
* `openmetrics_python_contrast_parse_submit_samples0962_t0500_r04` — parse_submit, 500 targets, 962 OpenMetrics samples
* `openmetrics_python_contrast_real_samples0962_t0500_r04` — real, 500 targets, 962 OpenMetrics samples
* `openmetrics_python_contrast_noop_samples0962_t1500_r04` — noop, 1500 targets, 962 OpenMetrics samples
* `openmetrics_python_contrast_io_read_samples0962_t1500_r04` — io_read, 1500 targets, 962 OpenMetrics samples
* `openmetrics_python_contrast_parse_submit_samples0962_t1500_r04` — parse_submit, 1500 targets, 962 OpenMetrics samples
* `openmetrics_python_contrast_real_samples0962_t1500_r04` — real, 1500 targets, 962 OpenMetrics samples

## Running locally

```bash
smp local-run --experiment-dir test/regression \
  --case openmetrics_python_contrast_real_samples0962_t1500_r04 \
  --target-image <agent-dev-image>
```

## Regenerating

```bash
test/regression/scripts/render_openmetrics_python_contrast_cases.py
```
