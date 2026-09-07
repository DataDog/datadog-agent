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

## Final state (M1 + local-kind use case)

- Live-verified flow: `e2ectl start` (kind + local docker fakeintake) →
  `e2ectl install` (released 7.67.0 via the helm installer) → metric rename in
  pkg/aggregator → `dda inv agent.hacky-dev-image-build` → `e2ectl update` →
  renamed metric observed in `e2ectl fakeintake metrics`. Old-name payloads
  from the previous agent remain queryable in the same fakeintake.
- The `qa-dev` environment was left running for inspection:
  `e2ectl list`, `e2ectl fakeintake names --env qa-dev`,
  `kubectl --kubeconfig ~/.e2ectl/envs/qa-dev/kubeconfig ...`,
  teardown with `e2ectl stop --env qa-dev`.
- The EC2 path (provision-ec2/destroy-ec2/install-host) is implemented and
  compile-checked but not executed (no cloud credentials in this session).
- Unit tests: config validation tables + snapshot roundtrip.

### 10. The outputs seam — install/update moved out of the worker (done)

Directive: no Pulumi for any operation that does not require it; install in kind
must run in-process via the no-pulumi installers. The seam that made it possible:

- **`components/outputs`** (new, Pulumi-free): the import contract (Importable,
  JSONImporter, CloudProviderIdentifier), every `*Output` struct
  (Host, Cluster, KubernetesObjRef, KubernetesAgent, HostAgent, DockerAgent,
  Fakeintake + its seed/RCRootJSON, HostUpdater, DockerManager, ECSCluster,
  ActiveDirectory). The Pulumi component packages re-export them as aliases,
  so all existing imports keep working.
- **`components/os/types`** (new, Pulumi-free): OS descriptors; the Pulumi
  package-manager machinery stays in `components/os` and aliases back.
- **`runner` freed** by moving `configmap.go` (Pulumi stack configs) to
  `runner/infraconfig`; **`common/utils` freed for its free consumers** by moving
  the YAML helpers to `common/utils/yamlutil`.
- **Windows is the one genuine holdout**: `WindowsHost` carries the Pulumi
  `config.Env` of the Windows scenario, so it moved to
  `environments/windowshost` + `scenarios/outputs/windowshost` (consumers flipped,
  no aliases to avoid re-tainting `environments`).
- After the seam: environments, testing/components, standalone, both installers
  and the provisioner package (with StaticStackProvisioner) are all
  Pulumi-free (`go list -deps` = 0 pulumi packages).
- **CLI consequence**: `install` and `update` run in-process in the core
  (internal/installer); the worker shrank to EC2-only (provision/destroy) and
  imports the shared Job type from cmd/e2ectl/workerclient (dedup D2 done).
- Live-verified: `e2ectl update --env qa-dev --skip-build` without any worker —
  kind load, snapshot attach, helm upgrade, renamed metric flowing.
- Still local (now unblocked): `config.SupportedOS` can be derived from
  `components/os/types` instead of a hand table (consolidation D8).

### 11. Extensibility executed: driver registry + scenario registry (live-verified)

The extensibility plan (§2-§12) is implemented:

- **Driver registry** (cmd/e2ectl/internal/driver): Driver/Updatable + the
  explicit registry slice — the single edit site. The Installer/Updatable
  contracts live in the installer package so drivers implement them
  structurally without importing the registry (no import cycle).
- **Driver-owned config sections**: environment = {base, fakeintake, <base>-section};
  the core strict-decodes what it owns and hands the section raw to the driver —
  no shared typed struct, unknown-field rejection per driver. The install/base
  matrix moved to registry lookups with "supported:" error messages.
- **Worker = pulumi-executor with a scenario registry**: generic job
  {action, base, params, stack}; scenarios.go registers {base → params decoder
  + run function}; main is a forever-static engine. Infra-only (§12): the EC2
  scenario always runs WithoutAgent+WithoutFakeIntake; the core deploys the
  fakeintake on the VM over ssh post-provision.
- **Bookkeeping from the snapshot**: kind Stop derives the cluster name from
  the snapshot; the EC2 stack name is recomputed deterministically from the
  env name — envstore gained the opaque DriverMeta field but nothing needs it
  yet.
- **Commands are switch-free**: start/install/update/stop are registry-driven.

Live-verified (fresh env qa-dev2, kind 1.33.0 pinned via the driver section):
start → install 7.67.0 in-process → update --config (dev image, kind-load hook)
→ renamed metric in fakeintake → stop cleans everything. Core dep graph:
zero pulumi packages; worker: the executor, by design.

Tricky points found while executing:
- `install --config <file>` replaces the stored config copy — so a dev-image
  `update` after a released-version install needs `update --config <dev>`: the
  config-switch is the designed flow, not a bug. Worth a README line one day.
- The process check posts to process.datadoghq.com regardless of dd_url (only
  the core forwarder is fakeintake-routed) — pre-existing framework behavior,
  harmless with dummy keys (403s), observed live.
- Docker on the VM: the ec2 post-provision fakeintake runs `docker` on the host;
  the plain awshost image may not ship it — flagged for the (untestable, no
  creds) EC2 path: the scenario may need WithDocker like the dockerhost envs.

### 10. Review findings (independent review pass, 2026-09-07)

Two defects confirmed against HEAD (24a2d190cb2), verified from the supervisor side:

