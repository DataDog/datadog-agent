# Agent Sandbox Stage A MVP

Agent Sandbox provides a local macOS Apple Virtualization.framework-backed Ubuntu VM for inspecting a published Datadog Agent host package.

## Quick demo

```bash
dda inv sandbox.cache-prepare
dda inv sandbox.up --name demo
dda inv sandbox.ssh --name demo --cmd "sudo agent status"
dda inv sandbox.logs --name demo --lines 50
dda inv sandbox.down --name demo
```

State is stored under `$HOME/.dd-agent-dev/sandbox` by default. Use `--state-root` to place it elsewhere.

## Useful commands

- `sandbox.up` — create, boot, SSH-discover, install Agent, and wait for `agent status` readiness.
- `sandbox.status` — show high-level state and useful next commands.
- `sandbox.ssh` — open direct SSH with managed credentials.
- `sandbox.ssh --cmd "sudo agent status"` — run a command inside the sandbox.
- `sandbox.logs` — show recent `datadog-agent` journal logs.
- `sandbox.down` — stop and destroy instance state while preserving caches.
- `sandbox.cache-prepare` — build helper/base caches ahead of a demo.
- `sandbox.fx-spans --summary` — summarize captured Fx startup spans from a sandbox created with `--fx-trace`.
- `sandbox.benchmark` — run the local Stage A benchmark flow.

## Startup tracing

Create a sandbox with Fx tracing enabled:

```bash
dda inv sandbox.up --name fx --fx-trace
dda inv sandbox.fx-spans --name fx --summary
```

`--fx-trace` creates a small local trace intake inside the guest before the Agent starts. The published Agent sends Fx startup spans to `127.0.0.1:8126`, and the sandbox stores them in `/var/log/datadog/fx-trace-spans.jsonl`.

## Current performance profile

A hot run with prepared base and apt archive cache is roughly:

- ~4s: APFS clone + cloud-init seed generation
- ~6s: VM config validation
- ~18s: SSH reachable
- ~55s: Agent binary present
- ~75s: cloud-init done and service active
- ~100s: `agent status` ready

Dominant remaining costs are the production installer flow and Agent startup topology.
