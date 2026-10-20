# GPU Metric Spec

This directory contains the GPU corecheck specification files:

- `gpu_metrics.yaml`: metric catalog (metric names, required tagsets/custom tags, and support matrix by architecture/device mode/capability).
- `architectures.yaml`: architecture capabilities (GPM, NVLink version from NVML `GetNvLinkVersion`, C2C support, unsupported NVML fields by mode, and unsupported device modes such as `mig`/`vgpu`).
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

## Metric availability

Metric availability is evaluated independently from support by architecture,
device mode, and hardware capabilities:

- `workload_only: true` means a metric is expected only when the caller
  declares an active workload.
- `config_required` lists named Agent configuration features that must all be
  enabled for the metric to be expected. `system_probe_ebpf` and
  `system_probe_prm` represent their respective system-probe GPU monitoring
  features.
- `optional: true` means an otherwise supported metric may be absent on an
  individual device, such as a fanless GPU. If emitted, optional metrics still
  undergo normal support, tag, type, and value validation.

Workload-only tagsets are selected separately through
`ValidationOptions.WorkloadTagsets`. This allows a bare process workload to
require the `process` tagset without also requiring Kubernetes container tags.
Live Datadog validation enables all config features and workload-only tagsets;
real-GPU integration tests provide their actual configuration and context.

## External value validation

Metric `validator` entries can validate real-GPU integration-test values against
external references:

- `nvidia_smi: true` compares with the normalized `nvidia-smi` sample.
- `calibrated_workload: true` compares with gpu-burner's measured status value.
- `value_tolerance` is required with either source and contains `absolute` and/or
  `relative` (percentage of the known-good value). When both are provided, both
  limits must pass.

These checks fail if the marked Agent metric or its external reference is
missing. They are only evaluated by real-GPU integration tests; the live
Datadog validator applies only static `range` and `values` constraints.

## Validate the spec

Run one or more of these three validation levels:

1. **Mocked NVML against spec** (`TestMetricsFollowSpec`):

   `dda inv test --targets=./pkg/collector/corechecks/gpu -- -tags "test,nvml" -run TestMetricsFollowSpec`

2. **Real NVML APIs against spec** (`integrationtests`, requires real GPU + NVML):

   `dda inv test --targets=./pkg/collector/corechecks/gpu/integrationtests -- -tags "nvml"`

3. **Live Datadog data against spec** (`spec/metrics-validator` through invoke task):

   `dda inv gpu.validate-metrics --lookback-seconds 3600 --org staging`
