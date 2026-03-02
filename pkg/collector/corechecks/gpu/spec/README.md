# GPU Metric Spec

This directory contains the GPU corecheck specification files:

- `gpu_metrics.yaml`: metric catalog (metric names, required tagsets/custom tags, and support matrix by architecture/device mode).
- `architectures.yaml`: architecture capabilities (GPM support, unsupported NVML fields by mode, and unsupported device modes such as `mig`/`vgpu`).

Each YAML file has headers describing the schema.

The spec files are validated by running the tests in `pkg/collector/corechecks/gpu/spec_test.go`.

## Validate the spec

Run spec-focused validation:

`go test -tags "test nvml" ./pkg/collector/corechecks/gpu -run TestMetricsFollowSpec`

Run all GPU corecheck tests:

`go test -tags "test nvml" ./pkg/collector/corechecks/gpu`

Notes:

- `test` tag is required because test helpers use `comp/core/tagger/fx-mock`.
- `nvml` tag is required because these tests are guarded by `//go:build linux && nvml`.
