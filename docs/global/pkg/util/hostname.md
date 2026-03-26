> **TL;DR:** Resolves and caches the agent's hostname via a prioritized provider chain (config, GCE, Azure, kubelet, EC2, OS, etc.), with background drift detection, RFC-1123 validation helpers, and a `validate` sub-package for hostname normalization.

# pkg/util/hostname

## Purpose

`pkg/util/hostname` resolves and caches the agent's hostname — the string that uniquely identifies a host in the Datadog backend. Getting this right is critical: a changed hostname creates a duplicate host in-app, breaking monitors and dashboards.

The package implements a prioritized chain of providers that mirrors the Agent v5 resolution order for backward compatibility. The resolved hostname is cached for the lifetime of the process, and a background drift-detection goroutine periodically re-runs the chain to alert on unexpected changes.

The sub-package `pkg/util/hostname/validate` provides RFC-1123 validation and normalization helpers used both by this package and by callers that handle arbitrary hostnames.

### Relationship to other packages

| Package / component | Role |
|---|---|
| [`pkg/util/ec2`](ec2.md) | Implements the `aws` provider in the chain. `ec2.GetHostname` (the instance ID), `ec2.IsDefaultHostname`, `ec2.GetHostAliases`, and `ec2.IsDefaultHostnameForIntake` are all called from `pkg/util/hostname` to decide whether to use or override an AWS-assigned hostname. |
| [`pkg/util/cloudproviders`](cloudproviders.md) | Implements the `gce` and `azure` providers (positions 4 and 5). Calls to the GCE and Azure IMDS endpoints are made via `cloudproviders/gce` and `cloudproviders/azure` sub-packages. |
| [`pkg/util/kubelet`](kubelet.md) | Implements the `container` provider (position 7) for Kubernetes nodes. `kubelet.GetHostname` returns `<nodeName>` or `<nodeName>-<clusterName>`. |
| [`comp/core/hostname`](../../../comp/core/hostname.md) | The FX-injectable component that wraps this package for use in the component graph. Most agent code should inject `hostname.Component` rather than calling `pkg/util/hostname.Get` directly. Sub-agents wire `remotehostnameimpl` to forward to the core agent over gRPC instead of re-resolving locally. |

## Key elements

### Key types

```go
// Data is an alias for hostnameinterface.Data
type Data struct {
    Hostname string
    Provider string
}
```

`Data` is returned by `GetWithProvider` to bundle the resolved hostname with the name of the provider that produced it.

### Key functions

| Function | Description |
|---|---|
| `Get(ctx) (string, error)` | Returns the cached hostname (or resolves it on first call) |
| `GetWithProvider(ctx) (Data, error)` | Returns hostname plus the name of the provider that resolved it |
| `GetWithLegacyResolutionProvider(ctx) (Data, error)` | Variant used during IMDSv2 transition: resolves without IMDSv2/MDI and caches under a separate key (`legacy_resolution_hostname`) |

`Get` is a thin wrapper around `GetWithProvider`. Both use the same in-process cache so repeated calls are free.

### Provider chain

Providers are tried in order. Early providers have `stopIfSuccessful = true` (the chain halts on success). Later providers (`fqdn`, `container`, `os`, `ec2`) have `stopIfSuccessful = false` so their results can be overridden by a higher-priority downstream provider — notably EC2.

| Order | Provider name | `stopIfSuccessful` | Source |
|---|---|---|---|
| 1 | `config` | yes | `hostname` config key |
| 2 | `hostnameFile` | yes | `hostname_file` config key (file contents) |
| 3 | `fargate` | yes | Fargate/sidecar mode — sets hostname to `""` |
| 4 | `gce` | yes | GCE instance metadata API |
| 5 | `azure` | yes | Azure instance metadata API |
| 6 | `fqdn` | no | `/bin/hostname -f` (Linux) or `GetHostByName` (Windows), gated on `hostname_fqdn` config |
| 7 | `container` | no | kube_apiserver (cluster-agent), Docker, or kubelet |
| 8 | `os` | no | `os.Hostname()` |
| 9 | `aws` | no | EC2 IMDS instance ID (conditional — see below) |

**EC2 special logic**: the EC2 provider runs only when the agent is on an ECS cluster, `ec2_prioritize_instance_id_as_hostname` is `true`, or the OS hostname matches a default EC2 hostname pattern (`ip-...` / `domU...`). This preserves the v5 behavior where non-default OS hostnames are kept as-is.

**Fargate / serverless**: on the `serverless` build tag, the entire provider chain is replaced with a stub that returns `("", nil)`.

### Drift detection

After the hostname is first resolved, a background goroutine re-runs the provider chain:
- After an initial delay (default 20 minutes, overridable via `hostname_drift_initial_delay`).
- Then every 6 hours (overridable via `hostname_drift_recurring_interval`).

Three drift states are tracked:
- `hostname_drift` — hostname changed, provider unchanged
- `provider_drift` — provider changed, hostname unchanged
- `hostname_provider_drift` — both changed

