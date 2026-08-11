# E2E Framework

## Overview

The E2E framework provides infrastructure-as-code test harnesses using Pulumi.
It provisions real cloud infrastructure (AWS, Azure, GCP), deploys the Datadog
Agent, and exposes typed environments for tests to interact with.

Tests live in `test/new-e2e/tests/` and import this framework.

## Structure

```
test/e2e-framework/
├── testing/
│   ├── e2e/              # Test harness: BaseSuite, Run(), SuiteOption
│   ├── environments/     # Environment types: Host, DockerHost, Kubernetes, ECS
│   ├── provisioners/     # Provisioner interfaces + cloud-specific implementations
│   │   ├── aws/          # host, docker, ecs, kubernetes (eks, kindvm, kubeadm)
│   │   ├── azure/        # host (linux, windows), kubernetes (aks)
│   │   ├── gcp/          # host (linux), kubernetes (gke, openshiftvm)
│   │   └── local/        # host (podman), kubernetes (kind)
│   └── components/       # Test-side wrappers: RemoteHost, Agent, FakeIntake
├── scenarios/
│   └── aws/              # Pulumi programs: ec2, ec2docker, ecs, eks, kindvm, kubeadm
├── components/
│   ├── datadog/          # Pulumi components: agent, agentparams, fakeintake
│   │   ├── agentparams/  # Agent configuration options (WithAgentConfig, etc.)
│   │   └── fakeintake/   # Fakeintake deployment component
│   ├── os/               # OS descriptors (Ubuntu, Windows, etc.)
│   ├── kubernetes/       # K8s components (KinD, OpenShift, Helm addons)
│   ├── docker/           # Docker compose components
│   └── remote/           # Remote host SSH management
├── resources/
│   └── aws/              # Low-level Pulumi resources (EC2, ECS, EKS, IAM)
├── common/
│   └── config/           # Configuration (AWS account, key pairs, agent params)
└── README.md             # Full setup and troubleshooting guide
```

## Key concepts

### Environments

An environment defines what infrastructure a test needs:

| Type | Components | Provisioner | Use when |
|------|-----------|-------------|----------|
| `environments.Host` | VM + Agent + FakeIntake | `awshost.Provisioner()` | System checks, agent commands, file-based config |
| `environments.DockerHost` | VM + Docker + FakeIntake | `awsdocker.Provisioner()` | Container checks, Docker integrations |
| `environments.Kubernetes` | K8s cluster + Agent + FakeIntake | `awskubernetes.Provisioner()` | K8s checks, DaemonSet, Cluster Agent |
| `environments.ECS` | ECS cluster + Agent + FakeIntake | `awsecs.Provisioner()` | ECS-specific tests |
| custom environment | user-defined struct | `e2e.WithPulumiProvisioner()` | Agent on host + workloads on docker, multi-VM, extra services |

### Provisioners

Provisioners create the environment's infrastructure. Built-in provisioners
live in `testing/provisioners/` organized by cloud provider (aws, azure, gcp, local).

```go
// Host on AWS EC2
awshost.Provisioner(
    awshost.WithRunOptions(
        ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.Ubuntu2204)),
        ec2.WithAgentOptions(
            agentparams.WithAgentConfig(config),
            agentparams.WithIntegration("check.d", checkConfig),
        ),
    ),
)
```

### BaseSuite

All E2E tests embed `e2e.BaseSuite[Env]` and use `e2e.Run()`:

```go
type mySuite struct {
    e2e.BaseSuite[environments.Host]
}

func TestMySuite(t *testing.T) {
    t.Parallel()
    e2e.Run(t, &mySuite{}, e2e.WithProvisioner(awshost.Provisioner()))
}
```

Key helpers on BaseSuite:
- `s.Env()` — access the provisioned environment
- `s.UpdateEnv(provisioner)` — change agent config mid-suite
- `s.EventuallyWithT(fn, timeout, interval)` — retry assertions until they pass;
  use `require` (not `assert`) inside the callback so failures short-circuit the
  current retry iteration instead of accumulating silently

## Agent configuration

Use `agentparams` to configure the agent on provisioned infrastructure:

