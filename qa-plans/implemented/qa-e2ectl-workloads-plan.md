# e2ectl: deploy workloads alongside the Agent — across environment types

> **Implemented** (commit `a345609a38f`). Specifying test workloads
> in the e2ectl config, deployed after the Agent, with the right mechanism per
> environment type. See the
> [plan status index](../qa-e2ectl-plans-index.md#5-category-c--pending-feature-designs-not-implemented).

**Status:** implemented for `kind` and `local` bases; live-verified on kind
(`app: nginx` → Deployment+Service in `workload-nginx`, agent monitoring
asserted by `TestWorkloadContainerMetrics`, the AD-annotation-triggered
nginx check asserted by `TestWorkloadADCheck` — 9/9 pass).
**Replaces the previous version of this plan**, which was Kubernetes-only.

## As-built deviations from the design

| Design | As built | Why |
|---|---|---|
| `WorkloadSupport` / `WorkloadDeployer` interfaces | Plain `Deploy`/`Validate` functions dispatched by base string | Two bases, two dispatch arms — the interface layer would be ceremony until a third mechanism appears |
| Section schema = `Config` wrapper | Per-entry schema; the section node is a sequence, so each list item decodes through `Schema` individually (`decodeWorkloadList` in config.go) | The wrapper expected a mapping where the YAML is a list — caught live |
| `command:` form | Not implemented | Defer until a host deployer exists (see below) |
| client-go apply | `kubectl` via the snapshot's kubeconfig | The kubeconfig is already materialized at `entry.KubeconfigPath()`; kubectl handles multi-doc and applies atomically |
| (not in design) | Idempotent namespace creation (`kubectlEnsureNamespace`) | Catalog namespaces (`workload-nginx`…) don't exist in a fresh kind cluster |
| (not in design) | AD annotation on the **pod template**, not the Deployment metadata | Deployment-level annotations never reach pods; fixed live |
| (not in design) | v2 annotation format `ad.datadoghq.com/<container>.checks` + `%%host%%` variable, not `.instances` + `%{host}` | The `.instances` suffix is only valid in the v1 trio (with `check_names`); standalone it is silently ignored. `%{host}` is not a template variable. Fixed live (commit `d1dc66fc3e9`); the nginx AD check now runs and `nginx.net.*` metrics reach the fakeintake |

## Not yet implemented (was step 6)

The **EC2 host deployer** is missing — `Deploy` rejects `ec2-host` with
"workloads are not supported on base %q yet". `Validate` also accepts
`app:`/`image:` on `ec2-host` (the catalog has no host definitions yet, so
`app:` is rejected per-entry; `image:` passes validation but fails at
install). Design section 4's host mechanism (SSH `docker run` / package
install) is the template for whoever picks this up.

## 1. The core insight

"Workload" means something different per environment:

| Environment | A workload is... | Deployment mechanism |
|---|---|---|
| kind / EKS | Kubernetes Deployment/Service/ConfigMap | `kubectl apply` via the Kubernetes client |
| EC2 VM | A process or service on the host | SSH: `apt-get install`, `docker run`, or a binary |
| local container agent | A Docker container alongside the agent | `docker run` on the same network (the same mechanism the fakeintake already uses) |

The **declaration** is the same ("I want nginx"); the **mechanism** is the
environment's. The config doesn't know how the workload gets deployed — the
environment does.

```
workloads:                    ← the intent (what to run)
  - app: nginx

environment:                 ← the target (where to run it)
  base: kind | ec2-host | local
```

## 2. The config

A top-level `workloads:` list — environment-agnostic:

```yaml
schema: 1
environment:
  base: kind                # or ec2-host, or local
  fakeintake: true
agent:
  install: helm | script | binary
workloads:
  # Named pre-built apps (the framework's existing test workloads)
  - app: nginx
  - app: redis

  # Inline manifest (YAML — a Kubernetes manifest, a docker-compose service,
  # or a host command, depending on what the environment consumes)
  - manifest: |
      apiVersion: apps/v1
      kind: Deployment
      ...

  # File reference
  - manifest: ./my-stack.yaml

  # A Docker image (runs as a container on any Docker-capable environment)
  - image: ghcr.io/datadog/apps-nginx-server:latest
    name: my-nginx
```

Four forms, layered by what they control:

| Form | What it specifies | Who interprets it |
|---|---|---|
| `app: <name>` | A named workload from the framework catalog | The environment maps it to the right deployment |
| `manifest: <yaml-or-path>` | The raw deployment definition (K8s manifest, compose service, etc.) | The environment applies it in its native format |
| `image: <ref>` | Just a container image | The environment runs it (K8s Deployment, `docker run`, or a process) |
| `command: <shell>` | A command to run on the host | Host environments only (SSH / docker exec) |

The `app:` form is the common case and the recommended default — the same
`app: nginx` works on kind (a Deployment + Service), on EC2 (a package
install or a Docker container), and on the local container agent (a Docker
container on the same network).

