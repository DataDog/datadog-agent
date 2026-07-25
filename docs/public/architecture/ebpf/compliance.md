# Compliance and SBOM

-----

This page covers two security products that share infrastructure with [Workload Protection](cws.md) but are otherwise independent of it. **Compliance** (CSPM — Cloud Security Posture Management) periodically evaluates benchmark rules — CIS Docker, CIS Kubernetes, host hardening — against the resolved state of the host, container runtime, and Kubernetes components, and reports pass/fail findings to a dedicated intake. **SBOM** (Software Bill of Materials) generates CycloneDX inventories of the packages installed in container images and on hosts using an embedded [Trivy](https://github.com/aquasecurity/trivy), stores them in [workloadmeta](../containers/workloadmeta.md), and ships them through the [event platform forwarder](../pipelines/event-platform.md) for vulnerability analysis.

Neither product depends on eBPF. Compliance runs in the security-agent by default (optionally in [system-probe](system-probe.md) or, for cluster-scope rules, in the [Cluster Agent](../containers/cluster-agent.md)); the vulnerability-oriented SBOM path runs entirely in the core agent. The one crossover is CWS's runtime-usage SBOM enrichment, which runs in system-probe and is described at the end of this page.

## Key packages and files

| Path | Purpose |
|---|---|
| [`pkg/compliance/agent.go`](<<<SRC>>>/pkg/compliance/agent.go) | Compliance agent: benchmark loading, scheduling, rule filtering |
| [`pkg/compliance/resolver.go`](<<<SRC>>>/pkg/compliance/resolver.go) | Resolves rule inputs: files, processes, commands, audit, docker, kubeapiserver, packages, constants |
| [`pkg/compliance/evaluator_rego.go`](<<<SRC>>>/pkg/compliance/evaluator_rego.go) | Rego (OPA) rule evaluation |
| [`pkg/compliance/evaluator_xccdf.go`](<<<SRC>>>/pkg/compliance/evaluator_xccdf.go) | XCCDF evaluation by driving a long-lived `oscap-io` (OpenSCAP) helper process |
| [`pkg/compliance/reporter.go`](<<<SRC>>>/pkg/compliance/reporter.go) | `LogReporter`: headless logs pipeline to `cspm-intake.<site>`, track `compliance` |
| [`pkg/compliance/k8sconfig`](<<<SRC>>>/pkg/compliance/k8sconfig), [`aptconfig`](<<<SRC>>>/pkg/compliance/aptconfig), [`dbconfig`](<<<SRC>>>/pkg/compliance/dbconfig) | Configuration exporters: Kubernetes component configs, APT config, database application configs |
| [`pkg/compliance/sysprobe.go`](<<<SRC>>>/pkg/compliance/sysprobe.go) ↔ [`cmd/system-probe/modules/compliance.go`](<<<SRC>>>/cmd/system-probe/modules/compliance.go) | Cross-container DB-config resolution through system-probe (`GET /compliance/dbconfig?pid=N`) |
| [`cmd/security-agent/subcommands/compliance`](<<<SRC>>>/cmd/security-agent/subcommands/compliance) | `security-agent compliance check` manual runs; hidden `oscap-exec` capability-dropping self-exec ([`oscapexec.go`](<<<SRC>>>/cmd/security-agent/subcommands/compliance/oscapexec.go)) |
| [`cmd/cluster-agent/subcommands/start/compliance.go`](<<<SRC>>>/cmd/cluster-agent/subcommands/start/compliance.go) | Cluster-agent runs `KubernetesClusterScope` benchmarks (leader only) |
| [`pkg/compliance/statusregistry`](<<<SRC>>>/pkg/compliance/statusregistry/registry.go) | Status text registration when compliance runs inside system-probe |
| [`pkg/sbom/scanner/scanner.go`](<<<SRC>>>/pkg/sbom/scanner/scanner.go) | Global SBOM scanner: work queue, backoff, disk-space guard rails |
| [`pkg/sbom/collectors`](<<<SRC>>>/pkg/sbom/collectors) | Scan collectors: `docker`, `containerd`, `crio`, `host`, `procfs` (behind the `trivy` build tag) |
| [`pkg/util/trivy`](<<<SRC>>>/pkg/util/trivy) | Embedded Trivy pipeline: image-layer access, overlayfs direct scan, custom cache |
| [`comp/core/workloadmeta/init/init.go`](<<<SRC>>>/comp/core/workloadmeta/init/init.go) | Creates the global scanner when `sbom.host.enabled` or `sbom.container_image.enabled` |
| [`comp/core/workloadmeta/collectors/internal/containerd/image_sbom_trivy.go`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/containerd/image_sbom_trivy.go) (+ docker, crio variants) | Subscribes to image events and enqueues scans; stores results on image entities |
| [`pkg/collector/corechecks/sbom`](<<<SRC>>>/pkg/collector/corechecks/sbom) | The long-running `sbom` core check: workloadmeta → `SBOMPayload` protos → event platform |
| [`comp/forwarder/eventplatform/impl/pipelines_sbom.go`](<<<SRC>>>/comp/forwarder/eventplatform/impl/pipelines_sbom.go) | The `container-sbom` event-platform pipeline |
| [`pkg/security/resolvers/sbom/resolver.go`](<<<SRC>>>/pkg/security/resolvers/sbom/resolver.go) | CWS SBOM resolver: per-workload rootfs scans plus runtime usage marking |
| [`comp/core/workloadmeta/collectors/internal/remote/sbomcollector`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/remote/sbomcollector/sbom_collector.go) | Core-agent collector streaming runtime-usage SBOMs from system-probe |
| [`pkg/proto/datadog/sbom/workloadmeta_sbom.proto`](<<<SRC>>>/pkg/proto/datadog/sbom/workloadmeta_sbom.proto) | `SBOMCollector` gRPC contract between system-probe and the core agent |
| [`cmd/sbomgen`](<<<SRC>>>/cmd/sbomgen) | Standalone CLI for generating SBOMs |

## Compliance (CSPM)

### Benchmarks and rule filtering

Benchmarks are YAML files loaded from `compliance_config.dir` (default `/etc/datadog-agent/compliance.d`). The benchmark and Rego content is **not in this repository** — it ships from [DataDog/security-agent-policies](https://github.com/DataDog/security-agent-policies) and is baked into packages and images at build time; the engine in [`pkg/compliance`](<<<SRC>>>/pkg/compliance) only schedules and evaluates.

Before a rule runs, `MakeDefaultRuleFilter` decides whether it applies to this node: Kubernetes-scoped rules are skipped off-Kubernetes and vice versa, Docker CIS rules are skipped when the kubelet's CRI is not Docker (with a fail-open on unknown — a GKE COS gotcha), and SECL-style `filters:` expressions are evaluated against the same synthetic host model CWS uses ([`pkg/security/rules/filtermodel`](<<<SRC>>>/pkg/security/rules/filtermodel)).

### Evaluation: Rego and XCCDF

Each rule declares typed inputs — file contents and permissions, running processes, command output, auditd rules, Docker API objects, Kubernetes API objects, installed packages, constants — resolved by [`resolver.go`](<<<SRC>>>/pkg/compliance/resolver.go) into a JSON document. Two evaluators consume them:

1. **Rego** ([`evaluator_rego.go`](<<<SRC>>>/pkg/compliance/evaluator_rego.go)): the resolved inputs are fed to embedded [OPA](https://www.openpolicyagent.org/) policies. A full benchmark pass runs every `compliance_config.check_interval` (20 min default), throttled to one rule per 2 s, with per-host jitter so fleets do not thunder.
1. **XCCDF** ([`evaluator_xccdf.go`](<<<SRC>>>/pkg/compliance/evaluator_xccdf.go)): host OS benchmarks (CIS RHEL, Ubuntu, ...) are standard SCAP content evaluated by OpenSCAP. Enabled by `compliance_config.host_benchmarks.enabled` (default true; legacy alias `xccdf.enabled`), running every 3 h at low priority. The engine spawns one long-lived **`oscap-io`** helper per XCCDF file and keeps it alive through 60 min of inactivity; the helper is re-exec'd through the hidden `security-agent compliance oscap-exec` subcommand, which drops capabilities before running OpenSCAP ([`oscapexec.go`](<<<SRC>>>/cmd/security-agent/subcommands/compliance/oscapexec.go)).

Findings are `CheckEvent`s sent through the `LogReporter` ([`reporter.go`](<<<SRC>>>/pkg/compliance/reporter.go)) — a headless [logs pipeline](../pipelines/logs.md) posting to `cspm-intake.<site>` on the `compliance` track with the `agent-json` protocol. Alongside findings, dedicated goroutines export raw configurations for backend-side evaluation: Kubernetes component configurations ([`k8sconfig`](<<<SRC>>>/pkg/compliance/k8sconfig) — kubelet, API server, etcd flags and files), APT configuration ([`aptconfig`](<<<SRC>>>/pkg/compliance/aptconfig)), and database application configurations ([`dbconfig`](<<<SRC>>>/pkg/compliance/dbconfig), behind `compliance_config.database_benchmarks.enabled`).

### Cross-container database checks via system-probe

Database benchmark checks need to read config files inside *other* containers via `/proc/<pid>/root`, which requires root — more privilege than the security-agent has. The security-agent therefore delegates: [`sysprobe.go`](<<<SRC>>>/pkg/compliance/sysprobe.go) calls `GET /compliance/dbconfig?pid=N` on the system-probe Unix socket, answered by system-probe's `compliance` module ([`cmd/system-probe/modules/compliance.go`](<<<SRC>>>/cmd/system-probe/modules/compliance.go)), which performs the privileged file reads.

### Where compliance runs

| Location | When | Scope |
|---|---|---|
| security-agent (default) | `compliance_config.enabled: true` | All host/container benchmarks; started by `StartCompliance` from [`cmd/security-agent/subcommands/start/command.go`](<<<SRC>>>/cmd/security-agent/subcommands/start/command.go) |
| system-probe | `compliance_config.run_in_system_probe: true` | Same benchmarks, run in-process where root privileges make resolution direct (uses a `LocalSysProbeClient`; status is exposed through [`statusregistry`](<<<SRC>>>/pkg/compliance/statusregistry/registry.go) for the remote-agent status integration) |
| cluster-agent | `compliance_config.enabled` in the DCA, leader only | Only `KubernetesClusterScope` rules, evaluated against the API server with reflector-cached objects ([`compliance.go`](<<<SRC>>>/cmd/cluster-agent/subcommands/start/compliance.go)) |

Node-scope and cluster-scope are complementary: running the cluster-agent checks does not remove the need for the per-node security-agent benchmarks.

Manual runs and debugging: `security-agent compliance check <rule-id>` evaluates one rule and prints the resolved inputs and outcome; the engine's counters are exposed on the security-agent expvar port (5011) under the `compliance` expvar map.

## SBOM and container image scanning

### The vulnerability path, end to end

```text
 workloadmeta image event            core agent
 (containerd/docker/crio) --------> image_sbom_trivy.go collector
                                        |
                                        v enqueue ScanRequest
                                 pkg/sbom/scanner (global queue,
                                 exponential backoff, disk guards)
                                        |
                                        v
                                 pkg/util/trivy (embedded Trivy:
                                 mount/export layers, analyzers)
                                        |
                                        v CycloneDX BOM
                                 workloadmeta ContainerImageMetadata
                                        |
                                        v subscription
                                 sbom core check (pkg/collector/corechecks/sbom)
                                        |
                                        v SBOMPayload proto
                                 event platform, track "container-sbom"
```

1. **Scanner creation**: [`comp/core/workloadmeta/init/init.go`](<<<SRC>>>/comp/core/workloadmeta/init/init.go) creates the global SBOM scanner in the core agent when `sbom.host.enabled` or `sbom.container_image.enabled` is set. It is a single Kubernetes-style work queue with exponential backoff (`sbom.scan_queue.*`) and disk-space guard rails (`sbom.container_image.check_disk_usage`, `.min_available_disk`) — scans are refused when the host is low on disk.
1. **Enqueueing**: the workloadmeta image collectors ([`image_sbom_trivy.go`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/containerd/image_sbom_trivy.go) for containerd, plus docker and crio variants) subscribe to image-set events and enqueue a scan request per new image.
1. **Scanning**: the per-runtime collectors in [`pkg/sbom/collectors`](<<<SRC>>>/pkg/sbom/collectors) (compiled behind the `trivy` and runtime-specific build tags) drive [`pkg/util/trivy`](<<<SRC>>>/pkg/util/trivy), an embedded Trivy pipeline that accesses image layers (mounting them, exporting to tarball, or scanning overlayfs layer directories directly when `sbom.container_image.overlayfs_direct_scan` is set, which avoids the expensive export) and runs the configured analyzers (`sbom.container_image.analyzers`, default `os` — language analyzers are opt-in). Trivy's cache lives under `sbom.cache_directory` (`${run_path}/sbom-agent`).
1. **Storage**: results convert via `ScanResult.ConvertScanResultToSBOM` and land on the `ContainerImageMetadata` entity in workloadmeta as a CycloneDX BOM.
1. **Shipping**: the long-running `sbom` core check ([`pkg/collector/corechecks/sbom`](<<<SRC>>>/pkg/collector/corechecks/sbom), gated by `sbom.enabled` plus its check config) subscribes to workloadmeta container and image events, batches BOMs into `SBOMPayload` protobufs, and emits them with `sender.EventPlatformEvent(..., eventplatform.EventTypeContainerSBOM)` onto the `container-sbom` [event platform](../pipelines/event-platform.md) pipeline. Entities re-emit periodically — container images every `ContainerPeriodicRefreshSeconds`, hosts every `HostPeriodicRefreshSeconds` — with heartbeats in between so the backend can distinguish "unchanged" from "gone".

**Host SBOM** (`sbom.host.enabled`) is the same pipeline pointed at the host filesystem: the `host` collector scans `/` (or `$HOST_ROOT` when containerized). [`cmd/sbomgen`](<<<SRC>>>/cmd/sbomgen) packages the scanning code as a standalone CLI for generating SBOMs outside the Agent.

### CWS runtime-usage enrichment

A separate SBOM exists inside system-probe for a different question: not "what is installed" but "what is actually *used* at runtime". Setting `sbom.enrichment.usage.enabled` in `datadog.yaml`:

1. Force-enables the `event_monitor` module in system-probe. If runtime security is otherwise disabled, only the `UsageConsumer` ([`pkg/security/module/usage_consumer.go`](<<<SRC>>>/pkg/security/module/usage_consumer.go)) registers — the eBPF probe and SBOM resolver come up with **no rule engine**, deliberately outside CWS billing.
1. The CWS SBOM resolver ([`pkg/security/resolvers/sbom/resolver.go`](<<<SRC>>>/pkg/security/resolvers/sbom/resolver.go)) computes per-workload SBOMs of container root filesystems (embedded Trivy again, tuned by `runtime_security_config.sbom.*`) and marks packages and files as used when exec/open events touch them.
1. The `SBOMCollector` gRPC service (contract in [`workloadmeta_sbom.proto`](<<<SRC>>>/pkg/proto/datadog/sbom/workloadmeta_sbom.proto)) is registered on the CWS cmd socket. The core agent's remote workloadmeta collector ([`sbomcollector`](<<<SRC>>>/comp/core/workloadmeta/collectors/internal/remote/sbomcollector/sbom_collector.go)) streams these reports and merges usage properties — `LastSeenRunning`, `HasSetSuidBit`, `RunningAsRoot` — into the workloadmeta SBOM entities, so the same `sbom` check ships them with the usage data attached.

There is also an experimental `runtime_security_config.sbom.generate_policies` flag (off by default) that generates SECL policies from SBOM contents and injects them through CWS's bundled policy provider via silent reloads; it is disabled because matching every package file can create heavy open-event load.

## Configuration

**Compliance** (`datadog.yaml` / `security-agent.yaml`; defaults in [`pkg/config/setup/common_settings.go`](<<<SRC>>>/pkg/config/setup/common_settings.go)):

| Key | Default | Meaning |
|---|---|---|
| `compliance_config.enabled` | `false` | Master switch (security-agent and cluster-agent) |
| `compliance_config.dir` | `/etc/datadog-agent/compliance.d` | Benchmark directory |
| `compliance_config.check_interval` | `20m` | Rego benchmark cycle |
| `compliance_config.host_benchmarks.enabled` | `true` | XCCDF/OpenSCAP host benchmarks |
| `compliance_config.database_benchmarks.enabled` | `false` | DB-config export (needs system-probe for cross-container reads) |
| `compliance_config.run_in_system_probe` | `false` | Move the engine into system-probe |
| `compliance_config.endpoints.*` | — | Intake overrides |
| `compliance_config.container_include` / `container_exclude` | — | Container filtering |

**SBOM** (`datadog.yaml`, core agent):

| Key | Default | Meaning |
|---|---|---|
| `sbom.enabled` | `false` | Gates the `sbom` core check |
| `sbom.container_image.enabled` | `false` | Scan container images |
| `sbom.container_image.analyzers` | `[os]` | Trivy analyzers to run |
| `sbom.container_image.overlayfs_direct_scan` | `false` | Scan overlayfs layers in place instead of exporting |
| `sbom.container_image.check_disk_usage` / `min_available_disk` | — | Disk guard rails |
| `sbom.host.enabled` | `false` | Host filesystem SBOM |
| `sbom.cache_directory` | `${run_path}/sbom-agent` | Trivy cache |
| `sbom.scan_queue.*` | — | Queue backoff tuning |
| `sbom.enrichment.usage.enabled` | `false` | CWS runtime-usage enrichment (system-probe side) |

## Deployment-mode notes

1. **Linux host**: compliance in the security-agent unit; SBOM in the core agent; `oscap-io` ships in the Agent package.
1. **Kubernetes DaemonSet**: benchmark files are baked into the images (node benchmarks in the agent image, CIS Kubernetes content in the cluster-agent image); the SBOM scanner needs access to the runtime's image store (containerd socket or overlayfs host mounts) for image scanning, and `HOST_ROOT` for host SBOMs.
1. **Cluster Agent**: only cluster-scope compliance, gated on leader election — see [Cluster Agent](../containers/cluster-agent.md).
1. **Windows/macOS**: no compliance benchmarks; container-image SBOM scanning is a Linux feature (host SBOM support exists on Windows through the WMI-based host collector in [`pkg/sbom/collectors/host`](<<<SRC>>>/pkg/sbom/collectors/host)).

## Gotchas

1. **Benchmark content lives out-of-repo** — if a rule seems wrong, the fix usually belongs in [DataDog/security-agent-policies](https://github.com/DataDog/security-agent-policies), not `pkg/compliance`.
1. **Docker CIS rules silently skip on Kubernetes when the CRI is not Docker**, and the check fails open when the CRI cannot be determined; a node reporting no Docker findings is often correct behavior, not a bug.
1. **XCCDF keeps `oscap-io` alive** for up to 60 min of inactivity — a resident OpenSCAP process on the host is expected, not a leak. Capability dropping happens through the hidden `security-agent compliance oscap-exec` self-exec.
1. **Two Trivy instances can run on one host**: the core agent's image/host scanner and CWS's runtime-usage resolver in system-probe are separate embeddings with separate caches and config knobs (`sbom.*` vs `runtime_security_config.sbom.*`).
1. **`sbom.enrichment.usage.enabled` starts the eBPF probe** even with CWS disabled — do not be surprised to see the event-monitor module loaded in system-probe on hosts that "only" do SBOM.
1. **Image scans are disk- and IO-hungry**: without `overlayfs_direct_scan` the image is exported to a tarball first; the disk-usage guard rails exist because early versions filled small root volumes.
1. **The `sbom` check is long-running and event-driven**, unlike normal interval [checks](../checks/collector.md) — it subscribes to workloadmeta rather than polling, and periodic re-emission is its own scheduling, not the collector's.
