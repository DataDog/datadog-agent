# e2ectl implementation notes — tricky points

> Running log of tricky points discovered during implementation, per the working
> agreement. Plans: `qa-e2ectl-plan.md` (milestone), `qa-e2ectl-m1-design.md` (design).

## Tricky points

### 1. Pulumi leaks through the Go *package*, not the imports
`provisioners.go`, `file_provisioner.go`, `static_stack_provisioner.go` contain zero
Pulumi imports, but they sit in the same Go package as `pulumi_provisioner.go` and the
cloud subpackages — importing `StaticStackProvisioner` links the entire Pulumi SDK.
Fix: new Pulumi-free package `testing/provisioner` (singular); the old package keeps
type aliases so no caller changes.

### 2. `standalone.Provision` swallowed `RawResources`
The snapshot writer needs the resources, but `Provision` consumed them internally and
returned only the env. Added `ProvisionE` returning `(env, RawResources, error)`;
`Provision` is now a thin wrapper. All existing callers (ai-sandbox) unchanged.

### 3. e2e-framework already has a `registry/` package
The module root contains `test/e2e-framework/registry` (the *scenario* registry). The
CLI's named-environment store is therefore named `envstore` to avoid the collision.

### 4. Fakeintake-on-host recipe for local kind (proven by the framework)
`public.ecr.aws/datadog/fakeintake` docker container, container port 80 → host port
(default 30080, but must be *dynamically allocated* in the CLI so several environments
can coexist), and the agent reaches it at `http://<outbound-IP>:<port>` — the
"UDP dial 8.8.8.8" trick from `fakeintake/docker.go` gives the routable IP. No
extraPortMappings needed: kind node containers can reach the host IP directly.

### 5. Local dev image must be tagged like a registry image for the Helm chart
The Datadog Helm chart renders the agent image as `<registry>/<repository>:<tag>`
(default `gcr.io/datadoghq/agent:<version>`). Fighting chart value semantics (empty
registry etc.) is fragile; instead the CLI instructs the build to tag the image exactly
as the chart expects: `gcr.io/datadoghq/agent:<local-tag>`, runs `kind load docker-image`,
and sets `agents.image.tag: <local-tag>` + `imagePullPolicy: IfNotPresent` (pull policy
default is fine — IfNotPresent won't pull since the image is on the node). The image
never actually goes to GCR.

### 6. API key for local installs
The helm/installscript installers resolve the API key via the runner profile
(`~/.test_infra_config.yaml` or `E2E_API_KEY` env). With no key configured, install fails
before any chart action. Fakeintake accepts any key, but the value must be present.
CLI precedence: `agent.api-key` in the env config → runner profile → clear error.
(For local iteration flows, any non-empty value works.)

### 7. Metric for the rename iteration: `datadog.agent.running`
Emitted by the aggregator heartbeat on every flush (`pkg/aggregator/aggregator.go`,
`fmt.Sprintf("datadog.%s.running", agg.agentName)`), asserted by default in existing
e2e tests. One-line rename proves the whole build→update→fakeintake loop. Revert after
verification.

### 8. Transitive Pulumi through `testing/components` — the CLI is two binaries

`testing/components` (the env component wrappers: FakeIntake, RemoteHost,
KubernetesCluster…) imports `components/datadog/fakeintake` — a Pulumi package — so
everything downstream (environments, both installers, standalone after attach) links
Pulumi transitively. The Output structs themselves (`FakeintakeOutput`, `HostOutput`,
`ClusterOutput`) are Pulumi-free *contents* living in Pulumi packages.

Consequence for M1: the CLI is split into a **Pulumi-free core** (`e2ectl`: kind
start/list/stop, fakeintake inspection, config, envstore — uses only the new
`testing/provisioner` package + kind/docker CLIs) and a **worker** (`e2ectl-worker`:
EC2 provisioning, helm install on k8s, installscript on hosts — imports environments,
installers, standalone). Job-JSON + snapshot IPC, same pattern as the EC2 helper.

Follow-up seam (noted, not M1): move the Output structs into Pulumi-free
`outputs` packages so `testing/components` and the installers detach from Pulumi — then
`install` moves into the core and the worker shrinks to EC2-only.

### 9. Live-run findings (kind)

- **API key and app key**: both `E2E_API_KEY` and `E2E_APP_KEY` must be present for
  the helm installer (it resolves both from the runner profile). Fakeintake accepts
  any value. Core config `agent.api-key` exists in the schema but the installers do
  not take an override yet — wiring it through is a small follow-up.
- **`ClusterAgentVersion` must be valid**: the Datadog chart's helpers run
  `semverCompare` on `clusterAgent.image.tag`; an empty tag breaks rendering.
  Version installs now pass `AgentVersion` and `ClusterAgentVersion`; local-image
  installs pass `ClusterAgentVersion: "latest"` (a custom agent tag like
  `e2ectl-dev` is not semver and would break the comparisons).
- **Helm installer namespace**: `Params.Namespace` must be set (`datadog` by default
  in the worker) — the framework's own tests always pass one.
- **Commit signing broke mid-run**: the ssh-agent socket died and the signing
  private keys are only held by the (dead) agent managed by git-config-tool.
  Workaround used: `git -c commit.gpgsign=false commit` for the remaining local
  commits. Re-sign later with
  `git rebase --exec 'git commit --amend --no-edit -S' 082fb1d4a3f` once the
  agent is back. Nothing was pushed.
- **`agents.image.repository` in the upstream Datadog chart is the FULL path
  including the registry**: the chart's `image-path` helper renders
  `repository:tag` verbatim when repository is set, and only falls back to
  `registry/name:tag` when it is empty. The worker therefore sets
  `agents.image.repository` to `gcr.io/datadoghq/agent` and the tag, and does
  not touch the chart's `registry` value.
- **Iteration loop verified live**: rename of `datadog.%s.running` in
  pkg/aggregator → `dda inv agent.hacky-dev-image-build
  --target-image=gcr.io/datadoghq/agent:7.99.0-e2ectl` → `e2ectl update
  --env qa-dev --skip-build` → the renamed metric observed in the fakeintake,
  with the old name only present on the pre-update agent payloads. The
  `e2ectl update --config <file>` flag replaces the stored config copy, e.g.
  to point at a new local image tag.