Drift is reported via the `hostname.drift_detected` telemetry counter and the `hostname.drift_resolution_time_ms` histogram. The cache is updated when drift is detected.

### `pkg/util/hostname/validate`

| Function | Description |
|---|---|
| `ValidHostname(hostname) error` | RFC-1123 compliance check; rejects empty, localhost, > 255 chars, or non-compliant strings |
| `NormalizeHost(host) (string, error)` | Liberal sanitization: strips `\n`, `\r`, `\t`; replaces `<`/`>` with `-`; rejects null bytes; enforces 253-char limit |
| `CleanHostnameDir(hostname) string` | Converts a hostname to a safe directory name: replaces non-`[a-zA-Z0-9_-]` chars with `_`, truncates to 32 chars |

### Configuration and build flags

| File | Build tag | Effect |
|---|---|---|
| `providers.go`, `drift.go` | `!serverless` | Full provider chain, drift detection, expvar registration |
| `providers_serverless.go` | `serverless` | Stubs `Get`/`GetWithProvider` to return `("", nil)` |
| `container.go` | `linux \|\| windows \|\| darwin` | Implements the `container` provider (kube_apiserver / docker / kubelet) |
| `container_stub.go` | `freebsd \|\| netbsd \|\| ...` | Always returns error from `container` provider |
| `fqdn_nix.go` | `!windows` | FQDN via `hostname -f` |
| `fqdn_windows.go` | `windows` | FQDN via `GetHostByName` |
| `os_hostname_linux.go` | `linux` | `os.Hostname()` with UTS-namespace awareness |
| `os_hostname_windows.go` | `windows` | `os.Hostname()` Windows variant |
| `os_hostname_others.go` | `!windows && !linux` | Generic `os.Hostname()` |

### Expvar / status page

Under the `!serverless` build, the package registers two expvar entries under the `"hostname"` map:
- `"provider"` — the name of the winning provider.
- `"errors"` — a map of per-provider error messages, shown on the agent status page.

### `isOSHostnameUsable` heuristic

Before returning a hostname from the `fqdn` or `os` provider, the package checks whether the OS hostname is meaningful in this environment:
- Returns `false` if running in a container without host UTS namespace (checked via `/proc/self/ns/uts`).
- Returns `false` if on Kubernetes without `hostNetwork: true`.
- Returns `true` otherwise (bare-metal, VM, or container with `hostNetwork`).

## Usage

### Basic — get the agent hostname

```go
import hostname "github.com/DataDog/datadog-agent/pkg/util/hostname"

h, err := hostname.Get(ctx)
if err != nil {
    return fmt.Errorf("could not determine hostname: %w", err)
}
```

### With provider information (for metadata / status)

```go
data, err := hostname.GetWithProvider(ctx)
// data.Hostname, data.Provider
```

### Validating an arbitrary hostname

```go
import "github.com/DataDog/datadog-agent/pkg/util/hostname/validate"

if err := validate.ValidHostname(h); err != nil {
    return fmt.Errorf("invalid hostname %q: %w", h, err)
}
```

### Normalizing a hostname from an external source

```go
normalized, err := validate.NormalizeHost(rawHost)
```

### Using the component vs. the package directly

Most FX-wired code should inject [`comp/core/hostname`](../../../comp/core/hostname.md) rather than calling `pkg/util/hostname.Get` directly. The component:
- Provides `GetSafe` (returns `"unknown host"` instead of an error) for use during early startup.
- Transparently dispatches to `remotehostnameimpl` in sub-agents (process-agent, security-agent) that forward the request to the core agent over gRPC.
- Is mockable in unit tests via `hostname.Mock`.

Direct use of `pkg/util/hostname.Get` is appropriate only in non-FX code paths (e.g., `pkg/collector/python/datadog_agent.go`) or packages that cannot take an FX dependency.

### Who imports these packages

`pkg/util/hostname.Get` is called from ~38 locations across the codebase:

- **Python check bridge** (`pkg/collector/python/datadog_agent.go`) — exposes the hostname to Python checks via `GetHostname`.
- **Cluster checks dispatcher** (`pkg/clusteragent/clusterchecks`) — uses the hostname to identify the dispatcher node.
- **Orchestrator checks** (`pkg/collector/corechecks/orchestrator/`) — tags pod/node/ECS resources with the agent hostname.
- **Agent worker / collector** (`pkg/collector/worker`) — includes hostname in check metadata.
- **Serverless metrics** (`pkg/serverless/metrics`) — uses the hostname for metric tagging in Lambda environments.
- **Kubernetes state check** (`pkg/collector/corechecks/cluster/ksm`) — sets hostname on KSM metrics.
- **Aggregator** (`pkg/aggregator/demultiplexer_serverless.go`) — uses hostname for serverless demux.
- **SNMP check** (`pkg/collector/corechecks/snmp`) — includes agent hostname in SNMP device reports.

`validate.ValidHostname` is used inside the hostname package itself and by any component that receives a hostname from config or an external API and needs to verify it before use.
