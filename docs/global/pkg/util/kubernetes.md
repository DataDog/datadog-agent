> **TL;DR:** Build-tag-independent Kubernetes helpers providing standard label/annotation constants, workload name-parsing utilities, Unified Service Tagging (UST) extraction, TLS/bearer-token authentication helpers, and resource quantity formatters shared across every agent component that interacts with Kubernetes objects.

# pkg/util/kubernetes

## Purpose

`pkg/util/kubernetes` provides shared, build-tag-independent helpers for working with Kubernetes concepts in the Datadog Agent. It contains:

- Standard label/annotation key constants used across every agent component that interacts with Kubernetes objects.
- Pure-Go utilities for parsing Kubernetes naming conventions (e.g. deriving a Deployment name from a ReplicaSet or pod name).
- TLS / bearer-token helpers for authenticating against in-cluster or external Kubernetes endpoints.
- Unified Service Tagging (UST) helpers that extract `env`, `service`, and `version` tags from object labels.
- Resource formatting helpers (CPU/memory) used when reporting container resource requests.

The package intentionally has no dependency on the Kubernetes API machinery for most files: it is safe to import in any component, including those that run on nodes without `kubeapiserver` or `kubelet` build tags.

## Build tags

| File | Build tag required |
|---|---|
| `auth.go`, `const.go`, `helpers.go`, `tags.go` | none (always compiled) |
| `resource.go` | `kubeapiserver \|\| kubelet` |
| `hostname.go` | `kubeapiserver && !kubelet` |
| `hostname_stub.go` | stub for builds without either tag |

## Key elements

### Key types

There are no exported struct types in the root package. Key constants grouped by category are documented under `### Constants` below. The `FormatCPURequests` and `FormatMemoryRequests` functions accept `k8s.io/apimachinery` `resource.Quantity` values.

### Key functions

See `### Name parsing helpers`, `### Tag extraction`, `### Authentication helpers`, `### Resource formatting`, and `### Hostname resolution` below.

### Configuration and build flags

| File | Build tag required |
|---|---|
| `auth.go`, `const.go`, `helpers.go`, `tags.go` | none (always compiled) |
| `resource.go` | `kubeapiserver \|\| kubelet` |
| `hostname.go` | `kubeapiserver && !kubelet` |
| `hostname_stub.go` | stub for builds without either tag |

---

### Constants (`const.go`)

**Datadog standard label / annotation keys**

| Constant | Value |
|---|---|
| `EnvTagLabelKey` | `tags.datadoghq.com/env` |
| `ServiceTagLabelKey` | `tags.datadoghq.com/service` |
| `VersionTagLabelKey` | `tags.datadoghq.com/version` |
| `ADAnnotationPrefix` | `ad.datadoghq.com/` |
| `ADTagsAnnotation` | `ad.datadoghq.com/tags` |
| `ADContainerTagsAnnotationFormat` | `ad.datadoghq.com/%s.tags` (use with `fmt.Sprintf`) |

**Kubernetes standard label keys**

`KubeAppNameLabelKey`, `KubeAppInstanceLabelKey`, `KubeAppVersionLabelKey`, `KubeAppComponentLabelKey`, `KubeAppPartOfLabelKey`, `KubeAppManagedByLabelKey`

**Workload kind strings**

String constants for every common Kubernetes kind: `PodKind`, `DeploymentKind`, `ReplicaSetKind`, `StatefulSetKind`, `DaemonSetKind`, `JobKind`, `CronJobKind`, `ServiceKind`, `NodeKind`, `HorizontalPodAutoscalerKind`, `VerticalPodAutoscalerKind`, `RolloutKind` (Argo), and more. These are used throughout the codebase wherever a kind string must be compared or emitted without hard-coding a literal.

**Environment variable names for UST**

`EnvTagEnvVar` (`DD_ENV`), `ServiceTagEnvVar` (`DD_SERVICE`), `VersionTagEnvVar` (`DD_VERSION`)

### Name parsing helpers (`helpers.go`)

```go
func ParseDeploymentForReplicaSet(name string) string
func ParseDeploymentForPodName(name string) string
func ParseReplicaSetForPodName(name string) string
func ParseCronJobForJob(name string) (string, int)
```

These functions reverse-engineer the Kubernetes name-generation algorithm (random suffix of known runes) to walk up the owner hierarchy from a pod or job name to its controller name. Used by the tagger and orchestrator check to attach owner tags.

Constants `KubeAllowedEncodeStringAlphaNums` and `Digits` expose the character sets so callers can validate suffixes independently.

### Tag extraction (`tags.go`)

```go
func GetStandardTags(labels map[string]string) []string
```

