# pkg/compliance — Compliance Monitoring (CIS Benchmarks / CSPM)

## Purpose

`pkg/compliance` implements the **compliance sub-agent** that runs continuously inside the
security-agent process. It scans the host, containers, and Kubernetes components for
misconfigurations and compliance violations, then reports findings to the Datadog backend
as log events.

The package evaluates rules from benchmark files (YAML) using two evaluation engines:

- **Rego** (Open Policy Agent) — primary engine for most CIS / custom benchmarks.
- **XCCDF / OpenSCAP** — optional engine for host-level OS hardening benchmarks (Linux only,
  requires the `libopenscap` build flag or an external `oscap` binary).

Beyond rule evaluation, the agent also periodically exports **resource configurations** for
Kubernetes nodes, APT package management, and database processes (PostgreSQL, MongoDB,
Cassandra). These exports are used by the Datadog Cloud Security Posture Management (CSPM)
product.

Entry point: `StartCompliance` in `compliance.go` creates an `Agent`, wires it up to the
log reporter, and calls `Agent.Start()`.

## Key elements

### Core types

| Type | File | Description |
|------|------|-------------|
| `Agent` | `agent.go` | Main coordinator. Starts goroutines for Rego benchmarks, XCCDF benchmarks, and each enabled configuration exporter. Implements `startstop.Stoppable`. |
| `AgentOptions` | `agent.go` | Configuration passed to `NewAgent`: config directory, reporter, check interval, enabled exporters, resolver options. |
| `ConfigurationExporter` | `agent.go` | Enum: `KubernetesExporter`, `AptExporter`, `DBExporter`. Controls which configuration export loops are started. |
| `Benchmark` | `data.go` | A set of `Rule` objects that share a framework ID, version and tags. Loaded from YAML files via `LoadBenchmarks`. |
| `Rule` | `data.go` | Single compliance rule: ID, description, list of `InputSpec` entries, rego module imports, scope constraints, SECL filters. |
| `InputSpec` | `data.go` | Union type describing what data to collect: `File`, `Process`, `Group`, `Audit`, `Docker`, `KubeApiserver`, `Package`, `XCCDF`, or `Constants`. |
| `CheckEvent` | `data.go` | Payload sent to the backend for each rule evaluation: rule ID, framework, result, resource type/ID, container metadata. |
| `ResourceLog` | `data.go` | Payload sent to the backend for configuration snapshots (K8s, APT, DB). |
| `ResolvedInputs` | `data.go` | `map[string]interface{}` passed to the Rego evaluator; always contains a `"context"` key of type `ResolvingContext`. |

### Interfaces

| Interface | File | Description |
|-----------|------|-------------|
| `Resolver` | `resolver.go` | `ResolveInputs(ctx, rule) (ResolvedInputs, error)` — collects all data needed by a rule. Default implementation in `defaultResolver`. |
| `LinuxAuditClient` | `resolver.go` | `GetFileWatchRules() ([]*FileWatchRule, error)` — abstraction for the Linux Audit subsystem. |
| `SysProbeClient` | `sysprobe.go` | `FetchDBConfig(ctx, pid) (*dbconfig.DBResource, error)` — fetches DB config from system-probe for containerized processes. |
| `RuleFilter` | `data.go` | `func(*Rule) bool` — predicate applied when loading benchmarks to skip irrelevant rules. |

### Check results

```go
const (
    CheckPassed  CheckResult = "passed"
    CheckFailed  CheckResult = "failed"
    CheckError   CheckResult = "error"
    CheckSkipped CheckResult = "skipped"
)
```

### Rule scopes

```go
const (
    Unscoped               RuleScope = "none"
    DockerScope            RuleScope = "docker"
    KubernetesNodeScope    RuleScope = "kubernetesNode"
    KubernetesClusterScope RuleScope = "kubernetesCluster"
)
```

Rules with a scope are automatically skipped when the corresponding client (Docker daemon,
kube-apiserver) is unavailable.

### Evaluators