### The catalog

The named apps are the framework's existing test workloads, defined once as
**data** (not Pulumi code):

```go
// testing/workloads/catalog.go
type Workload struct {
    Name        string
    Description string
    // For Kubernetes environments: the manifests to apply
    K8sManifests func(namespace string) ([]runtime.Object, error)
    // For Docker-capable environments: the image and exposed ports
    Image string
    Ports []int
    // For host environments: the install command (if different from Docker)
    HostInstall func() string
}

var Catalog = map[string]Workload{
    "nginx": {
        Name:        "nginx",
        Description: "NGINX web server for AD-annotation and HTTP check testing",
        K8sManifests: nginx.Manifests,
        Image:       "ghcr.io/datadog/apps-nginx-server:" + apps.Version,
        Ports:       []int{80},
    },
    "redis": {
        Name:        "redis",
        Description: "Redis for auto-discovery and endpoint check testing",
        K8sManifests: redis.Manifests,
        Image:       "ghcr.io/datadog/redis:" + apps.Version,
        Ports:       []int{6379},
    },
    // ... cpustress, tracegen, dogstatsd
}
```

## 3. The typed schema

One data-only config type, validated by the existing schema engine:

```go
// cmd/internal/envconfig/workloads/config.go
type Workload struct {
    // App is a named pre-built workload from the framework catalog.
    App string `yaml:"app,omitempty" description:"Deploy a named test workload from the catalog."`
    // Manifest is inline YAML or a path to a YAML file.
    Manifest string `yaml:"manifest,omitempty" description:"Inline manifest or a path to a YAML file."`
    // Image is a Docker image reference (container-native environments).
    Image string `yaml:"image,omitempty" description:"Docker image to run as a workload."`
    // Name overrides the derived name (for multi-instance workloads).
    Name string `yaml:"name,omitempty" description:"Override the workload name."`
    // Namespace overrides the target namespace (Kubernetes only).
    Namespace string `yaml:"namespace,omitempty" description:"Kubernetes namespace override."`
}

type Config struct {
    Workloads []Workload `yaml:"workloads,omitempty"`
}
```

Exactly one of `app`, `manifest`, or `image` must be set — a cross-field
rule via the `Validate` hook.

## 4. Who validates, who deploys

The **environment owns both** — the same principle as the agent section.
Just as the selected installer validates `agent.script` / `agent.helm` /
`agent.binary` (a script install rejects `image` before any infrastructure
is created), the selected environment validates the `workloads` declarations
against its capabilities.

### The validation chain (mirrors the agent section exactly)

| Step | What it checks | When | What it prevents |
|---|---|---|---|
| 1. Shape (schema) | Exactly one of `app` / `manifest` / `image` / `command` | Parse time | Typos, unknown fields, wrong types |
| 2. Form compatibility | The selected environment supports this form | Prepare time | K8s manifests on a host; `command` on kind |
| 3. Catalog availability | The named `app:` has a definition for this environment type | Prepare time | An app that only exists for Kubernetes on a host base |
| 4. Runtime | The workload actually deploys and becomes Ready | Install time | Image pull failures, bad manifests, scheduling issues |

Steps 1–3 happen before any infrastructure is created. A user who writes
a K8s manifest and selects `base: ec2-host` gets:

```
e2ectl: workloads[0].manifest: Kubernetes manifests are not supported on
base "ec2-host" (supported forms: app, image, command)
```

The same validation catches an `app:` that has no definition for the
selected environment:

```
e2ectl: workloads[0].app: "nginx" has no definition for base "local"
(available: image)
```

Step 4 is the runtime boundary — image pulls, scheduling and quota are the
same class of check as credentials and network reachability.

### The capability and deployer interfaces

The driver declares what it can deploy; the deployer implements the
mechanism. The CLI checks capabilities at prepare time, then dispatches to
the deployer at install time.

```go
// The driver declares which workload forms it handles. The CLI checks
// declarations against this at prepare time — before provisioning.
type WorkloadSupport interface {
    // SupportedForms returns which config forms this environment handles.
    // Example: kind returns ["app", "image", "manifest-k8s"];
    // ec2-host returns ["app", "image", "command"];
    // local returns ["app", "image"].
    SupportedWorkloadForms() []string
}

// The deployer is an environment-specific capability, like Installers for
// the agent. The CLI dispatches; the environment implements.
type WorkloadDeployer interface {
    // DeployWorkloads deploys the declared workloads into the environment.
    // It is called at the end of install, after the agent is running.
    DeployWorkloads(cfg *config.File, entry envstore.Entry) error
}
```

### kind / EKS (Kubernetes environments)

Apply manifests via the standard Kubernetes client (client-go), the same
client the Helm installer already uses. `app:` entries resolve to their
K8s manifests; `manifest:` entries are applied as-is; `image:` entries
become a minimal Deployment.

