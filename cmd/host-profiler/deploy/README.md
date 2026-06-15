# Deploying Datadog Host Profiler (Preview)

The Full Host Profiler collects CPU profiles across all processes, regardless of language or runtime. In this preview, it runs as a DaemonSet directly on your Kubernetes nodes.

The profiler is an OpenTelemetry Collector distribution powered by the [OpenTelemetry eBPF Profiler](https://github.com/open-telemetry/opentelemetry-ebpf-profiler).

> **Preview:** These deployment instructions use a preview Host Profiler image. Capabilities and configuration may change before general availability.

## Supported environments

This preview is Kubernetes-only.

Your cluster must run nodes that meet all of these requirements:

| Requirement              | Supported values                          |
|--------------------------|-------------------------------------------|
| Operating system         | Linux                                     |
| Kernel                   | 5.10 or later                             |
| CPU architecture         | amd64 or arm64                            |
| Kubernetes workload type | Host-level DaemonSet with `hostPID: true` |

Common supported setups include self-managed Kubernetes clusters, Amazon EKS with EC2 nodes, GKE Standard, and AKS VM node pools.

Not supported in this preview:

- Installing the profiler directly on hosts or VMs, including Docker without Kubernetes.
- Serverless runtimes and container platforms (AWS Lambda, AWS Fargate for ECS, Google Cloud Run, Azure Container Apps, and Azure Functions).
- Kubernetes serverless, virtual-node, or restricted-node modes that do not support host-level DaemonSets (Amazon EKS on Fargate, GKE Autopilot, and AKS virtual nodes backed by Azure Container Instances).

## Deployment

Choose the guide that matches how your Kubernetes cluster is managed:

| If your cluster...                                                    | Use this guide                                   | What it deploys                                                    |
|-----------------------------------------------------------------------|--------------------------------------------------|--------------------------------------------------------------------|
| Already runs the Datadog Agent installed with Helm                    | [Datadog Helm chart](bundled/helm.md)            | Adds the profiler as a sidecar to the Agent DaemonSet.             |
| Already runs the Datadog Agent installed with the Datadog Operator    | [Datadog Operator](bundled/operator.md)          | Adds the profiler as a sidecar to the Agent DaemonSet.             |
| Does not run the Datadog Agent and you use Helm                       | [OpenTelemetry Helm chart](standalone/helm.md)   | Deploys the profiler as its own OpenTelemetry Collector DaemonSet. |
| Does not run the Datadog Agent and you use the OpenTelemetry Operator | [OpenTelemetry Operator](standalone/operator.md) | Deploys the profiler as its own OpenTelemetry Collector DaemonSet. |

If the Datadog Agent is already installed, use one of the Datadog Agent paths so the Agent can enrich profiles with infrastructure metadata. Otherwise, use one of the OpenTelemetry paths.

If something isn't working, see [Troubleshooting](troubleshooting.md).

## Common usage

### Service naming

The host profiler determines each process's service name from its `OTEL_SERVICE_NAME` environment variable.

If `OTEL_SERVICE_NAME` is not set, the profiler infers the service name from the binary name. For interpreted languages, this is the interpreter name (for example, `java` for a Java process). If multiple services share the same interpreter and none set `OTEL_SERVICE_NAME`, their profiles are grouped under the same inferred name.

Set `OTEL_SERVICE_NAME` on each workload to identify them separately:

```yaml
env:
  - name: OTEL_SERVICE_NAME
    value: my-service
```

#### Datadog-specific naming

The profiler is also compatible with `DD_SERVICE` as replacement for `OTEL_SERVICE_NAME`.

Set `DD_ENV` and `DD_VERSION` for richer filtering in the Profiler UI.

### Manually uploading debug symbols

For compiled languages (C, C++, Rust, Go), the host profiler uploads debug symbols to Datadog for symbolization. Binaries must include debug symbols (not stripped) for function names to appear in profiles.

To upload symbols from stripped binaries:

1. Install the [datadog-ci CLI](https://github.com/DataDog/datadog-ci).
2. Set your API key and site:

```shell
export DD_API_KEY=<DATADOG_API_KEY>
export DD_SITE=<DATADOG_SITE>
```

3. Upload symbols:

```shell
DD_BETA_COMMANDS_ENABLED=1 datadog-ci elf-symbols upload /path/to/build/symbols/
```

## Security

The host profiler does not run privileged. It requests only the specific Linux capabilities it needs (`BPF`, `PERFMON`, `SYS_PTRACE`, `SYS_RESOURCE`, `DAC_READ_SEARCH`, `SYSLOG`, `CHECKPOINT_RESTORE`, `IPC_LOCK`).

**seccomp** further restricts the syscalls the container can make beyond what those capabilities allow. The profile ships at `/etc/dd-host-profiler/seccomp.json` inside the image and is applied automatically in the Helm and standalone deployment paths. In the Datadog Operator preview path, seccomp is optional and must be provisioned manually if you want the extra hardening; a future Operator version is expected to configure it by default.

**AppArmor** is optional extra hardening where available. It restricts which binaries the profiler can exec: only `objcopy`, used for symbol extraction, is permitted.