| Symbol | File | Description |
|--------|------|-------------|
| `EvaluateRegoRule` | `evaluator_rego.go` | Runs a Rego program against `ResolvedInputs`. Queries `data.datadog.findings`. Build constraint: `//go:build unix`. |
| `EvaluateXCCDFRule` | `evaluator_xccdf.go` | Runs `oscap` (or the embedded libopenscap) for XCCDF rules. Build constraint: requires `libopenscap` CGO tag or falls back to subprocess. |
| Rego helpers | `evaluator_rego.go` | Built-in `datadog` package with `passed_finding`, `failing_finding`, `skipped_finding`, `error_finding`, `raw_finding`. Available to all rules. |
| `parse_octal` | `evaluator_rego.go` | Custom Rego builtin that converts an octal string to an integer (used for file permission checks). |

### Sub-packages

#### `aptconfig/`

Parses `/etc/apt/apt.conf` and `apt.conf.d/` fragments along with systemd timer unit files
(`apt-daily.timer`, `apt-daily-upgrade.timer`). Only runs on Ubuntu hosts (guarded by
`SeclFilter = "os.id == \"ubuntu\""`).

- `LoadConfiguration(ctx, hostroot) (types.ResourceType, interface{})` — main entry point.
- Returns `types.ResourceTypeHostAPTConfig` with a map containing `apt` and `systemd.timers` keys.

#### `dbconfig/`

Extracts configuration from running database processes (identified by process name):

| Database | Process | Config source |
|----------|---------|---------------|
| PostgreSQL | `postgres` | `postgresql.conf` (follows `include` directives) |
| MongoDB | `mongod` | `/etc/mongod.conf` (YAML); sensitive flags redacted |
| Cassandra | `java` (CassandraDaemon) | `/etc/cassandra/cassandra.yaml` + logback XML |

Key types: `DBConfig`, `DBResource`. Key functions: `LoadConfiguration`, `LoadDBResourceFromPID`,
`GetProcResourceType`.

