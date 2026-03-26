# pkg/util/fargate

## Purpose

`pkg/util/fargate` answers two questions for any agent component that runs inside a Fargate container:

1. **Am I in sidecar mode?** — used to suppress hostname reporting, because in Fargate the task or pod is the unit of identity rather than a host.
2. **What is the right hostname?** — the hostname convention differs between ECS Fargate (`fargate_task:<TaskARN>`), EKS Fargate (Kubernetes node name), and ECS Managed Instances in sidecar mode (`sidecar_host:<TaskARN>`).

Detection relies entirely on feature flags set by `pkg/config/env` (e.g. `env.ECSFargate`, `env.EKSFargate`, `env.ECSManagedInstances`), so the package itself contains no network calls or environment-variable parsing — those are done earlier during agent startup.

## Key elements

### Types

| Symbol | Description |
|--------|-------------|
| `OrchestratorName` (`string`) | Discriminated string type for the detected orchestrator. |
| `ECS`, `EKS`, `ECSManagedInstances`, `Unknown` | `OrchestratorName` constants covering every supported Fargate variant. |

### Functions

| Symbol | Build tag | Description |
|--------|-----------|-------------|
| `IsSidecar() bool` | — | Returns `true` when the agent is running as a sidecar — ECS Fargate, EKS Fargate, or ECS Managed Instances configured with `ecs_deployment_mode: sidecar`. Callers use this to skip hostname resolution and host-level tagging. |
| `GetOrchestrator() OrchestratorName` | — | Returns the active orchestrator (`ECS`, `EKS`, `ECSManagedInstances`, or `Unknown`) by checking the feature-flag set. |
| `GetEKSFargateNodename() (string, error)` | — | Reads `kubernetes_kubelet_nodename` from agent config (injected via `DD_KUBERNETES_KUBELET_NODENAME` using the Kubernetes downward API). Returns an error with a descriptive message if the variable is missing. |
| `GetFargateHost(ctx) (string, error)` | `fargateprocess` | Returns the hostname the **process-agent** should use. Routes to the appropriate helper based on `GetOrchestrator()`. Without the `fargateprocess` tag the function returns `("", nil)` as a no-op. |

### Hostname conventions (fargateprocess tag only)

| Orchestrator | Hostname format |
|--------------|----------------|
| `ECS` | `fargate_task:<TaskARN>` (from ECS metadata v2) |
| `EKS` | value of `kubernetes_kubelet_nodename` |
| `ECSManagedInstances` | `sidecar_host:<TaskARN>` (from ECS metadata v4) |

### Build tags

| Tag | Effect |
|-----|--------|
| `fargateprocess` | Activates the real `GetFargateHost` implementation that calls ECS metadata endpoints. Without this tag, `GetFargateHost` is a no-op returning `("", nil)`. |

## Usage

### Sidecar detection

Many components check `IsSidecar()` to decide whether to emit a hostname:

```go
import "github.com/DataDog/datadog-agent/pkg/util/fargate"

if fargate.IsSidecar() {
    // omit hostname; use task/pod identity instead
}
```

Examples in the codebase: `pkg/process/checks/host_info.go`, `pkg/util/hostname/common.go`, `pkg/util/tags/static_tags.go`, `comp/trace/config/impl/setup.go`.

### Orchestrator discrimination

```go
switch fargate.GetOrchestrator() {
case fargate.ECS:
    // ECS-specific path
case fargate.EKS:
    // EKS-specific path
case fargate.ECSManagedInstances:
    // managed instances path
}
```

### Process-agent hostname (fargateprocess builds)

```go
host, err := fargate.GetFargateHost(ctx)
if err != nil {
    log.Warn("could not determine Fargate host:", err)
}
```

## Related packages

| Package / component | Relationship |
|---|---|
| [`pkg/util/aws`](aws.md) | Provides IAM credential retrieval (`creds.GetSecurityCredentials`) and region detection used when the agent needs to call AWS APIs from a Fargate task. `pkg/util/fargate` handles *identity and orchestrator detection* while `pkg/util/aws/creds` handles *AWS API authentication*. |
| [`pkg/util/ecs`](ecs.md) | The `fargateprocess`-tagged `GetFargateHost` implementation calls ECS Task Metadata Service (TMDS) endpoints via `pkg/util/ecs` to resolve the Task ARN used in the `fargate_task:<TaskARN>` and `sidecar_host:<TaskARN>` hostname formats. `pkg/util/ecs` owns all TMDS HTTP communication. |
| [`pkg/util/hostname`](hostname.md) | The hostname provider chain in `pkg/util/hostname` has a dedicated `fargate` provider at position 3. When `fargate.IsSidecar()` returns `true`, that provider sets the hostname to `""`, preventing any subsequent provider from assigning a host-level name. `fargate.GetEKSFargateNodename()` feeds the `EKS` path of `GetFargateHost`, which is ultimately called during hostname resolution for EKS Fargate deployments. |
