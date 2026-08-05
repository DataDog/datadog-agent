# Agent Sandbox Stage A/B MVP

Agent Sandbox provides a local macOS Apple Virtualization.framework-backed Ubuntu VM for inspecting published Datadog Agent artifacts.

Supported modes:

- Host Agent: installs a published Datadog Agent host package with the real installer script.
- Kubernetes Agent: starts single-node k3s and deploys a published Datadog Agent container image with Helm.

State is stored under `$HOME/.dd-agent-dev/sandbox` by default. Use `--state-root` to place it elsewhere.

## Host Agent demo

```bash
dda inv sandbox.cache-prepare
dda inv sandbox.up --name demo
dda inv sandbox.ssh --name demo --cmd "sudo agent status"
dda inv sandbox.logs --name demo --lines 50
dda inv sandbox.down --name demo
```

## Kubernetes Agent demo

```bash
dda inv sandbox.up --name k8s --kubernetes --agent-image gcr.io/datadoghq/agent:7.80.1
dda inv sandbox.status --name k8s
dda inv sandbox.kubeconfig --name k8s
KUBECONFIG=$HOME/.dd-agent-dev/sandbox/instances/k8s/kubeconfig kubectl get pods -A
dda inv sandbox.logs --name k8s --lines 50
dda inv sandbox.down --name k8s
```

## Useful commands

- `sandbox.up` — create, boot, SSH-discover, install/deploy the selected Agent artifact, and wait for readiness.
- `sandbox.up --kubernetes` — create a k3s sandbox, deploy the Agent chart, wait for Agent DaemonSet readiness, and export kubeconfig.
- `sandbox.status` — show high-level state and useful next commands.
- `sandbox.ssh` — open direct SSH with managed credentials.
- `sandbox.ssh --cmd "sudo agent status"` — run a command inside a host Agent sandbox.
- `sandbox.kubeconfig` — export and print the host-usable kubeconfig path for a Kubernetes sandbox.
- `sandbox.logs` — show recent host Agent journal logs or Kubernetes Agent pod logs.
- `sandbox.down` — stop and destroy instance state while preserving caches.
- `sandbox.cache-prepare` — build helper/base caches ahead of a demo.
- `sandbox.fx-spans --summary` — summarize captured Fx startup spans from a host sandbox created with `--fx-trace`.
- `sandbox.benchmark --granular` — run the local benchmark flow with readiness markers.

## Startup tracing

Create a host sandbox with Fx tracing enabled:

```bash
dda inv sandbox.up --name fx --fx-trace
dda inv sandbox.fx-spans --name fx --summary
```

`--fx-trace` creates a small local trace intake inside the guest before the Agent starts. The published Agent sends Fx startup spans to `127.0.0.1:8126`, and the sandbox stores them in `/var/log/datadog/fx-trace-spans.jsonl`.

## Current performance profile

A hot Host Agent run with prepared base, fakeintake, and apt archive cache is roughly 32–33s to `agent status` readiness.

A fresh Kubernetes Agent run currently takes roughly 2 minutes to k3s readiness, Helm deployment, Agent DaemonSet rollout, and kubeconfig export. Subsequent runs against an already-provisioned k3s sandbox are faster because k3s, Helm, chart state, and images are already present in the mutable instance.