For containerized databases, configuration is fetched via `SysProbeClient.FetchDBConfig` (the
host agent cannot enter the container's filesystem namespace directly).

#### `k8sconfig/`

Reads the configuration of every Kubernetes control-plane component on the current node by
inspecting running process command-line flags and associated files (manifests, kubeconfigs,
admission configs, encryption provider configs, certificates).

- `LoadConfiguration(ctx, hostroot) (types.ResourceType, *K8sNodeConfig)` — main entry point.
- Detects the managed environment (EKS, GKE, AKS) and sets the returned `ResourceType`
  accordingly (`ResourceTypeAwsEksWorkerNode`, etc.).
- `K8sNodeConfig` contains structured sub-configs for: etcd, kube-apiserver,
  kube-controller-manager, kube-scheduler, kubelet, kube-proxy, manifests, kubeconfig files,
  and TLS certificates (fingerprints, SANs, expiry dates).

#### `scap/`

Thin wrapper over `github.com/gocomply/scap` that provides Go types for OpenSCAP documents:

- `Document` — union type holding whichever SCAP document type was decoded (XCCDF Benchmark,
  OVAL definitions/results/syschar, CPE dictionary, Source Data Stream, OCIL).
- `ReadDocument(r io.Reader) (*Document, error)` — decodes an XML stream.
- `SysChar(doc *Document) (*SystemCharacteristics, error)` — extracts a simplified
  `SystemCharacteristics` (system info + collected objects) from an OVAL syschar document.

### Configuration keys

```yaml
compliance_config:
  enabled: true
  dir: /etc/datadog-agent/compliance.d   # benchmark YAML files
  check_interval: 20m
  metrics:
    enabled: false
  xccdf:
    enabled: false        # or host_benchmarks.enabled
  database_benchmarks:
    enabled: false
```

### Build flags

| Flag | Effect |
|------|--------|
| `unix` | Enables the Rego evaluator (`evaluator_rego.go`). |
| `libopenscap` + `cgo` + `linux` | Uses the embedded libopenscap C bindings for XCCDF. Without this combination the XCCDF evaluator shells out to the `oscap` binary. |
| `!linux` | `inputs_audits_nolinux.go` stubs out Linux Audit input resolution. |
| `!docker` | `inputs_docker_nodocker.go` stubs out Docker input resolution. |

## Usage

### How the compliance agent starts

1. `StartCompliance` (called from `cmd/security-agent/subcommands/start`) reads
   `compliance_config.*` from `datadog.yaml`.
2. It constructs `ResolverOptions` (Docker/audit providers, host root, statsd client) and
   creates a `LogReporter` that feeds findings into the Datadog logs pipeline.
3. `NewAgent` + `Agent.Start()` launches:
   - `runRegoBenchmarks` — loads `*.yaml` files from `ConfigDir`, filters rules, then loops
     every `CheckInterval` (default 20 min). For each rule it calls `NewResolver`,
     `resolver.ResolveInputs`, and `EvaluateRegoRule`.
   - `runXCCDFBenchmarks` — same loop but lower priority interval (default 3 h) and calls
     `EvaluateXCCDFRule`.
   - One goroutine per enabled `ConfigurationExporter` calling the corresponding `Load*`
     function.
4. Results are sent via `LogReporter.ReportEvent` to a Datadog log intake endpoint.

### Rule jitter

To avoid all agents running simultaneously, each benchmark loop calls `sleepRandomJitter`
which derives a deterministic random offset (up to 10% of the interval) from an FNV hash of
`(runCount, hostname, frameworkID)`.

### Adding a new configuration exporter

1. Add a new `ConfigurationExporter` constant in `agent.go`.
2. Implement a `Load*` function in a new sub-package returning `(types.ResourceType, interface{})`.
3. Wire it into `Agent.Start()` and expose it through `AgentOptions.EnabledConfigurationExporters`.

### Adding a new Rego benchmark

Place a `<framework>.yaml` file in the compliance config directory (`compliance_config.dir`).
For each rule, create a matching `<rule-id>.rego` file in the same directory. The Rego module
must define `data.datadog.findings` as a set of objects produced by the
`passed_finding` / `failing_finding` / `skipped_finding` helpers.

### Testing

```bash
# Unit tests (no special build tags required)
dda inv test --targets=./pkg/compliance/...

# XCCDF integration tests (Linux with OpenSCAP installed)
dda inv test --targets=./pkg/compliance/... --build-tags=libopenscap
```

---

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/security/secl` | [`security/secl.md`](../security/secl.md) | SECL (Security Evaluation and Control Language) is the rule DSL shared between CWS and compliance. Compliance uses SECL filter expressions (`Rule.SeclFilter`) to scope rules to a host OS (e.g. `os.id == "ubuntu"`) before loading them. The same `eval` package compiles and evaluates these filter predicates. |
| `pkg/security` | [`security/security.md`](../security/security.md) | The compliance agent runs inside the same `security-agent` binary as CWS. `pkg/security` owns the CWS probe, rule engine, and gRPC servers; `pkg/compliance` owns the benchmark evaluation loop. Both share the `LogReporter`-based findings pipeline and are started in the same `StartCompliance` / `StartCWS` sequence from `cmd/security-agent`. |
| `pkg/sbom` | [`sbom.md`](../sbom.md) | SBOM scanning and compliance are complementary CSPM workflows. SBOM inventories packages on container images; compliance checks the configuration of those images and host components. Both write findings into the Datadog CSPM backend. The `pkg/sbom` package is separately invoked from workloadmeta collectors, but compliance can consume container image metadata to scope Kubernetes-cluster rules. |
| `comp/core/workloadmeta` | [`comp/core/workloadmeta.md`](../../comp/core/workloadmeta.md) | The `KubernetesNodeScope` and `KubernetesClusterScope` rules consume Kubernetes resource state. The compliance resolver (`defaultResolver`) uses container metadata from workloadmeta to resolve `Docker` and `KubeApiserver` input specs and to determine whether Docker/kube-apiserver clients are available for a rule's scope. |

### SECL filter scoping

Compliance rules that carry a `SeclFilter` field are evaluated against a per-host context
before the rule enters the benchmark loop. For example, the APT configuration exporter
uses:

```yaml
secl_filter: 'os.id == "ubuntu"'
```

This prevents the benchmark from running on non-Ubuntu hosts without requiring separate
benchmark files. The filter is compiled by `pkg/security/secl/compiler/eval` and
evaluated once per host, not per event.
