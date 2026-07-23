# Host Profiler FAQ

## Questions

- [How does the Host Profiler name services?](#how-does-the-host-profiler-name-services)
- [Can I deploy the Host Profiler without the Datadog Agent or DDOT?](#can-i-deploy-the-host-profiler-without-the-datadog-agent-or-ddot)
- [Why isn't host profiling included directly in DDOT?](#why-isnt-host-profiling-included-directly-in-ddot)
- [How does the Host Profiler relate to OpenTelemetry?](#how-does-the-host-profiler-relate-to-opentelemetry)
- [What does the Datadog Host Profiler distribution add?](#what-does-the-datadog-host-profiler-distribution-add)
- [What overhead should I expect?](#what-overhead-should-i-expect)
- [How do I configure resource requests and limits?](#how-do-i-configure-resource-requests-and-limits)
- [Do I need debug symbols?](#do-i-need-debug-symbols)
- [What security privileges does the Host Profiler require?](#what-security-privileges-does-the-host-profiler-require)

## How does the Host Profiler name services?

The Host Profiler determines each process's service name from the `OTEL_SERVICE_NAME` or `DD_SERVICE` environment variables.

If neither variable is set, the profiler infers the service name from the binary name. For interpreted languages, this is the interpreter name, such as `java` or `python`. If multiple services share the same interpreter and do not set a service name, their profiles are grouped under the same inferred name.

Set `OTEL_SERVICE_NAME` or `DD_SERVICE` on each workload to identify services separately:

```yaml
env:
  - name: OTEL_SERVICE_NAME
    value: my-service
```

Set `DD_ENV` and `DD_VERSION` on your workloads for richer filtering in the Datadog Profiler UI. Support for equivalent metadata from `OTEL_RESOURCE_ATTRIBUTES` is in progress.

## Can I deploy the Host Profiler without the Datadog Agent or DDOT?

Yes. The OpenTelemetry Helm chart and OpenTelemetry Operator guides deploy the Host Profiler as its own OpenTelemetry Collector DaemonSet. They do not require a pre-existing Datadog Agent, DDOT Collector, or other Agent-side Datadog component on the host.

You can run this Host Profiler DaemonSet alongside other Collector distributions. If the Datadog Agent is already installed, including deployments that also run the Datadog Distribution of OpenTelemetry (DDOT), use one of the Datadog Agent deployment paths so the Agent can enrich profiles with infrastructure metadata.

## Why isn't host profiling included directly in DDOT?

Host-wide eBPF profiling requires elevated host access, such as `hostPID: true`, host kernel mounts, and eBPF-related Linux capabilities. Keeping the Host Profiler in a profiling-focused distribution avoids granting that access to a larger general-purpose Collector distribution and reduces the amount of code running with host-level privileges.

This follows the upstream OpenTelemetry approach: the eBPF profiler is packaged as a dedicated distribution rather than being included directly in `otelcol-contrib`.

## How does the Host Profiler relate to OpenTelemetry?

The Host Profiler is built as an OpenTelemetry Collector distribution for host-wide eBPF profiling. It uses the [OpenTelemetry eBPF Profiler](https://github.com/open-telemetry/opentelemetry-ebpf-profiler) and exports profile telemetry to Datadog through OTLP HTTP.

Datadog is a major contributor to the OpenTelemetry profiling ecosystem, including:

- the OTLP Profiles signal, which reached alpha in April 2026. See the [OpenTelemetry profiles alpha announcement](https://opentelemetry.io/blog/2026/profiles-alpha/) and [KubeCon talk](https://youtu.be/TKp2snmgvtQ?si=lhQ-n7lREvPCqF6Y);
- the [OpenTelemetry eBPF Profiler](https://github.com/open-telemetry/opentelemetry-ebpf-profiler);
- OpenTelemetry specification work for [process](https://github.com/open-telemetry/opentelemetry-specification/pull/4719) and [thread](https://github.com/open-telemetry/opentelemetry-specification/pull/4947) context sharing, which supports trace-to-profile correlation.

## What does the Datadog Host Profiler distribution add?

The Datadog Host Profiler distribution packages upstream OpenTelemetry profiling components with Datadog integration and tested defaults.

It provides:

- native debug symbol upload to Datadog when symbols are available locally, so native frames for languages such as C, C++, and Rust can be symbolized without a separate CI symbol-upload setup;
- curated defaults and components for Datadog export and container tagging;
- a version tested at scale on thousands of hosts for overhead and stability.

The long-term goal is to make these benefits available with upstream OpenTelemetry distributions directly. The preview distribution provides them out of the box today.

## What overhead should I expect?

The Host Profiler is designed to run continuously on every supported node.

In Datadog's internal deployments, the Host Profiler runs on thousands of hosts in densely packed Kubernetes clusters. In that environment, average usage per Host Profiler container is:

- **CPU**: about `15m` (15 millicores)
- **Memory**: about `350 MB`

Actual usage depends on node density, process churn, and debug symbol processing.

The provided manifests set limits of `500m` CPU and `1Gi` memory to cap usage while leaving room for temporary spikes. If you observe sustained usage considerably above these averages, contact Datadog Support so we can help review your workload characteristics and tune the deployment.

## How do I configure resource requests and limits?

The provided manifests set Host Profiler container requests to `0` and limits to `500m` CPU and `1Gi` memory. This caps profiler usage without reserving CPU or memory on every node.

Tune these values when needed:

- **Resource requests**: set nonzero requests if your cluster requires them, or if you want the scheduler to reserve capacity for the profiler.
- **Dense nodes**: increase limits on nodes with many running processes.
- **Large debug symbols**: set the memory limit above the size of the largest debug symbol file, with headroom. During upload, the profiler copies and prepares symbol data before sending it to Datadog.
- **Guaranteed QoS**: set requests equal to limits for every container in the pod, including init containers. In bundled Agent deployments, Agent containers also affect pod QoS.

## Do I need debug symbols?

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

## What security privileges does the Host Profiler require?

The Host Profiler does not run as a privileged container.

In Kubernetes, it runs as a host-level DaemonSet with `hostPID: true` so it can observe processes on the node. The deployment manifests add only the Linux capabilities required for eBPF-based host profiling.

Seccomp and AppArmor provide additional hardening:

- **seccomp** restricts the syscalls the container can make beyond what those capabilities allow. The seccomp profile ships at `/etc/dd-host-profiler/seccomp.json` inside the image and is applied automatically in the Helm-based guides and the OpenTelemetry Operator guide. In the Datadog Operator preview path, seccomp is optional and must be provisioned manually if you want the extra hardening; a future Operator version is expected to configure it by default.
- **AppArmor** is optional where available. The provided profile restricts which binaries the profiler can execute: only `objcopy`, used for symbol extraction, is permitted.
