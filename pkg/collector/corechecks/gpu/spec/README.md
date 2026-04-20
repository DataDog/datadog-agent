# GPU Metric Spec

This directory contains the GPU corecheck specification files:

- `gpu_metrics.yaml`: metric catalog (metric names, required tagsets/custom tags, and support matrix by architecture/device mode).
- `architectures.yaml`: architecture capabilities (GPM support, unsupported NVML fields by mode, and unsupported device modes such as `mig`/`vgpu`).
- `tags.yaml`: reusable tag definitions (`tags`) and reusable tag groups (`tagsets`), including workload-only tagsets and regex validation for tag values.

Each YAML file has headers describing the schema.

The Go code in this package turns those YAML specs into shared validation logic used in tests and in live data validation:

- `validation.go`:
  - Enumerates supported architecture/device-mode combinations via `KnownGPUConfigs`.
  - Computes expected metrics per config via `ExpectedMetricsForConfig`.
  - Validates emitted metrics/tags/values via `ValidateEmittedMetricsAgainstSpec` (`missing`, `unknown`, `unsupported`, `invalid_value`).
- `metrics-validator/`: validates Datadog metric data against the same shared spec logic.
- `allowlist/`: syncs GPU metrics from the spec into the billing allowlist.

The spec files are also validated by tests in `pkg/collector/corechecks/gpu/spec/spec_test.go`.

## Validate the spec

Run one or more of these three validation levels:

1. **Mocked NVML against spec** (`TestMetricsFollowSpec`):

   `dda inv test --targets=./pkg/collector/corechecks/gpu -- -tags "test nvml" -run TestMetricsFollowSpec`

2. **Real NVML APIs against spec** (`integrationtests`, requires real GPU + NVML):

   `dda inv test --targets=./pkg/collector/corechecks/gpu/integrationtests -- -tags "nvml"`

3. **Live Datadog data against spec** (`spec/metrics-validator` through invoke task):

   `dda inv gpu.validate-metrics --lookback-seconds 3600 --org staging`
