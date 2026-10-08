# Background: the Datadog Agent and its e2e tests (context for the run/skip decision)

The Datadog Agent is a family of daemons installed on hosts and in Kubernetes
clusters. They collect metrics, traces, logs, process and security events, and
forward them to the Datadog backend. In e2e tests the agent is pointed at a
local fakeintake (a mock backend, `test/fakeintake`) that records and asserts
the submitted payloads. Supported platforms: Linux (amd64/arm64), Windows
(amd64), macOS, Docker and Kubernetes.

## Processes and their main code locations
- agent - the core collector (`cmd/agent`, most of `comp/`): runs integrations
  (Go core checks in `pkg/collector/corechecks`, Python checks via `rtloader`),
  the DogStatsD metrics listener, logs collection, host metadata, and the
  forwarder that ships payloads (`comp/forwarder`, `comp/serializer`).
- trace-agent (`cmd/trace-agent`, `pkg/trace`, `comp/trace`) - APM trace
  collection and OTLP ingestion.
- process-agent (`cmd/process-agent`, `pkg/process`, `comp/process`) - live
  process and container views.
- system-probe (`cmd/system-probe`, `pkg/ebpf`, `pkg/collector/corechecks/ebpf`)
  - eBPF-based network and system monitoring (USM, NDM data, TCP queue
  lengths, connection tracking).
- security-agent (`cmd/security-agent`, `pkg/security`) - CWS runtime
  security; relies on system-probe eBPF programs.
- cluster-agent (`cmd/cluster-agent`, `pkg/clusteragent`, `pkg/orchestrator`)
  - Kubernetes admission controller, orchestrator monitoring, cluster checks.
- dogstatsd - standalone StatsD-only binary.
- installer/updater (`cmd/installer`, `pkg/fleet`, `comp/updater`) - bootstrap
  installs and remote agent management and upgrades (fleet automation).

Shared libraries under `pkg/` are used by several processes; `comp/<name>`
holds per-feature components wired into the binaries; `packages/` declares the
content of each distribution package (deb/rpm/msi); `omnibus/` is the legacy
build system.

## Configuration
`datadog.yaml` fields are declared once in the config schema (`pkg/config`,
`pkg/config/schema`) and rendered into every component; `DD_` environment
variables override them. Changes under `pkg/config/schema` affect ALL
components and ALL suites.

## E2E test suites (`test/new-e2e/tests/<suite>`)
Tests provision real VMs or Kubernetes clusters with `test/e2e-framework`,
install the agent (OS package, install script, bootstrap installer, helm chart
or operator), and verify its behavior - usually by asserting payloads in the
fakeintake. Suite names map to features:
agent-configuration, agent-health, agent-subcommands, agent-data-plane,
agent-platform, agent-runtimes, agent-devx - core agent behavior, CLI,
packaging and installs across OSes (platform jobs encode the distro and
flavor: fips, iot-agent, dogstatsd, ddot); installer, fleet - bootstrap
installer and fleet automation; windows - Windows-specific packaging (MSI);
orchestrator, containers, discovery, autoscaling - k8s deployment and
cluster-agent behavior; cspm - cloud security posture compliance checks; cws,
security-agent-functional, sysprobe-functional - runtime security and eBPF
probes; ndm, usm, npm, netpath - network monitoring (devices, services,
performance, path); apm, language-detection, otel - trace collection and
OTel components; remote-config - remote config client; gpu - GPU monitoring;
sbom, data-security - software inventories and data security features;
ha-agent - high availability; fips-compliance - FIPS builds; anomalydetection
- anomaly detection observer; ssi, ddi, ddot - instrumentation (single-step
injection, DatadogInstrumentation, single-binary distribution).

## Rules of thumb for blast radius
- Affect EVERY e2e test: changes under `test/e2e-framework/**`,
  `test/fakeintake/**`, `test/new-e2e/go.mod`, the root `go.mod`,
  `pkg/config/schema/*`, `flakes.yaml`, `release.json` (versions drive all
  packaging) and `.gitlab/test/e2e/*.yml`.
- Shared code (`pkg/util`, `pkg/telemetry`, `pkg/api`, `pkg/status`,
  `comp/forwarder`, `comp/serializer`, `comp/core`, `comp/dogstatsd`) is used
  by several processes: judge from the feature it implements.
- Component-specific dirs map to that component's suites (see above), e.g.
  `pkg/security` or `pkg/security/ebpf` changes matter for cws, cspm,
  data-security, sbom, security-agent-functional and sysprobe-functional;
  `pkg/trace` for apm and language-detection; `pkg/clusteragent` or
  `pkg/orchestrator` for orchestrator, containers and discovery; `pkg/fleet`
  or `comp/updater` for installer and fleet.
- Tooling-only areas - `tasks/`, `.github/`, `doc/`, `dev/` - usually do not
  change what an e2e test verifies, but `.gitlab/` CI changes can affect a
  suite's own job environment.

This is background only: the decision must still follow the actual changed
files, the PR description or summary, and the specific test under evaluation.
