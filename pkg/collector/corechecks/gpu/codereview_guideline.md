# GPU monitoring review guidelines

Scope: `pkg/collector/corechecks/gpu/`, `pkg/gpu/`, and
`cmd/system-probe/modules/gpu*`.

## Metric coverage: do not request new fakeintake E2E tests

For code in this scope, this section replaces the "E2E coverage with
fakeintake" check from the root `codereview_guideline.md`.

GPU metric emission is covered by the GPU spec (`spec/gpu_metrics.yaml`) and
the tests that validate against it:

- `TestMetricsFollowSpec` checks the emitted metrics and tags against the spec
  with mocked NVML, for every supported architecture and device mode.
- `integrationtests/` runs the check against real NVML and GPUs and validates
  the output against the same spec.

Do not ask for new fakeintake assertions in `test/new-e2e/tests/gpu/` for new
or changed GPU metrics or tags. Instead, flag a PR that adds, removes, or
renames a metric or tag without updating `spec/gpu_metrics.yaml` or
`spec/tags.yaml`, because the spec-driven tests only cover what the spec
declares.

## Independent hardware capabilities

When adding NVML metrics, check support at the operation, field, and port level. Flag using one counter's support as a prerequisite for independent counters when that discards supported metrics. Preserve successful independent results and distinguish unsupported capability errors from other failures. A shared support probe is valid when the vendor contract guarantees that those capabilities are supported together.