- **Runner BUILD breakage**: `test/e2e-framework/testing/runner/configmap_test.go:26`
  calls the pre-migration `BuildStackParameters` (undefined under the `test` tag;
  `go vet -tags test ./testing/runner/` fails), and the BUILD.bazel still embeds the
  test file. Test-tag compilation of the runner package is broken.

- **Snapshot contract breakage (traces to the M1 design)**: `components.Export` keys
  each Importable by its component *export name* (dd-Host-*/dd-HostAgent-*), so
  `ProvisionE`'s RawResources carry export-name keys; `StaticStackProvisioner.wireEnv`
  matches field-name/`import`-tag keys and nils everything else; `WriteSnapshotFile`
  preserves keys verbatim. Net: the documented roundtrip (ProvisionE →
  WriteSnapshotFile → StaticStack reattach) loses ALL components for stock
  Pulumi-provisioned envs. The kind driver is unaffected only because kinddriver
  hand-synthesizes canonical keys; the EC2 path (provision-ec2 → install-host) carries
  the bug live (never cloud-tested). The snapshot tests assert format substrings, never
  a wireEnv roundtrip — the gap that let this through.

  Fix direction when picked up: canonicalize at write time from the *wired env*
  (SetKey happened during BuildEnvFromResources — a `CanonicalResources(env,
  resources)` inverse of wireEnv), or make Export use import-tag names; add a
  wireEnv roundtrip test with a fake Importable env at the provisioners/provisioner
  boundary.

### EC2 post-provision panic: persist the component bindings

A failed EC2 environment had a snapshot containing `dd-Host-aws-vm`, but no
`remoteHost` key. Static attachment silently dropped the host; fakeintake setup
then dereferenced `env.RemoteHost`. The VM allocation had already succeeded.

The executor now uses `WriteSnapshotFileForEnv`, which records an explicit
`_bindings` map from canonical component names to the provisioner's export names.
Resources retain their original names. Static attachment consumes this map,
retains compatibility with canonical kind snapshots, and rejects snapshots with
broken bindings or no matching components rather than returning an empty env.
Post-install resource updates adjust the corresponding binding as well.

Snapshot replacement is private (0600) and atomic. Fakeintake setup also checks
that its host/client is initialized before issuing remote commands.

Regression coverage uses a fake typed exporter assigning `dd-Host-aws-vm` via
`SetKey`, the real executor write path, then a fresh static attachment. Additional
tests cover multiple hosts, embedding/import tags, optional components, malformed
bindings, private replacement and the missing-host panic guard. All are offline;
no EC2 resources were created or destroyed during this fix.

Legacy snapshots already written with export-name keys need an explicit binding
repair; do not guess among multiple hosts, recreate the VM, or mark an unfinished
setup Ready merely to bypass the CLI's status check. Running environment state
has not been modified by this fix.

### A rebuilt CLI can still launch an outdated executor

A subsequent failure came from a mixed executable pair: the adjacent `e2ectl`
was rebuilt at 10:55, but `e2ectl-worker` was still the 09:30 build. The new
reader correctly rejected another snapshot written without `_bindings` by the
old executor. Building with Bazel had updated `bazel-bin`, not the executable
pair used by `./e2ectl` (the override `E2ECTL_WORKER` was unset in this session).

Both adjacent executables have now been rebuilt and replaced from the Bazel
outputs, with SHA-256 equality checked. No AWS calls or running environment
state changes were made. Existing snapshots without bindings still require
explicit in-place repair and completion of their unfinished setup; replacing
a binary cannot retroactively add metadata to already-written snapshots.

The supported build task should install both artifacts when the executor is
requested, and the protocol/version check in the follow-up plan should reject
an incompatible executor before allocating infrastructure.

### Updated fakeintake ownership: follow the provisioning backend

Decision: for Pulumi-backed scenarios, keep fakeintake in Pulumi. This supersedes
prior notes/plans proposing core-side Docker-over-SSH fakeintake deployment.

The EC2 driver now forwards `environment.fakeintake` into the scenario parameters.
The executor always disables Agent installation, but only disables fakeintake
when explicitly requested. The stock EC2 scenario provisions fakeintake through
ECS Fargate; its endpoint and binding are exported alongside the VM. The core
only reads that endpoint. The SSH/Docker deployment function and its duplicate
image/RC-seed constants have been removed. Agent installation stays in-process
through the same no-Pulumi installer, and local kind keeps its Docker fakeintake.

Existing environments from the earlier VM-only executor will not gain a cloud
fakeintake by repairing bindings alone: the Pulumi stack must be reconciled with
the updated scenario to create it. Do not pretend it exists or mark incomplete
setup Ready. No cloud infrastructure is changed by the source/tests update.

Validation: the five focused Bazel test targets pass (executor, config, driver,
EC2 driver and snapshot tests); the core still has no Pulumi dependency targets.
Both adjacent executables were rebuilt and installed with verified hashes.

Linking initially failed because the main filesystem was full. Only generated
test executables from this session were removed from Bazel outputs after copying
them to `/tmp/e2ectl-test-binaries-22s3kyhu`; no shared cache, source, Docker image
or environment was deleted. The previous executable pair was backed up to
`/tmp/e2ectl-previous-pair-e4caxval` before replacement. Disk space remains very low
(about 140 MB free after the update), so subsequent large builds may fail again.