- `WithAgentConfig(yaml)` — override `datadog.yaml`
- `WithIntegration(name, yaml)` — add check config under `conf.d/`
- `WithLogs()` — enable log collection
- `WithSystemProbeConfig(yaml)` — system-probe config
- `WithFile(path, content, useSudo)` — place arbitrary files on the host

For `environments.DockerHost`, use `dockeragentparams.WithAgentServiceEnvVariable`
or `AgentServiceEnvironment` for environment variables that must be visible
inside the Agent container. `dockeragentparams.WithEnvironmentVariables` only
sets the environment for the `docker-compose` command and compose-file variable
interpolation.

## Driving the framework outside of `go test`

The client and component layers no longer depend on `*testing.T` (PR #51954), so the
framework can be driven from a standalone binary. Use the `testing/standalone` package:

```go
ctx := standalone.NewContext(localOutputDir) // implements common.Context (T() returns nil)
provisioner := awshost.Provisioner(awshost.WithRunOptions(...))
env, err := standalone.Provision[environments.Host](ctx, "my-stack", provisioner)
defer standalone.Destroy(ctx, "my-stack", provisioner)
// env.RemoteHost.Execute(...), env.RemoteHost.GetFolder(remote, local), etc.
```

`standalone.Provision` mirrors `BaseSuite.reconcileEnv` (CreateEnv → ProvisionEnv →
`environments.BuildEnvFromResources` → `Init`) without any test dependency.
`environments.BuildEnvFromResources` is the shared import loop, used by both `BaseSuite`
and the standalone driver — keep them in sync.

Reference consumer: `cmd/ai-sandbox/main.go` (provisions a host, runs an AI agent on it,
retrieves a directory), wrapped by the `dda inv ai-sandbox.run` invoke task.

## No-Pulumi local provisioning (`cmd/e2ectl`)

For a local `kind` cluster, Pulumi is pure DAG-engine overhead around shelled-out
`kind`/`helm` commands (see `testing/provisioners/local/kubernetes/kind.go`).
`cmd/e2ectl` is a standalone CLI (no Pulumi, no testify) that owns the whole
local test lifecycle — provision infra, install the agent, run the `go test` —
described by a single YAML test definition. What's actually been created is
tracked in a separate, auto-generated JSON state file, centralized under
`test/e2e-framework/.e2ectl-state/<name>.state.json` (gitignored) — `name` is
the YAML's `name:` field, already required to be unique since it's reused for
the kind cluster name, the fakeintake container name, and the Helm release
namespace/labels, so it doubles as a safe state-file key. Centralizing state
this way (rather than colocating it with each YAML) is what lets `e2ectl`'s
dashboard (see below) discover every environment on the machine regardless of
which directory it's invoked from.

**Each test that uses this flow keeps its own YAML test definition colocated
with its `_test.go` file** (same basename, `.yaml` extension) rather than
sharing one config — e.g. `test/new-e2e/examples/kind_nopulumi_test.yaml` next
to `kind_nopulumi_test.go`:

```yaml
# test/new-e2e/examples/kind_nopulumi_test.yaml
name: kind-nopulumi
provisioner:
  type: kind                    # registry key, see cmd/e2ectl/provisioners.go
  options:
    kubeVersion: "1.31"
    withoutFakeIntake: false
agent:
  installer: helm-k8s            # optional; auto-detected if omitted
  agentVersion: latest
  clusterAgentVersion: latest
  namespace: datadog
test:
  package: ./examples/...        # --targets for `dda inv new-e2e-tests.run`
  run: TestKindNoPulumi          # -run pattern
```

```bash
cd test/e2e-framework
go run ./cmd/e2ectl                                              # interactive dashboard: lists every known environment
CFG=../new-e2e/examples/kind_nopulumi_test.yaml
go run ./cmd/e2ectl run --config=$CFG                            # interactive: per-env menu for just this one
go run ./cmd/e2ectl run --config=$CFG --stage=provision          # non-interactive: infra only
go run ./cmd/e2ectl run --config=$CFG --stage=test --yes         # non-interactive: provision+install+test
go run ./cmd/e2ectl destroy --config=$CFG                        # tear down
```

**Dashboard (`e2ectl` with no arguments, `dashboard.go`):** globs
`.e2ectl-state/*.state.json`, follows each one's recorded `_source` metadata
entry back to the YAML that produced it, and lists one line per environment
with a live status summary (`provisioned, agent 7.81 / cluster-agent latest
(up to date)`, `... — drifted`, `not provisioned`), plus `o) open a
config...` for a YAML with no state yet and `q) quit`. Picking an entry (or
`o`) enters that environment's loop.

**Per-env loop (`runEnvLoop`, `wizard.go`):** replaces what used to be a
one-shot "how far should this run go?" prompt. Reached either directly
(`e2ectl run --config=...` without `--stage`/`--yes`) or via the dashboard,
it reprints a status block (infra provisioned? agent status/drift? which test
would run?) and a fixed menu — `1) provision infra`, `2) install/update
agent`, `3) run test`, `4) destroy environment`, `b) back to dashboard` (only
when entered via the dashboard), `q) quit` — after every action, including a
failed one: an action's error is printed and the loop continues rather than
exiting the process, so a transient failure (network blip, port conflict)
doesn't cost you the whole session. `4) destroy` requires typing the
environment's `name` back to confirm — the one action here that's hard to
undo. If stdin isn't a terminal and neither `--stage` nor `--yes` was given
(covers both the dashboard and `run`), `e2ectl` errors out instead of
hanging, so a misconfigured CI job fails fast rather than stalling.

The install stage isn't just "skip if already installed": `installers.Status`
(`testing/installers/installers.go`) compares the state file's recorded
`agent` entry (versions, namespace) against what the YAML currently asks for,
so editing `agentVersion`/`clusterAgentVersion`/`namespace` and re-running
`e2ectl run` (or picking "install/update agent" again from the loop)
re-installs (a real Helm upgrade, via `installOrUpgradeHelmRelease`'s
release-history check) instead of silently skipping — no need to `e2ectl
destroy` first just to pick up a version bump. This comparison is an
`Installer`-interface method (`status`), not logic living in `cmd/e2ectl`
itself, precisely so a future non-Kubernetes installer can describe its own
notion of "up to date" without `cmd/e2ectl` needing to know its output shape;
`stagesCompleted`'s "is infra provisioned" check is similarly generic — any
state-file entry other than `agent` and `_`-prefixed metadata counts, so it
isn't hardcoded to `kind`'s `kubernetesCluster` key either.

The provisioning stage dispatches through a `provisionerRegistry` keyed by the
YAML `provisioner.type` field (`cmd/e2ectl/provisioners.go`); the install stage
is a thin wrapper around `testing/installers.UpdateAgent`, which dispatches
through its own installer registry, chosen explicitly via `agent.installer` or
auto-detected from the shape of the state file. Only `kind` (via
`testing/provisioners/local/kubernetes/kindinfra`) and `helm-k8s` (Helm Go SDK,
installing the public `datadog` chart) are implemented — adding another
environment type means registering a new provisioner/installer (and, for the
installer side, implementing `status`), not changing `main.go`, which
intentionally has zero environment-specific imports. The test stage shells out
to `dda inv new-e2e-tests.run --targets=<package> [--run=<pattern>]` (this
repo's mandated test runner) with `E2E_ENV_FILE` set to the state file's
absolute path.

`testing/installers` exists as its own importable package (rather than living
in `cmd/e2ectl`, `package main`) so the install step can be called in-process,
not just from the CLI: a running `go test` can call
`installers.UpdateAgent(ctx, envPath, installerName, params)` directly to
change agent config mid-suite, the same operation `e2ectl run`'s install stage
performs, with no subprocess/`exec` involved. `installers.Status(installerName,
envEntries, desired)` is exported the same way, for any caller that wants a
read-only drift check without performing an install — it's what both the
dashboard and the per-env loop use to render agent status.
`installers.ResolveAPIKeys()` (env vars, falling back to
`~/.test_infra_config.yaml`) is exported for the same in-process-callable
reason. See `test/new-e2e/examples/kind_nopulumi_test.go`'s `installAgent`
suite helper for the pattern.

The resulting state file is consumed by `provisioners.NewSingleFileProvisioner[Env]`
(`testing/provisioners/file_provisioner.go`), instantiated per-environment-type
(e.g. `NewSingleFileProvisioner[environments.Kubernetes](...)`). Its
`ProvisionEnv` reads that one JSON file's top-level keys as `RawResources`,
same shape as `FileProvisioner` (directory of JSON files) or any Pulumi
provisioner. It also hashes the file's content into an unexported
`fingerprint` field at construction time, so that calling `BaseSuite.UpdateEnv`
again with the same `{id, path}` after `installers.UpdateAgent` has rewritten
the file is detected as a real change — `UpdateEnv`/`reconcileEnv`
(`testing/e2e/suite.go`) decides whether to re-provision purely via
`reflect.DeepEqual` on the provisioner struct, and `{id, path}` alone would
compare equal across calls despite the file's content differing. See
`test/new-e2e/examples/kind_nopulumi_test.go` for a full `go test` reading
`E2E_ENV_FILE`, asserting `datadog-cluster-agent status`, then changing the
cluster agent's version mid-suite via `installAgent` + `UpdateEnv` and
asserting on the new version (`TestClusterAgentVersionUpdate`).

**Why it's `TypedProvisioner[Env]`, not `UntypedProvisioner`:**
`environments.BuildEnvFromResources` looks up each importable env field by
`Importable.Key()`, which for a freshly-allocated component is empty — the
only code path that ever sets it (`components.Export` → `SetKey`) runs
inside a live `pulumi.Context`, and only for `TypedProvisioner[Env]`s, since
those alone receive the live `*Env` to mutate during `ProvisionEnv`. Fields
without `import:"..."` tags (true of every stock `environments.*` struct)
have no other way to get a key. `SingleFileProvisioner[Env]` works around
this by receiving `*Env` itself and calling `SetKey` via reflection
(`assignImportKeys`), matching each importable field to a same-named (or
tag-named) top-level entry in the JSON file before returning
`RawResources`. If you write a new `UntypedProvisioner` meant to feed a
stock environment struct, it will fail with `"... has no import key set and
no annotation"` — make it a `TypedProvisioner[Env]` instead.

This is a POC: no Helm-values parity with the Pulumi path (no
kube-state-metrics/SBOM/autoscaling/APM-instrumentation/OTel/Windows/FIPS/JMX),
`SingleFileProvisioner.Destroy` is a no-op (only `e2ectl destroy` tears down —
`go test` never provisions or destroys anything here), and `e2ectl destroy`
trusts the caller to pass the same `--config`/`--state` used at `run` time.

## Beyond out of the box environments

The stock environments are highly customizable via provisioner options (OS,
agent config, with/without fakeintake, etc.) — explore the `With*` options on
each provisioner before creating a custom environment.

When that's not enough, common advanced patterns:

- **Custom environment structs** — define your own struct with extra components
  (e.g., a second `RemoteHost`, multiple fakeintakes, an HTTPBin service).
  Use `e2e.WithPulumiProvisioner()` to wire it up with inline Pulumi code.
  Start from the examples in `test/new-e2e/examples/customenv_*` and see
  `test/new-e2e/tests/npm/` and `test/new-e2e/tests/ha-agent/` for real usage.
- **Custom provisioners** — environments also support custom provisioners beyond
  the stock ones. Implement the `provisioners.Provisioner` interface to
  target different infrastructure.
- **`e2e.WithUntypedPulumiProvisioner()`** — escape hatch for fully custom Pulumi
  programs when no typed environment fits.
- **`s.UpdateEnv(provisioner)`** — re-provision the agent mid-suite (e.g., change
  config, toggle features) without destroying the underlying infra. Widely used
  but error-prone; may be removed in the future.

### Useful suite options

- **`e2e.WithDevMode()`** — keep infrastructure alive after test for faster iteration.
- **`e2e.WithStackName(name)`** — custom Pulumi stack naming for parameterized tests.

### Example tests by pattern

| Pattern | Look at |
|---------|---------|
| Stock host test | `test/new-e2e/tests/agent-runtimes/` |
| Custom environment (extra hosts/services) | `test/new-e2e/tests/npm/`, `test/new-e2e/tests/ha-agent/` |
| K8s + Helm | `test/new-e2e/tests/ssi/` |
| Multi-fakeintake | `test/new-e2e/tests/agent-runtimes/forwarder/` |
| GPU / specialized hardware | `test/new-e2e/tests/gpu/` |
| Windows | `test/new-e2e/tests/windows/` |
| Docker Compose | `test/new-e2e/tests/agent-health/` |
| ECS / Fargate | `test/new-e2e/tests/cws/` |

## Validating E2E tests

E2E tests provision real cloud infrastructure (~10 min per run). **Always run
the test locally before pushing** — `go vet` catches compilation errors but not
runtime failures:

```bash
dda inv new-e2e-tests.run --targets=./tests/<area>/...
```

Use `e2e.WithDevMode()` to keep infrastructure alive after a failure so you can
SSH in and inspect the agent directly.

## Fakeintake image version

Every fakeintake default (`scenarios/{aws,azure,gcp}/fakeintake/params.go`,
`components/datadog/fakeintake/docker.go`) resolves through
`components/datadog/fakeintake.ImageURL(...)`: it uses the
`FakeintakeImageOverride` runner parameter (`E2E_FAKEINTAKE_IMAGE_OVERRIDE`) when
set — read through the runner parameter store like any other `E2E_*` value, not
`os.Getenv` — otherwise the pinned tag from `test/fakeintake/version.Tag`.
`WithImageURL(...)` on any fakeintake provisioner still wins over both.

CI wiring (`.gitlab-ci.yml`): the `.on_e2e_main_release_or_rc` rule — inherited
by every e2e job through its team rule (`.on_<team>_or_e2e_changes`) — sets
`E2E_FAKEINTAKE_IMAGE_OVERRIDE` to the PR-built `v<sha>` image on a fakeintake
*server* change (`.fakeintake_server_paths`). So such a PR runs the **whole**
e2e suite against the PR's image (including mixed PRs), and no e2e job can miss
the override. `.needs_new_e2e_template` gains optional needs on `publish_fakeintake`
(PR `v<sha>`) and `publish_fakeintake_pinned` (main pinned tag) so e2e waits for
the image to exist. Plain `.on_fakeintake_changes` is for non-consuming
build/publish/version-check jobs only. See `test/fakeintake/AGENTS.md`
§ "Image version pinning" for the full workflow (bumping VERSION, the
strictly-increasing CI check, publish jobs).

## Key files

- `testing/e2e/suite.go` — `BaseSuite` and `Run()` (test entry point)
- `testing/e2e/suite_params.go` — `SuiteOption` (WithProvisioner, WithDevMode, etc.)
- `testing/standalone/standalone.go` — non-test driver (`Provision`/`Destroy`/`Context`)
- `cmd/ai-sandbox/main.go` — standalone consumer (provision + run AI agent + retrieve dir)
- `cmd/e2ectl/` — no-Pulumi CLI: no-argument dashboard (`dashboard.go`) across every known environment, `run` (per-env interactive loop, or `--stage`/`--yes`-driven) / `destroy`; the install stage is a thin wrapper over `testing/installers`
- `testing/installers/installers.go` — `UpdateAgent`/`Status`/`ResolveAPIKeys`, the in-process install/status API shared by `cmd/e2ectl` and tests that change agent config mid-suite
- `testing/provisioners/local/kubernetes/kindinfra/provisioner.go` — no-Pulumi kind cluster provisioner
- `testing/provisioners/file_provisioner.go` — `FileProvisioner` (`UntypedProvisioner`, unused) / `SingleFileProvisioner[Env]` (`TypedProvisioner[Env]`, sets import keys via reflection, fingerprints file content so `UpdateEnv` detects out-of-band changes)
- `testing/environments/host.go` — Host environment definition
- `testing/environments/environments.go` — `CreateEnv` / `BuildEnvFromResources` (shared import loop)
- `testing/provisioners/aws/host/host.go` — AWS host provisioner
- `components/datadog/agentparams/params.go` — agent configuration options
- `scenarios/aws/ec2/run.go` — EC2 + Agent + FakeIntake Pulumi program
- `common/config/environment.go` — Pulumi config management
- `README.md` — setup guide, troubleshooting, examples

## Keeping this file accurate

This file is part of the `AGENTS.md` hierarchy (see root `AGENTS.md` §
"Keeping AI context accurate"). Update it when environments, provisioners,
agentparams, or key APIs change. AI agents should fix inaccuracies they
encounter during tasks.
