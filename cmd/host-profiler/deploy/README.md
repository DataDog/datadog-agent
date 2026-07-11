# Deploying Datadog Host Profiler (Preview)

> **Preview:** These deployment instructions use a preview Host Profiler image. Capabilities and configuration may change before general availability.

The Full Host Profiler collects CPU profiles across all processes, regardless of language or runtime. It runs as a DaemonSet directly on your Kubernetes nodes.

The profiler is an OpenTelemetry Collector distribution powered by the [OpenTelemetry eBPF Profiler](https://github.com/open-telemetry/opentelemetry-ebpf-profiler).

## Supported environments

The Host Profiler is supported in Kubernetes environments that meet these requirements:

| Requirement              | Supported values                          |
|--------------------------|-------------------------------------------|
| Operating system         | Linux                                     |
| Kernel                   | 5.10 or later                             |
| CPU architecture         | amd64 or arm64                            |
| Kubernetes workload type | Host-level DaemonSet with `hostPID: true` |

Common supported setups include self-managed Kubernetes clusters, Amazon EKS with EC2 nodes, GKE Standard, and AKS VM node pools.

This preview does not support direct host or VM installation, Docker without Kubernetes, serverless runtimes, or Kubernetes modes that cannot run host-level DaemonSets, such as Amazon EKS on Fargate, GKE Autopilot, and AKS virtual nodes.

## Deployment

Choose the guide that matches how your Kubernetes cluster is managed:

| If your cluster...                                                                   | Use this guide                                   | What it deploys                                                    |
|--------------------------------------------------------------------------------------|--------------------------------------------------|--------------------------------------------------------------------|
| Already runs the Datadog Agent with Helm, including DDOT deployments                 | [Datadog Helm chart](bundled/helm.md)            | Adds the profiler as a sidecar to the Agent DaemonSet.             |
| Already runs the Datadog Agent with the Datadog Operator, including DDOT deployments | [Datadog Operator](bundled/operator.md)          | Adds the profiler as a sidecar to the Agent DaemonSet.             |
| Does not run the Datadog Agent and you use Helm                                      | [OpenTelemetry Helm chart](standalone/helm.md)   | Deploys the profiler as its own OpenTelemetry Collector DaemonSet. |
| Does not run the Datadog Agent and you use the OpenTelemetry Operator                | [OpenTelemetry Operator](standalone/operator.md) | Deploys the profiler as its own OpenTelemetry Collector DaemonSet. |

If the Datadog Agent is already installed, use one of the Datadog Agent paths so the Agent can enrich profiles with infrastructure metadata. Otherwise, use one of the OpenTelemetry paths.

## Recommended workload metadata

Set `OTEL_SERVICE_NAME` or `DD_SERVICE` on workloads so profiles appear under meaningful service names. Set `DD_ENV` and `DD_VERSION` for richer filtering in the Datadog Profiler UI.

For details, see [How does the Host Profiler name services?](faq.md#how-does-the-host-profiler-name-services).

## After deployment

Profiles appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes after deployment.

If profiles do not appear, see [Troubleshooting](troubleshooting.md).

## Learn more

- [Service names and tags](faq.md#how-does-the-host-profiler-name-services)
- [Deploying without the Datadog Agent or DDOT](faq.md#can-i-deploy-the-host-profiler-without-the-datadog-agent-or-ddot)
- [How the Host Profiler relates to OpenTelemetry](faq.md#how-does-the-host-profiler-relate-to-opentelemetry)
- [Overhead and resource usage](faq.md#what-overhead-should-i-expect)
- [Debug symbols](faq.md#do-i-need-debug-symbols)
- [Security privileges](faq.md#what-security-privileges-does-the-host-profiler-require)