Reads `EnvTagLabelKey`, `ServiceTagLabelKey`, and `VersionTagLabelKey` from a labels map and returns them as `["env:x", "service:y", "version:z"]` formatted strings. Returns an empty slice when no labels match. Used by every check and tag provider that reads pod/deployment labels.

### Authentication helpers (`auth.go`)

```go
const DefaultServiceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
const DefaultServiceAccountCAPath    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

func GetBearerToken(authTokenPath string) (string, error)
func GetCertificates(certFilePath, keyFilePath string) ([]tls.Certificate, error)
func GetCertificateAuthority(certPath string) (*x509.CertPool, error)
```

Low-level TLS helpers. `GetBearerToken` reads the projected service-account token from disk; `GetCertificateAuthority` builds an `x509.CertPool` from a PEM file. Used by the kubelet client and custom TLS configurations.

### Resource formatting (`resource.go`, build tag: `kubeapiserver || kubelet`)

```go
func FormatCPURequests(cpuRequest resource.Quantity) *float64
func FormatMemoryRequests(memoryRequest resource.Quantity) *uint64
```

Converts `k8s.io/apimachinery` `resource.Quantity` values into plain numeric types (CPU as percentage of a core, memory as bytes) for inclusion in metrics payloads.

### Hostname resolution (`hostname.go`, build tag: `kubeapiserver && !kubelet`)

```go
func GetKubeAPIServerHostname(ctx context.Context) (string, error)
```

Returns a hostname in the form `<nodeName>-<clusterName>` (or just `<nodeName>` when no cluster name is set) by querying the API server for the pod's node and reading the cluster name from `pkg/util/kubernetes/clustername`.

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/util/kubernetes/apiserver`](kubernetes-apiserver.md) | The `apiserver` package is the heavier sibling — it provides the authenticated `APIClient`, informer factories, and leader election on top of the lightweight constants and helpers defined here. Many files in `apiserver` import `pkg/util/kubernetes` for workload kind constants and `GetStandardTags`. |
| [`pkg/util/kubelet`](kubelet.md) | The kubelet hostname helper (`pkg/util/kubelet.GetHostname`) resolves `<nodeName>-<clusterName>` using the cluster-name logic from `pkg/util/kubernetes/clustername`, which normalises RFC-1123 invalid names (underscore → hyphen) before building the suffix. |
| [`comp/core/workloadmeta`](../../comp/core/workloadmeta.md) | The `kubelet` and `kubeapiserver` workloadmeta collectors call `ParseDeploymentForReplicaSet` and `GetStandardTags` when constructing `KubernetesPod` and `KubernetesDeployment` entities so that owner and UST tags are available before the tagger processes them. |

---

## Usage

The package is imported by a wide range of components. Representative examples:

- **Orchestrator pod check** (`pkg/collector/corechecks/orchestrator/pod/`): uses `ParseDeploymentForReplicaSet` and `GetStandardTags` to attach owner and UST tags to orchestrator payloads.
- **KSM check** (`pkg/collector/corechecks/cluster/ksm/`): uses the standard label constants to read pod and deployment labels.
- **Autoscaling workload controller** (`pkg/clusteragent/autoscaling/workload/`): uses `FormatCPURequests` / `FormatMemoryRequests` and the standard label constants when reconciling vertical scaling decisions.
- **Admission controller and AppSec config** (`pkg/clusteragent/appsec/config/`): reads `ADAnnotationPrefix` and related constants to locate injection annotations.
- **CRI check** (`pkg/collector/corechecks/containers/cri/`): imports `CriContainerNamespaceLabel`.
- **Language detection util** (`pkg/languagedetection/util/`): calls `ParseDeploymentForReplicaSet` inside `GetNamespacedBaseOwnerReference` to walk up from a ReplicaSet to its parent Deployment when resolving APM annotation targets.
- **Tagger** (`comp/core/tagger`): the tag store uses `GetStandardTags` to seed `StandardTags` for Kubernetes pod entities.

Typical import pattern (no build tag needed for most uses):

```go
import "github.com/DataDog/datadog-agent/pkg/util/kubernetes"

tags := kubernetes.GetStandardTags(pod.Labels)
deployment := kubernetes.ParseDeploymentForReplicaSet(rs.Name)
```

### Build-tag guidance

Most callers can import `pkg/util/kubernetes` unconditionally — the constants and name-parsing helpers have no build-tag requirements. The two exceptions are:

- `resource.go` (`FormatCPURequests`, `FormatMemoryRequests`): requires `kubeapiserver || kubelet`. Guard call sites with the appropriate build tag or put them in files already restricted to those environments.
- `hostname.go` (`GetKubeAPIServerHostname`): requires `kubeapiserver && !kubelet`. This is only needed in the Cluster Agent hostname provider; do not call it from node-agent code.
