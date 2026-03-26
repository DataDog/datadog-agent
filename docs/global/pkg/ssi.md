# pkg/ssi

## Purpose

`pkg/ssi` provides **test helpers for Single Step Instrumentation (SSI)**, Datadog's feature that automatically injects APM tracing libraries into Kubernetes pods without any application code change. The injection is performed by the cluster agent's admission controller webhook when a pod is created.

The package currently contains one sub-package:

- `pkg/ssi/testutils` — assertion utilities for unit-testing the admission webhook's mutation output and for E2E tests that validate injected pods.

It has its own `go.mod` (an independent module) so it can be imported by both the main agent module and the E2E test module without circular dependencies.

## Injection modes

SSI supports three distinct mechanisms for delivering APM library files into containers:

| Constant | Value | Description |
|----------|-------|-------------|
| `InjectionModeInitContainer` | `"init_container"` | Dedicated `datadog-init-*` init containers copy library files into a shared `emptyDir` volume. |
| `InjectionModeCSI` | `"csi"` | The `k8s.csi.datadoghq.com` CSI driver mounts pre-built library images directly as pod volumes. |
| `InjectionModeImageVolume` | `"image_volume"` | Kubernetes image volumes (OCI images mounted directly) supply the library files. |
| `InjectionModeAuto` | `"auto"` | Alias for `init_container` (current default). |

Each mode produces a different set of pod-level volumes, init containers, and container volume mounts, all of which the validators in this package can assert.

## Key types

### `PodValidator`

The primary entry point for test assertions. Wraps a `*corev1.Pod` and provides a rich set of `Require*` methods that fail the test via `testing.T` on mismatch.

```go
validator := testutils.NewPodValidator(pod, testutils.InjectionModeInitContainer)
```

Constructor creates mode-specific `InjectionValidator` and builds `ContainerValidator` instances for every container and init container.

| Method | Description |
|--------|-------------|
| `RequireInjection(t, containers []string)` | Asserts that the named containers are injected and that all other containers are not; also validates mode-specific pod-level artifacts (volumes, annotations, init containers). |
| `RequireNoInjection(t)` | Asserts that no injection artifacts exist on any container or at the pod level. |
| `RequireAnnotations(t, map[string]string)` | Asserts key/value annotation pairs. |
| `RequireVolumeNames(t, []string)` | Asserts named volumes exist. |
| `RequireMissingVolumeNames(t, []string)` | Asserts named volumes do not exist. |
| `RequireEnvs(t, map[string]string, containers)` | Asserts env var values in specific containers. |
| `RequireMissingEnvs(t, []string, containers)` | Asserts env var keys are absent in specific containers. |
| `RequireUnmutatedContainers(t, containers)` | Asserts no `DD_*` or `LD_PRELOAD` env vars were injected (allows a small whitelist set by other webhooks). |
| `RequireLibraryVersions(t, map[string]string)` | Asserts the language-to-version map of injected APM libraries (e.g. `{"java": "v1", "python": "v3"}`). |
| `RequireInjectorVersion(t, string)` | Asserts the injector (`apm-inject`) version tag. |
| `RequireInitContainerImages(t, []string)` | Asserts the full list of init container image strings. |
| `RequireInitSecurityContext(t, *corev1.SecurityContext)` | Asserts the security context on all `datadog-*` init containers. |
| `RequireInitResourceRequirements(t, *corev1.ResourceRequirements)` | Asserts resource requests/limits on all `datadog-*` init containers. |

### `ContainerValidator`

Per-container assertions. Usually obtained indirectly via `PodValidator`, but can be used directly.

| Method | Description |
|--------|-------------|
| `RequireInjection(t)` | Asserts `LD_PRELOAD` and `DD_INJECT_SENDER_TYPE=k8s` are set, and validates mode-specific volume mounts. |
| `RequireNoInjection(t)` | Asserts injection env vars and volume mounts are absent. |
| `RequireEnvs(t, map[string]string)` | Asserts env var key/value pairs. |
| `RequireMissingEnvs(t, []string)` | Asserts env var keys are absent. |
| `RequireVolumeMounts(t, []corev1.VolumeMount)` | Asserts volume mounts exist and match. |
| `RequireMissingVolumeMounts(t, []corev1.VolumeMount)` | Asserts volume mounts do not exist. |
| `RequireCommand(t, string)` | Asserts the full command+args string. |
| `RequireSecurityContext(t, *corev1.SecurityContext)` | Asserts container security context. |
| `RequireResourceRequirements(t, *corev1.ResourceRequirements)` | Asserts CPU/memory requests and limits. |
| `RequireUnmutated(t)` | Asserts no unexpected `DD_*` or `LD_PRELOAD` env vars. |