```go
// For app: nginx → apply the Deployment + Service
// For manifest: ... → apply the YAML documents
// For image: foo → create a Deployment with just that image
```

### EC2 VM (host environment)

Deploy via SSH — the same mechanism the install script already uses.

| Form | Mechanism |
|---|---|
| `app: nginx` | `apt-get install nginx` or `docker run ghcr.io/datadog/apps-nginx-server` |
| `image: foo` | `docker run foo` on the VM |
| `manifest: ...` | Not supported on host (no Kubernetes) — or interpreted as a docker-compose service in the future |
| `command: ...` | SSH `Execute(command)` |

### local container agent (Docker environment)

The agent is already in a Docker container on a network. Workloads are
**additional Docker containers on the same network** — the exact same
mechanism as the fakeintake (`localinfra.RunFakeintakeOnNetwork`):

```go
// For app: nginx → docker run --network <env>-net ghcr.io/datadog/apps-nginx-server
// For image: foo → docker run --network <env>-net foo
// For manifest: ... → not supported (no Kubernetes) — defer
```

This is the fastest path: the local environment's fakeintake is already a
Docker container on the network; adding nginx/redis is the same call.

## 5. The lifecycle

```
e2ectl start --base kind       → cluster + fakeintake
e2ectl install --env dev       → agent installed + workloads deployed (both)
e2ectl update --env dev        → agent updated (workloads unchanged)
e2ectl stop --env dev          → everything destroyed (cluster removes workloads)
```

Workloads deploy **after the agent is running**, so the agent captures
metrics/logs from workload startup. `update` doesn't redeploy them.

`stop` destroys them with the environment:
- kind: the cluster deletion removes the workloads
- EC2 VM: the Pulumi stack destroy removes everything
- local: the driver's stop removes all containers on the network

## 6. What each environment type supports first

| Capability | kind | EC2 VM | local container |
|---|---|---|---|
| `app:` catalog | ✓ (K8s manifests) | ✓ (Docker image or install command) | ✓ (Docker image) |
| `image:` | ✓ (as Deployment) | ✓ (docker run on VM) | ✓ (docker run on network) |
| `manifest:` (K8s YAML) | ✓ | ✗ (no Kubernetes) | ✗ (no Kubernetes) |
| `manifest:` (host command) | ✗ | ✓ (SSH execute) | ✗ |
| `manifest:` (compose file) | future | future | future |

The `app:` and `image:` forms work everywhere; `manifest:` is
environment-specific. This is honest — the plan doesn't pretend a K8s
manifest can run on a VM.

## 7. Implementation steps

| Step | What | Gate | Status |
|---|---|---|---|
| 1. `workloads` schema | `cmd/internal/envconfig/workloads` — per-entry type, mutual-exclusion rule | Round-trip, rules, valid example | ✅ done |
| 2. `workloads` in the envelope | Top-level section in config.Parse | `init --base kind` accepts the section | ✅ done |
| 3. Workload catalog | `testing/workloads/catalog` — named apps with per-base definitions | Each app produces the right artifacts for each environment type | ✅ done (kind + local forms) |
| 4. Local Docker deployer | `docker run` on the agent's network for `image:`/`app:` | `e2ectl install` with `app: nginx` deploys a container alongside the agent | ✅ done |
| 5. Kubernetes deployer | kubectl apply for `manifest:`/`app:`/`image:` | `e2ectl install` with `app: nginx` deploys a Deployment on kind | ✅ done, live-verified |
| 6. Host deployer | SSH execute / docker run for `image:`/`app:` | `e2ectl install` with `app: nginx` deploys nginx on the VM | ⬜ not started |
| 7. Stop integration | Each driver removes its workload type | `stop` cleans everything on all three bases | ✅ inherent (cluster deletion / container network teardown) |
| 8. Test expansion | `TestContainersOnLocalKind` gains workload assertions | Workload tests pass on local kind | ✅ done (TestWorkloadRunning, TestWorkloadContainerMetrics, TestWorkloadADCheck — 9/9 pass) |

The local Docker deployer is the simplest (step 4) — it's the same
mechanism as the fakeintake. The Kubernetes deployer (step 5) is the next
most valuable — it unlocks the containers test suite on local kind.

## 8. What is deliberately deferred

- **`manifest:` on non-Kubernetes environments** — a compose-file format for
  host/local environments, designed when needed.
- **`wire: agent`** — auto-configure workload-to-agent communication beyond
  the standard endpoints (dogstatsd, APM).
- **Workload lifecycle commands** (`e2ectl workloads add/remove`) — the first
  pass deploys at install only.
- **EKS workloads** — the same Kubernetes deployer applies; EKS is not yet
  an e2ectl base.
- **Workload-specific test configuration** — the workload's own check
  parameters stay in the agent config.
- **Windows workloads** — same pattern, different images/commands.
