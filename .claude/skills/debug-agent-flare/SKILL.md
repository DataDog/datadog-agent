---
name: debug-agent-flare
description: Analyze a Datadog Agent flare archive to identify issues across all agent components
allowed-tools: Bash, Read, Glob, Grep
argument-hint: "<path-to-flare.zip or flare-directory/> [issue description]"
---

Analyze the Datadog Agent flare provided in `$ARGUMENTS`. Split arguments into:
- `FLARE_PATH`: the first token (file or directory path)
- `ISSUE`: everything after the first token (optional — the specific problem to investigate)

If `ISSUE` is provided, orient the analysis around it. Otherwise, do a broad sweep.

## Step 1: Prepare the flare directory

If `FLARE_PATH` ends in `.zip`, extract it to a temp directory. The archive contains a single top-level directory named after the hostname — that is your `ROOT` for all subsequent reads.

## Step 2: Understand what's in the flare

List `ROOT` to see what's present, then read what's relevant. The flare has this structure:

**Top-level files** — start here for orientation:
- `status.log` — full agent status at collection time, organized by component (core agent, APM, DogStatsD, forwarder, logs agent, Fleet Automation, Remote Configuration, etc.)
- `health.yaml` — which components are healthy or unhealthy
- `runtime_config_dump.yaml` — the full resolved configuration (all sources merged)
- `envvars.log` — environment variables visible to the agent at runtime
- `diagnose.log` — connectivity and self-diagnostic checks
- `install_info.log` — installation method and tool version (e.g. Helm chart version)
- `version-history.json` — agent version history on this host
- `secrets.log` — secrets backend configuration and resolution status
- `remote-config-state.log` — Remote Configuration subsystem state
- `flare_creation.log` — metadata about the flare itself

**`logs/`** — log files for each agent process that was running:
- `agent.log` — core agent
- `trace-agent.log` — APM / Trace Agent
- `process-agent.log` — Process Agent
- `system-probe.log` — System Probe
- `security-agent.log` — Security Agent (if enabled)

**`expvar/`** — runtime statistics exported by each agent process at collection time. Each file is typically JSON (but may contain an error string if the process was unreachable). Key files: `agent`, `trace-agent`, `process-agent`. These contain live counters: forwarder transaction stats, receiver stats, writer stats, memory/CPU usage, sampling rates, check run stats, etc.

**`etc/`** — configuration files on disk: `datadog.yaml`, `system-probe.yaml`, `security-agent.yaml`, and the `conf.d/` directory with check configurations.

**`k8s/`** — Kubernetes-specific data (if running on K8s): `kubelet_pods.yaml` (full pod specs including all container definitions, env vars, volumes) and `kubelet_config.yaml`.

**`metadata/`** — host and agent inventory metadata sent to the Datadog backend: `host.json`, `inventory/agent.json` (feature flags, configuration summary), `inventory/host.json`.

**`system-probe/`** — system-probe specific diagnostics (eBPF, conntrack, network, SELinux, etc.)

**`otel/`** — OpenTelemetry collector data, if enabled.

Other files that may be present: `docker_ps.log`, `tagger-list.json`, `workload-list.log`, `process_check_output.json`, `config-check.log`, `go-routine-dump.log`, `permissions.log`, `sbom/`, `profiles/`.

## Step 3: Analyze

Read the files that are relevant to the `ISSUE` (or all key files if no issue was given). Approach:

1. **Get oriented** — version, hostname, platform, uptime, which sub-agents are present
2. **Check overall health** — `health.yaml` and the component sections in `status.log`
3. **Read the configuration** — `runtime_config_dump.yaml` and `envvars.log` together tell the full story; when deployed via Helm or another orchestrator, `etc/datadog.yaml` is often minimal and env vars carry most of the config. In Kubernetes, `k8s/kubelet_pods.yaml` shows the actual per-container env vars and can reveal differences between containers in the same pod.
4. **Read the logs** — prioritize files relevant to the issue; look for errors and warnings, startup messages, connectivity failures, and recurring patterns
5. **Read the expvar files** — runtime counters show what the agent is actually doing: is it receiving data? sending it? are there errors or retries?
6. **Check diagnostics** — `diagnose.log` for connectivity failures; `metadata/inventory/agent.json` for feature flags the agent reports to the backend; subsystem-specific files (`remote-config-state.log`, `system-probe/`, `otel/`, etc.) as relevant to the issue

## Step 4: Report

Present findings as:

**Summary** — agent version, hostname, platform, uptime, sub-agents present

**Critical issues** — things that definitively break functionality. For each: what was observed (cite specific values from the flare), where it came from, what it means, how to fix it.

**Warnings** — degraded or misconfigured behavior, same format.

**Observations** — useful context that isn't a problem.

**Recommendations** — concrete next steps, ordered by priority.

If `ISSUE` was provided, explicitly address it: does the flare confirm the problem, what evidence supports or contradicts it, and what is the likely root cause?

Be specific — quote actual values from the flare rather than speaking in generalities. If a key file is missing or unreadable, note it and explain what that implies.