### `ImageValidator`

Parses and validates a container image string of the form `registry/name:tag`.

| Method | Description |
|--------|-------------|
| `NewImageValidator(image string) *ImageValidator` | Parses `registry/image:tag`. |
| `RequireRegistry(t, expected)` | Asserts registry host path. |
| `RequireName(t, expected)` | Asserts image name. |
| `RequireTag(t, expected)` | Asserts image tag/version. |

### `InjectionValidator` interface

Implemented by the three mode-specific validators (`initContainerInjectionValidator`, `csiInjectionValidator`, `imageVolumeInjectionValidator`). Selected automatically by `NewPodValidator` based on the `InjectionMode`.

```go
type InjectionValidator interface {
    ContainerInjectionValidator
    RequireInjection(t *testing.T)
    RequireNoInjection(t *testing.T)
    RequireLibraryVersions(t *testing.T, expected map[string]string)
    RequireInjectorVersion(t *testing.T, expected string)
}
```

## What each mode validates

### `init_container` mode

- Pod volumes: `datadog-auto-instrumentation` (emptyDir) and `datadog-auto-instrumentation-etc` (emptyDir).
- Pod annotation: `cluster-autoscaler.kubernetes.io/safe-to-evict-local-volumes`.
- Init container `datadog-init-apm-inject`: copies injector files and writes `/etc/ld.so.preload`.
- Per-language init containers `datadog-lib-<lang>-init`: copy language library files.
- Container volume mounts: injector path, `ld.so.preload`, and library path — all read-only.

### `csi` mode

- Pod volumes: `datadog-auto-instrumentation` (type `DatadogLibrary`) and `datadog-auto-instrumentation-etc` (type `DatadogInjectorPreload`) backed by the `k8s.csi.datadoghq.com` driver.
- No init containers for library injection.
- Container volume mounts: CSI-backed volumes.

### `image_volume` mode

- Pod volumes: injector image volume + per-language image volumes (`dd-lib-<lang>`).
- Single `datadog-apm-inject-preload` init container (writes `ld.so.preload` only).
- Container volume mounts: image volumes mounted read-only under `/opt/datadog-packages/datadog-apm-inject` and `/opt/datadog/apm/library/<lang>`.

## Usage

The package is used in:

- **Admission webhook unit tests** (`pkg/clusteragent/admission/mutate/autoinstrumentation/`): to assert that the webhook correctly mutates pod specs.
- **E2E tests** (`test/new-e2e/tests/ssi/`): to assert that SSI is correctly applied to pods created in a live Kubernetes cluster.

Typical unit test pattern:

```go
import "github.com/DataDog/datadog-agent/pkg/ssi/testutils"

func TestInjection(t *testing.T) {
    mutatedPod := runWebhook(inputPod) // call the webhook under test

    v := testutils.NewPodValidator(mutatedPod, testutils.InjectionModeInitContainer)
    v.RequireInjection(t, []string{"my-app-container"})
    v.RequireLibraryVersions(t, map[string]string{"java": "v1.42.0"})
    v.RequireInjectorVersion(t, "0.54.0")
}
```

## Module boundary

`pkg/ssi/testutils` has its own `go.mod` (`module github.com/DataDog/datadog-agent/pkg/ssi/testutils`). This ensures the Kubernetes client-go dependency it brings in does not affect the main agent module's dependency graph. When adding a new dependency here, update `pkg/ssi/testutils/go.mod` and `go.sum` rather than the root `go.mod`.

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/clusteragent/admission` | [admission.md](../clusteragent/admission.md) | The `mutate/autoinstrumentation` webhook in `pkg/clusteragent/admission` is the production code that `pkg/ssi/testutils` validates. Unit tests under `pkg/clusteragent/admission/mutate/autoinstrumentation/` construct a `PodValidator` with the expected `InjectionMode` and call `RequireInjection` / `RequireLibraryVersions` to assert the webhook's pod mutation output. When adding a new injection mode or changing mutation logic in the webhook, a corresponding update to the validators here is required. |
| `pkg/fleet/installer` | [installer.md](../fleet/installer.md) | The APM library packages managed by `pkg/fleet/installer` (e.g. `datadog-apm-inject`, `datadog-apm-library-java`) are the artifacts that the `init_container` and `image_volume` injection modes deliver into pods. The `InjectionModeImageVolume` mode in particular corresponds to OCI image volumes whose content is produced by the installer's OCI layer extraction pipeline. E2E tests in `test/new-e2e/tests/ssi/` use `PodValidator` to confirm that the correct package version from the installer's stable slot is reflected in the injected container images. |
