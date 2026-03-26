> **TL;DR:** Thin adapter exposing kubelet-derived hostname and host-alias helpers to the agent, returning the Kubernetes node name (optionally suffixed with the cluster name) without requiring callers to import the lower-level kubelet package directly.

# pkg/util/kubelet

## Purpose

`pkg/util/kubelet` is a thin adapter that exposes **kubelet-derived hostname
and host-alias helpers** to the rest of the agent without requiring callers to
import the lower-level `pkg/util/kubernetes/kubelet` package directly.

Specifically, it answers two questions asked during agent startup:

1. **What is the agent's hostname on this Kubernetes node?**
   (`GetHostname`) — returns `<nodeName>-<clusterName>` when a cluster name is
   available, or just `<nodeName>` when it is not.
2. **What host aliases should be associated with this host?**
   (`GetHostAliases`) — returns the same hostname string wrapped in a
   validated alias slice, used by host metadata collection.

The package is deliberately small.  Heavy kubelet interaction (pod listing,
container inspection, health probes) lives in `pkg/util/kubernetes/kubelet`.

## Key elements

### Key functions

All exported functions are defined in both the real and the stub file.

| Function | Signature | Description |
|---|---|---|
| `GetHostname` | `(ctx context.Context) (string, error)` | Returns the Kubernetes-based hostname. Returns `""` (no error) when the `Kubernetes` env feature is not detected. |
| `GetHostAliases` | `(ctx context.Context) ([]string, error)` | Wraps `GetHostname` and validates the result with `validate.ValidHostname`. Returns a single-element slice on success. |
| `GetMetaClusterNameText` | `(ctx context.Context, hostname string) string` | Returns a human-readable cluster name string for the agent status page. When the raw cluster name contained underscores (non-RFC-1123), the string is formatted as `<compliant> (original name: <raw>)`. Returns `""` when Kubernetes is not detected. |

**RFC 1123 cluster name normalisation**

Kubernetes cluster names from EKS and AKS sometimes contain underscores, which
are not valid in DNS labels (RFC 1123).  `GetHostname` replaces `_` with `-`
when building the hostname suffix.  If the replacement produces an
RFC-1123-invalid result (e.g. trailing `-`), the cluster name suffix is dropped
entirely and the plain node name is returned.

**Dependency on `pkg/util/kubernetes/kubelet`**

The real implementation delegates to `k.GetKubeUtil()` (from
`pkg/util/kubernetes/kubelet`) to obtain the node name.  The getter is stored
in a package-level variable (`kubeUtilGet`) so tests can inject a mock without
starting a real kubelet connection.

### Configuration and build flags

| Build tag | Behaviour |
|---|---|
| `//go:build kubelet` | Full implementation: queries `KubeUtilInterface.GetNodename`, resolves cluster name. |
| `//go:build !kubelet` | Stub: `GetHostname` and `GetHostAliases` return an error; `GetMetaClusterNameText` returns `""`. |

## Usage

Import path: `github.com/DataDog/datadog-agent/pkg/util/kubelet`

**Hostname resolution** (`pkg/util/hostname/container.go`) assigns
`kubelet.GetHostname` as the implementation for the `"kubelet"` hostname
provider:

```go
kubeletGetHostname = kubelet.GetHostname
```

**Cloud-provider host alias collection** (`pkg/util/cloudproviders/cloudproviders.go`)
registers `GetHostAliases` as the callback for the `"kubelet"` alias provider,
which runs during host metadata collection to associate the Kubernetes node name
with the host.

**Host metadata** (`comp/metadata/host/hostimpl/utils/meta.go`) calls
`GetMetaClusterNameText` to display the cluster name (with the original name
when it was normalised) in the agent status output.

### Testing

Inject a mock `KubeUtilInterface` by overriding the package-level variable:

```go
kubeUtilGet = func() (k.KubeUtilInterface, error) {
    return myMock, nil
}
```

Use `env.SetFeatures(t, env.Kubernetes)` to activate the Kubernetes code path
and `clustername.ResetClusterName()` to clear the cached cluster name between
subtests.  Tests must use the `kubelet` build tag (the test file carries
`//go:build kubelet`).

---

## Related packages

| Package / component | Relationship |
|---------------------|--------------|
| [`pkg/util/kubernetes`](../util/kubernetes.md) | The shared Kubernetes helpers package. `pkg/util/kubelet` delegates cluster-name normalisation to `pkg/util/kubernetes/clustername` (RFC-1123 underscore→hyphen replacement). `pkg/util/kubernetes` in turn lists `pkg/util/kubelet.GetHostname` in its cross-reference table as the kubelet-based hostname provider. |
| [`pkg/util/hostname`](../util/hostname.md) | The hostname resolution chain registers `kubelet.GetHostname` as the `"container"` provider at position 7 in the chain (after GCE/Azure, before OS and EC2). `kubelet.GetHostAliases` is registered as the kubelet alias callback in `pkg/util/cloudproviders`. `pkg/util/hostname/validate.ValidHostname` is called by `GetHostAliases` before returning results. |
| [`comp/core/workloadmeta`](../../comp/core/workloadmeta.md) | workloadmeta's built-in `kubelet` collector (`comp/core/workloadmeta/collectors/internal/kubelet`) wraps the lower-level `pkg/util/kubernetes/kubelet` package to populate `KubernetesPod` and `KubeletMetrics` entities. `pkg/util/kubelet` (this package) is a separate, thin adapter layer that only handles hostname/alias resolution and is **not** the same package as the workloadmeta kubelet collector. |
