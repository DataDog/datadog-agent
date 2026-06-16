# Agent Sandbox Stage A MVP

Agent Sandbox provides a local macOS Apple Virtualization.framework-backed Ubuntu VM for inspecting a published Datadog Agent host package.

## Quick demo

```bash
dda inv agent-sandbox.build-helper
dda inv agent-sandbox.prepare-base
dda inv agent-sandbox.create --name demo
dda inv agent-sandbox.agent --name demo --args status
dda inv agent-sandbox.logs --name demo --lines 50
dda inv agent-sandbox.destroy --name demo
```

State is stored under `$HOME/.dd-agent-dev/sandbox` by default. Use `--state-root` to place it elsewhere.

## Useful commands

- `agent-sandbox.build-helper` — compile and sign the Swift Virtualization.framework helper. Skips rebuild when up to date.
- `agent-sandbox.prepare-base` — build a reusable Ubuntu base with OS dependencies prebaked.
- `agent-sandbox.create` — create, boot, SSH-discover, install Agent, and wait for `agent status` readiness.
- `agent-sandbox.ssh` — open direct SSH with managed credentials.
- `agent-sandbox.agent --args status` — run an Agent command through managed SSH.
- `agent-sandbox.logs` — show recent `datadog-agent` journal logs.
- `agent-sandbox.fx-spans --summary` — summarize captured Fx startup spans from a sandbox created with `--fx-trace`.
- `agent-sandbox.benchmark` — run the local Stage A benchmark flow.

## Startup tracing

Create a sandbox with Fx tracing enabled:

```bash
dda inv agent-sandbox.create --name fx --fx-trace
dda inv agent-sandbox.fx-spans --name fx --summary
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
