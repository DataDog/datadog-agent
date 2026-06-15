# Deploying Datadog Host Profiler (Preview)

> **Preview:** These deployment instructions use a preview Host Profiler image. Capabilities and configuration may change before general availability.

The Full Host Profiler collects CPU profiles across all processes, regardless of language or runtime. It runs as a DaemonSet directly on your Kubernetes nodes.

The profiler is an OpenTelemetry Collector distribution powered by the [OpenTelemetry eBPF Profiler](https://github.com/open-telemetry/opentelemetry-ebpf-profiler).

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
| Already runs the Datadog Agent with Helm, including DDOT deployments | [Datadog Helm chart](bundled/helm.md)            | Adds the profiler as a sidecar to the Agent DaemonSet.             |
| Already runs the Datadog Agent with the Datadog Operator, including DDOT deployments | [Datadog Operator](bundled/operator.md)          | Adds the profiler as a sidecar to the Agent DaemonSet.             |
| Does not run the Datadog Agent and you use Helm                       | [OpenTelemetry Helm chart](standalone/helm.md)   | Deploys the profiler as its own OpenTelemetry Collector DaemonSet. |
| Does not run the Datadog Agent and you use the OpenTelemetry Operator | [OpenTelemetry Operator](standalone/operator.md) | Deploys the profiler as its own OpenTelemetry Collector DaemonSet. |

If the Datadog Agent is already installed, use one of the Datadog Agent paths so the Agent can enrich profiles with infrastructure metadata. Otherwise, use one of the OpenTelemetry paths.

If something isn't working, see [Troubleshooting](troubleshooting.md).

## Frequently asked questions

### Summary

- Set `OTEL_SERVICE_NAME` or `DD_SERVICE` on workloads so profiles appear under meaningful [service names](#how-does-the-host-profiler-name-services). Set `DD_ENV` and `DD_VERSION` for richer filtering in the Datadog Profiler UI. Support for equivalent metadata from `OTEL_RESOURCE_ATTRIBUTES` is in progress.
- You can [deploy the Host Profiler without the Datadog Agent or DDOT](#can-i-deploy-the-host-profiler-without-the-datadog-agent-or-ddot).
- Host profiling is [not included directly in DDOT](#why-isnt-host-profiling-included-directly-in-ddot) to keep host-level profiling privileges scoped to a profiling-focused component.
- The Host Profiler is built on [OpenTelemetry profiling components](#how-does-the-host-profiler-relate-to-opentelemetry) and exports profiles to Datadog through OTLP.
- The Datadog Host Profiler distribution [adds Datadog integration, native symbol upload, and tested defaults](#what-does-the-datadog-host-profiler-distribution-add).
- For compiled languages, keep [debug symbols](#do-i-need-debug-symbols) available or upload them separately for readable function names.
- The Host Profiler [does not run privileged](#what-security-privileges-does-the-host-profiler-require), but it needs host-level process visibility and specific Linux capabilities; seccomp and AppArmor provide additional hardening.

### How does the Host Profiler name services?

The Host Profiler determines each process's service name from the `OTEL_SERVICE_NAME` or `DD_SERVICE` environment variables.

If neither variable is set, the profiler infers the service name from the binary name. For interpreted languages, this is the interpreter name, such as `java` or `python`. If multiple services share the same interpreter and do not set a service name, their profiles are grouped under the same inferred name.

Set `OTEL_SERVICE_NAME` or `DD_SERVICE` on each workload to identify services separately:

```yaml
env:
  - name: OTEL_SERVICE_NAME
    value: my-service
```

Set `DD_ENV` and `DD_VERSION` on your workloads for richer filtering in the Datadog Profiler UI. Support for equivalent metadata from `OTEL_RESOURCE_ATTRIBUTES` is in progress.

### Can I deploy the Host Profiler without the Datadog Agent or DDOT?

Yes. The OpenTelemetry Helm chart and OpenTelemetry Operator guides deploy the Host Profiler as its own OpenTelemetry Collector DaemonSet. They do not require a pre-existing Datadog Agent, DDOT Collector, or other Agent-side Datadog component on the host.

You can run this Host Profiler DaemonSet alongside other Collector distributions. If the Datadog Agent is already installed, including deployments that also run the Datadog Distribution of OpenTelemetry (DDOT), use one of the Datadog Agent deployment paths so the Agent can enrich profiles with infrastructure metadata.

### Why isn't host profiling included directly in DDOT?

Host-wide eBPF profiling requires elevated host access, such as `hostPID: true`, host kernel mounts, and eBPF-related Linux capabilities. Keeping the Host Profiler in a profiling-focused distribution avoids granting that access to a larger general-purpose Collector distribution and reduces the amount of code running with host-level privileges.

This follows the upstream OpenTelemetry approach: the eBPF profiler is packaged as a dedicated distribution rather than being included directly in `otelcol-contrib`.

### How does the Host Profiler relate to OpenTelemetry?

The Host Profiler is built as an OpenTelemetry Collector distribution for host-wide eBPF profiling. It uses the [OpenTelemetry eBPF Profiler](https://github.com/open-telemetry/opentelemetry-ebpf-profiler) and exports profile telemetry to Datadog through OTLP HTTP.

Datadog is a major contributor to the OpenTelemetry profiling ecosystem, including:

- the OTLP Profiles signal, which reached alpha in April 2026. See the [OpenTelemetry profiles alpha announcement](https://opentelemetry.io/blog/2026/profiles-alpha/) and [KubeCon talk](https://youtu.be/TKp2snmgvtQ?si=lhQ-n7lREvPCqF6Y);
- the [OpenTelemetry eBPF Profiler](https://github.com/open-telemetry/opentelemetry-ebpf-profiler);
- OpenTelemetry specification work for [process](https://github.com/open-telemetry/opentelemetry-specification/pull/4719) and [thread](https://github.com/open-telemetry/opentelemetry-specification/pull/4947) context sharing, which supports trace-to-profile correlation.

### What does the Datadog Host Profiler distribution add?

The Datadog Host Profiler distribution packages upstream OpenTelemetry profiling components with Datadog integration and tested defaults.

It provides:

- native debug symbol upload to Datadog when symbols are available locally, so native frames for languages such as C, C++, and Rust can be symbolized without a separate CI symbol-upload setup;
- curated defaults and components for Datadog export and container tagging;
- a version tested at scale on thousands of hosts for overhead and stability.

The long-term goal is to make these benefits available with upstream OpenTelemetry distributions directly. The preview distribution provides them out of the box today.

### Do I need debug symbols?

For compiled languages such as C, C++, Rust, and Go, debug symbols are required for function names to appear in profiles.

The Host Profiler uploads debug symbols to Datadog when they are available. If your production binaries are stripped, upload symbols from your build artifacts separately:

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

### What security privileges does the Host Profiler require?

The Host Profiler does not run as a privileged container.

In Kubernetes, it runs as a host-level DaemonSet with `hostPID: true` so it can observe processes on the node. The deployment manifests add only the Linux capabilities required for eBPF-based host profiling.

Seccomp and AppArmor provide additional hardening:

- **seccomp** restricts the syscalls the container can make beyond what those capabilities allow. The seccomp profile ships at `/etc/dd-host-profiler/seccomp.json` inside the image and is applied automatically in the Helm-based guides and the OpenTelemetry Operator guide. In the Datadog Operator preview path, seccomp is optional and must be provisioned manually if you want the extra hardening; a future Operator version is expected to configure it by default.
- **AppArmor** is optional where available. The provided profile restricts which binaries the profiler can execute: only `objcopy`, used for symbol extraction, is permitted.
