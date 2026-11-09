---
name: injector-dev
description: >-
  Build, deploy, and test Datadog Agent components (agent, cluster-agent, operator, CSI driver)
  on a local Kubernetes cluster using the injector-dev CLI. Use when the user wants to iterate on
  local Agent or Operator change, spin up a local k8s test environment.
model: sonnet
---

# injector-dev

`injector-dev` is a CLI that turns the  manual loop of *build image → push → manage a cluster → deploy workloads by hand*
into a single declarative command. You describe a **scenario** in YAML — which
Agent components to deploy, how to configure them, and what test workloads to
run — and `injector-dev apply` brings the whole environment up on a local
Kubernetes cluster.

Scenarios are reproducible and shareable: tear an environment down and recreate
it identically at any time.

## When to use this skill

- Iterating on local `datadog-agent`, `datadog-operator`, or Helm
  chart changes and needing them running on a real cluster.
- Reproducing an APM auto-instrumentation / injection bug locally.
- Writing, editing, or debugging a `scenario.yaml`.
- Pinning a test environment against a specific released version, a CI pipeline
  artifact, or the agent `main` branch.

## Prerequisites

- A local Kubernetes platform. Supported drivers: **`kind` (recommended)**,
  **`colima`**, **`minikube`**, `nvkind`, and `none` (use an existing
  cluster/context). `kind` is fast, reliable across machines, and the easiest to
  reset. There is also a **`workspace`** driver that runs the cluster on a remote
  Datadog Workspace VM — see [Remote clusters](#remote-clusters-the-workspace-platform).
- A **Docker runtime** installed and running.
- `helm` and `kubectl` on your PATH. `kind` must be installed to use the
  recommended kind platform.
- **Datadog API + App keys** (a real API key is needed for the Agent to report).
  Set them however you prefer:
  ```shell
  export DD_API_KEY="<your-api-key>"
  export DD_APP_KEY="<your-app-key>"
  ```

> If you already have colima and/or minikube running, shut them down before
> using `injector-dev` to avoid conflicts.

## Installation

```shell
git clone https://github.com/DataDog/injector-dev
cd injector-dev
make install   # builds and installs the binary to /usr/local/bin (uses sudo)
```

## Configuration (`~/.injector-dev/config.yaml`)

A practical starting config:

```yaml
---
platform: kind              # kind (recommended) | colima | minikube | nvkind | workspace | none
builder:
  code_root: "/Users/<USER>/dd"   # parent dir containing datadog-agent/, auto_inject/, datadog-operator/, ...
  dev_container:
    name: "injector-dev-builder"
    enabled: true
    persist: true           # keep the build container alive between builds → much faster rebuilds
installer:
  repo_root: "/Users/<USER>/dd/injector-dev"
  # api_key / app_key are optional here; DD_API_KEY / DD_APP_KEY env vars are the fallback.
```

Key points:

- **`builder.code_root`** — parent directory holding your component repos. The
  tool derives each repo path as `code_root/<repo-name>` (e.g.
  `code_root/datadog-agent`), so all repos must be siblings under this dir.
- **`builder.dev_container.persist: true`** — leaves the build container running
  between applies. First build takes several minutes (installing deps); later
  builds drop to ~30 seconds.
- **`installer.repo_root`** — path to your local `injector-dev` checkout.
- **Every config key can be overridden by env var**: prefix with `INJECTOR_DEV_`
  and replace dots with underscores, e.g.
  `INJECTOR_DEV_INSTALLER_API_KEY`, `INJECTOR_DEV_BUILDER_CODE_ROOT`.
- **API/App key resolution order**: `config.yaml` (`installer.api_key`/`app_key`)
  → then `DD_API_KEY` / `DD_APP_KEY` env vars.

### Profiles (multiple configs)

To manage several environments (staging, sandbox, org2, …), drop additional
config files at `~/.injector-dev/<profile>.yaml` and select one per command:

```shell
injector-dev apply -f scenario.yaml --profile sandbox
```

Omitting `--profile` uses `config.yaml`.

## Remote clusters: the `workspace` platform

Instead of a local cluster, `injector-dev` can run the kind cluster on a remote
[Datadog Workspace](https://datadoghq.atlassian.net/wiki/spaces/DEVX/pages/3109585281)
VM. **Image builds still happen locally** — only the cluster and workloads run on
the workspace. Useful when your laptop is resource-constrained or you want a
beefier, disposable environment.

How it works: `docker build` runs locally; the image is streamed to the workspace
(`docker save | ssh … kind load`); `kind` is installed on the workspace
automatically on first use (workspaces ship with docker but not kind); and the
remote cluster's API server is exposed to your machine through a persistent SSH
tunnel, with an `injector-dev-ws-<name>` context merged into `~/.kube/config` and
made current — so local `kubectl`/`helm`/`k9s` work against it transparently.

### Prerequisites

- A workspace reachable over SSH as `ssh workspace-<name>` (be connected to
  Appgate). Create one with:
  ```shell
  workspaces create <name> --repo dd/datadog-agent
  ```
  `injector-dev` does **not** create the workspace; if it's missing it fails fast
  and prints this command.
- Local **docker** for building images, as usual. The workspace only needs docker
  (`kind` is installed for you).

### Selecting the workspace

The workspace name comes from the `--workspace` flag or a scenario's
`platform.workspace` (flag wins). It is intentionally **not** read from the global
`config.yaml`.

In a scenario (picked up by `apply`):

```yaml
platform:
  type: workspace
  workspace: firstname-lastname   # SSH host = workspace-firstname-lastname
  name: my-cluster                # optional kind cluster name on the VM (default: "kind")
  reset: false
helm:
  # ... same as any other scenario ...
```

Or by flag — **required** for `start`/`stop`/`reset`, which don't read a scenario:

```shell
injector-dev apply -f scenario.yaml --workspace firstname-lastname --build
injector-dev stop  --platform=workspace --workspace firstname-lastname
```

### Usage

```shell
# bring the remote cluster up + deploy (build local, load remote)
injector-dev apply -f scenario.yaml --workspace <name> --build

# local kubectl now targets the remote cluster through the tunnel
kubectl get pods -A

# tear down the remote cluster, tunnel, and kube-context
injector-dev stop --platform=workspace --workspace <name>
```

### Notes

- `start`/`apply` are non-destructive when the cluster already exists: they reuse
  it and just re-establish the tunnel and switch your kube-context (so
  `apply --reset=false` still re-points `kubectl` at the workspace).
- The SSH tunnel is persistent (survives after the command exits) so local
  `kubectl` keeps working. `injector-dev stop` closes it.
- Inspect open tunnels: `ls ~/.injector-dev/*.sock`, and check one with
  `ssh -O check -S ~/.injector-dev/workspace-<name>.sock workspace-<name>`.
- You can also inspect the cluster on the workspace directly: `ssh workspace-<name>`
  then `kubectl` (kind writes a kubeconfig there too).

## The core loop: `apply`

`apply` is the primary command. It (optionally) resets/starts the cluster,
installs the Datadog stack via Helm or the Operator, deploys any apps/manifests,
and waits for health.

```shell
injector-dev apply -f workloads/my-feature/scenario.yaml            # deploy
injector-dev apply -f workloads/my-feature/scenario.yaml --build    # build local source first
```

### `apply` flags

| Flag | Default | Purpose |
| --- | --- | --- |
| `-f, --file` | — | Path to the scenario file (required). |
| `--build` | `false` | Run build steps for any component with `build: {}`. |
| `--reset` | `true` | Reset the cluster before applying. Set `--reset=false` for fast iteration. |
| `--hard` | `false` | Hard reset (rebuilds the VM — colima only). |
| `--wait` | `true` | Wait for the install to become healthy. |
| `--skip-agent-validation` | `false` | Skip the "agent started successfully" check. |
| `-t, --app-image-tag` | — | Global image tag applied to all test apps. |
| `--helm-skip-schema-validation` | `false` | Pass `--skip-schema-validation` to Helm (useful with local chart changes). |
| `--profile` | `config.yaml` | Select a config profile. |
| `--platform` | from config | Override the driver. **If you set it on `start`, you must set it on every `apply`.** |
| `--workspace` | — | Remote workspace name for `--platform=workspace` (see [Remote clusters](#remote-clusters-the-workspace-platform)). Global flag — also valid on `start`/`stop`/`reset`. |
| `--debug` | `false` | Verbose logging. |

## Scenario files

A scenario is `helm:` **or** `operator:`, optionally preceded by a `platform:`
block. Keep each scenario in its own directory alongside its manifests:

```
workloads/
├── hello-world/
│   └── scenario.yaml
├── my-feature/
│   ├── scenario.yaml
│   └── redis.yaml
```

Generate a starter template with `injector-dev new --type helm --output scenario.yaml`
(add `--edit` to open it in `$EDITOR`).

### `platform` block

```yaml
platform:
  type: kind              # kind (recommended) | colima | minikube | nvkind | workspace | none
  name: my-dev-cluster    # unique cluster/profile name — give each scenario its own
  reset: false            # false → reuse the cluster if it exists (fast); true → recreate each apply
```

Precedence for both platform and reset: **CLI flag > scenario `platform:` block > default**.

### Deploying a pre-built version (simplest case)

```yaml
---
platform:
  type: kind              # recommended
  name: hello-world
  reset: false
helm:
  versions:
    agent:
      version: "7.81.0"       # use the latest available agent version
    cluster_agent:
      version: "7.81.0"       # use the latest available cluster-agent version
    injector: "0.60.0"        # use the latest available injector version
  config:
    datadog:
      kubelet:
        tlsVerify: false      # needed locally; the kubelet cert usually isn't trusted
    clusterAgent:
      enabled: true
```

### Building from local source

Add `build: {}` to any component and pass `--build`:

```yaml
---
platform:
  type: kind              # recommended
  name: my-dev-cluster
  reset: false
helm:
  versions:
    agent:
      version: "7.81.0"       # use the latest available version
      build: {}             # build agent from local source at code_root/datadog-agent
    cluster_agent:
      version: "7.81.0"       # use the latest available version
      build: {}
    injector:
      version: "0.60.0"       # use the latest available version
      build: {}             # build auto_inject from code_root/auto_inject
  config:
    datadog:
      kubelet:
        tlsVerify: false
    clusterAgent:
      enabled: true
```

```shell
injector-dev apply -f scenario.yaml --build
```

You can pin a build tag with `build: { tag: "dev.1" }` (defaults to a
git-derived tag otherwise).

### Version / image field reference

Each of `agent`, `cluster_agent`, `injector`, `csi`, (and `operator` in operator
scenarios) accepts either a **string** or a **map**:

```yaml
injector: "0.60.0"          # string → pull this tag from the default repo

agent:                       # map form
  tag: "7.81.0"
  repository: registry.ddbuild.io/ci/datadog-agent/agent   # override the image repo
  pullPolicy: IfNotPresent
  build:                     # presence of `build` → build locally (needs --build)
    tag: "dev.1"
```

**Pin to a CI pipeline / branch artifact** — reproduce a coworker's PR build (or
any pipeline build) without compiling locally:

```yaml
helm:
  versions:
    agent:
      repository: registry.ddbuild.io/ci/datadog-agent/agent
      tag: v<PIPELINE>-<COMMIT>-7-amd64
    cluster_agent:
      repository: registry.ddbuild.io/ci/datadog-agent/cluster-agent
      tag: v<PIPELINE>-<COMMIT>-amd64
```

### Full `helm:` schema

| Field | Description |
| --- | --- |
| `versions` | `agent`, `cluster_agent`, `injector`, `csi` image specs (see above). |
| `config` | YAML passed to Helm as the values file (the `datadog` / `clusterAgent` / `agents` tree). |
| `configFile` | Path to an external Helm values file instead of inline `config`. |
| `localChartPath` | Install from a local chart dir instead of the public repo (see below). |
| `apps` | List of test apps deployed via the base app chart (see Apps). |
| `namespaces` | Explicitly create namespaces with specific labels. |
| `manifests` | Raw Kubernetes YAML files applied after the agent + apps. |
| `charts` | Additional Helm charts to install alongside. |

### Test apps

Apps are deployed through a shared base chart (schema in `apps/base/values.yaml`).
Sample apps live in `apps/`: **c, dotnet, java, js, php, python, ruby**.

```yaml
helm:
  apps:
    - name: python
      namespace: application
      values:
        image:
          repository: registry.ddbuild.io/ci/injector-dev/python
          tag: "2cd78ded"
        service:
          port: "8080"
        podLabels:
          language: python
          tags.datadoghq.com/env: local
        env:
          - name: DD_TRACE_DEBUG
            value: "true"
          - name: DD_APM_INSTRUMENTATION_DEBUG
            value: "true"
```

App fields: `name`, `namespace`, `values` (or `valuesFile`), `build` (build the
app image locally), `injector` (override injector image per-app), `wait`.

> Kubernetes health checks hit each pod's endpoints, so a running sample app
> automatically produces traces once instrumentation is enabled — a quick way to
> confirm injection is working.

### Raw manifests & namespaces

```yaml
helm:
  namespaces:
    - name: cache
      labels:
        team: platform
  manifests:
    - path: "redis-with-password.yaml"   # relative to the scenario file
      namespace: cache                    # auto-created if missing
```

### Local Helm chart

If you're also changing the Datadog Helm chart, point at a local copy.
`injector-dev` then skips the repo add/update and installs from the path:

```yaml
helm:
  localChartPath: ~/dd/helm-charts/charts/datadog
  versions:
    agent: { version: "7.81.0", build: {} }         # use the latest available version
    cluster_agent: { version: "7.81.0", build: {} }
    injector: "0.60.0"
  config:
    datadog:
      kubelet: { tlsVerify: false }
    clusterAgent: { enabled: true }
```

Pair with `--helm-skip-schema-validation` if your local chart adds values the
published schema doesn't know about yet.

### Operator scenarios

Switch the top-level key to `operator:`. `config` becomes the **DatadogAgent CRD
spec** rather than Helm values:

```yaml
---
platform:
  type: kind              # recommended
  name: operator-example
  reset: false
operator:
  versions:
    operator: "1.28.0"        # use the latest available operator version
    agent: "7.81.0"           # use the latest available version
    cluster_agent: "7.81.0"
    injector: "0.60.0"
  config:
    apiVersion: datadoghq.com/v2alpha1
    kind: DatadogAgent
    metadata:
      name: datadog
    spec:
      features:
        apm:
          instrumentation:
            enabled: true
```

The operator itself can be built from local source too: set
`versions.operator.build: {}` and pass `--build`.

## Other commands

| Command | Description |
| --- | --- |
| `injector-dev start [--platform <p>] [--debug]` | Start the k8s platform manually. |
| `injector-dev stop` | Tear everything down (end of day). |
| `injector-dev reset` | Reset the cluster to a clean state. |
| `injector-dev reset --hard` | Full reset including the VM (colima only). |
| `injector-dev new --type helm\|operator --output scenario.yaml [--edit]` | Scaffold a scenario. |
| `injector-dev build --type <t> [...]` | Build a single component without deploying. |
| `injector-dev version` | Print version / commit / build time. |

### Standalone builds

`build --type` accepts: `app`, `agent`, `cluster-agent`, `injector`, `operator`, `csi`.

```shell
injector-dev build --type injector
injector-dev build --type cluster-agent
injector-dev build --type app --context ./apps/python
```

`build` flags: `-t/--type`, `-n/--name`, `-c/--context`, `-f/--dockerfile`,
`-r/--repository`, `-g/--tag`.

## Fast-iteration tips

- Set `platform.reset: false` and a stable `platform.name` per scenario so
  applies reuse the cluster instead of recreating it. Override with `--reset=true`
  only when you need a clean slate.
- Keep `dev_container.persist: true` for ~30s rebuilds after the first build.
- Use `--skip-agent-validation` when the Agent intentionally won't fully start
  (e.g. testing a failure path) so `apply` doesn't error out.
- `--app-image-tag`/`-t` sets one image tag across all apps at once.
- `tlsVerify: false` under `datadog.kubelet` is almost always needed locally.

## Troubleshooting

- **Agent won't report / auth errors** → check `DD_API_KEY`/`DD_APP_KEY` (or the
  active profile's config) and that you're pointed at the right org.
- **Kubelet TLS errors** → set `datadog.kubelet.tlsVerify: false`.
- **Platform flag "sticks"** → if you passed `--platform` to `start`, pass it to
  every `apply` too, or set `platform:` in the scenario/config.
- **Stale/unhealthy cluster** → `injector-dev reset` (or `reset --hard` on colima).
- **Helm schema rejects new values** → `--helm-skip-schema-validation`.
- **Colima/minikube conflicts** → stop any pre-existing instances first.
- Add `--debug` to any command for verbose logs.
