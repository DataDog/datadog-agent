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
- `testing/environments/host.go` — Host environment definition
- `testing/environments/environments.go` — `CreateEnv` / `BuildEnvFromResources` (shared import loop)
- `testing/provisioners/aws/host/host.go` — AWS host provisioner
- `components/datadog/agentparams/params.go` — agent configuration options
- `scenarios/aws/ec2/run.go` — EC2 + Agent + FakeIntake Pulumi program
- `common/config/environment.go` — Pulumi config management
- `README.md` — setup guide, troubleshooting, examples

## Unified scenario model

A scenario is defined **once** in Go and driven from tests, the `scenariorun`
CLI, and the `scenario-service` stub without any duplication. See
`scenario/` (reflection: `BuildSchema`/`Decode`/`RegisterFlags`/`CollectFlags`,
the generic `Scenario[Env]`, the type-erased `Runnable` registry, and `Describe()`
with `ProtocolVersion`), `scenario/params/` (reusable `AgentParams`/`FakeintakeParams`
components with `ToOptions()`), and `scenario/scenarios/ec2host/` (reference scenario).

### Two convergent provisioning paths — zero migration

Tests keep using the existing typed provisioner and typed `With…` options
unchanged. The CLI/service path decodes flags into the canonical struct and maps
them onto **those same typed options via the same provisioner**:

```go
// Test path — unchanged:
awshost.Provisioner(awshost.WithRunOptions(
    ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.Ubuntu2204)),
    ec2.WithAgentOptions(agentparams.WithAgentConfig(cfg)),
))

// CLI/service path — Provisioner(p) adapter calls the same awshost.Provisioner:
ec2host.Provisioner(p)  // p is decoded from CLI flags into *EC2HostParams
```

### Canonical params struct and defaulted constructors

Params are a Go struct with `scenario:` tags; `BuildSchema` reflects them:

```
scenario:"name=os,default=ubuntu-22.04,help=Operating system,enum=ubuntu-22.04|debian-12"
scenario:"-"   // Go-only escape hatch, not exposed to CLI (e.g. InstanceOptions []ec2.VMOption)
```

Reusable components (`AgentParams`, `FakeintakeParams`) embed directly into
the canonical struct; `BuildSchema` recurses into them automatically.

The blessed constructor is `scenario.NewParams[T]()` (generic) or each scenario's
own `NewParams()` wrapper (e.g. `ec2host.NewParams()`, `agenthealth.NewParams()`).
Both return a fully-defaulted struct by applying every `default=` tag value. The CLI
applies those same defaults through `Decode` (ApplyDefaults then overlay from flags), so
Go and CLI paths start from identical baselines.

### Single action path

All action execution flows through one function:

```go
scenario.DispatchAction[Env](ctx, s, stack, action, cfg, resolver)
```

The `scenario.EnvResolver[Env]` interface is the only variation:

- **CLI / `scenariorun action`**: uses `scenario.StateResolver[Env]`, which hydrates the
  env from the local state store (no Pulumi call).
- **Tests**: uses a fixed resolver backed by the live suite env, invoked via
  `scenariotest.RunAction(env, s, action, cfg)`.

The registry entry points `scenario.Create` / `scenario.RunAction` / `scenario.Destroy`
are what the `scenariorun` CLI delegates to (keyed by scenario name and stack).

### Actions are curated CLI affordances, not test-step mirrors

Actions expose a small set of operations that are meaningful **from the CLI** (e.g.
`connection-info`, `restart-agent`). They are not reflections of every test mutation.
Test-step mutations stay as ordinary Go against `s.Env()`. For example, in the
`dockerPermissionSuite` the socket `chmod`s (`sudo chmod 660 /var/run/docker.sock`)
remain inline test code — only `connection-info` and `restart-agent` are registered
as actions because those are useful from the command line.

### E2E bridge — reuse a scenario in tests

Tests adopt a scenario with a single `SuiteOption`:

```go
func TestDockerPermissionSuite(t *testing.T) {
    sc := agenthealth.Scenario()
    e2e.Run(t, &dockerPermissionSuite{},
        scenariotest.WithScenario(sc, sc.NewParams()),
    )
}
```

Subtests call `s.Env()` exactly as they would with any other provisioner. To invoke
an action from a test, use `scenariotest.RunAction(s.Env(), sc, "restart-agent", nil)`.

### agent-health: worked custom-env example

`scenario/scenarios/agenthealth/` is the reference for a **custom environment**
(VM + host Agent + Docker app + FakeIntake). The `agenthealth.Env` struct carries
all four components; the scenario's provisioner wires them with `provisioners.NewTypedPulumiProvisioner` (exposed to tests via `scenariotest.WithScenario`).
The `dockerPermissionSuite` in `test/new-e2e/tests/agent-health/` drives this scenario
end-to-end, demonstrating both the E2E bridge and the curated-actions principle.

### CLI

The CLI is the `scenariorun` binary (`test/e2e-framework/cmd/scenariorun`).
Run it directly from the module so flags pass through as normal argv — there is
deliberately no `dda inv` wrapper (invoke cannot cleanly forward cobra-style
flags):

```bash
cd test/e2e-framework
go build -o bin/scenariorun ./cmd/scenariorun   # or: go run ./cmd/scenariorun <args>

./bin/scenariorun list
./bin/scenariorun describe --json                # carries protocolVersion
./bin/scenariorun create ec2-host --os debian-12 --arch arm64 --use-fakeintake --stack my-stack
./bin/scenariorun action ec2-host run-command --command "uname -a" --stack my-stack
./bin/scenariorun action ec2-host restart-agent --stack my-stack
./bin/scenariorun destroy ec2-host --stack my-stack
```

The command tree and flags are generated from the registry by reflection —
never hand-declared. Create-time config is persisted (keyed by stack name, under
`$SCENARIORUN_STATE_DIR` or `~/.scenariorun/stacks`) and replayed on
`action`/`destroy` so they operate on the topology `create` built.

### Local state store

The CLI keeps a local state store of every provisioned stack. Each JSON file
records the scenario name, stack name, create-time config, and the full Pulumi
stack outputs (`resources` field). State files are written to
`$SCENARIORUN_STATE_DIR` when set, otherwise `~/.scenariorun/stacks/`. The
directory is created with 0700; files are written with 0600.

`scenariorun ps` lists all currently-provisioned stacks with their scenario
name and creation time:

```bash
./bin/scenariorun ps
# STACK            SCENARIO   CREATED
# my-stack         ec2-host   2025-07-01T12:00:00Z
```

**Action hydration** uses the cached outputs and import keys captured at create
time — no Pulumi call is ever made. The local state file records both the raw
Pulumi stack outputs (`resources` field) and the field→import-key mapping
(`keys` field, populated by `environments.ImportKeys`). `HydrateFromResources`
replays those keys via `Importable.SetKey` before calling
`BuildEnvFromResources`, so resource lookup uses the correct export name. If
the local state file is absent the action fails with a clear error; there is no
Pulumi fallback. A successful `destroy` deletes the state file (best-effort;
destroy never fails because of a state-cleanup error).

Key implementation files:
- `scenario/state.go` — `ProvisionedStack` (including `Keys` field), `SaveProvisionedStack`, `LoadProvisionedStack`, `ListProvisionedStacks`, `DeleteProvisionedStack`
- `testing/environments/environments.go` — `ImportKeys` (snapshots field→key from a provisioned env)
- `testing/standalone/standalone.go` — `ProvisionWithResources` (returns raw outputs alongside the env), `HydrateFromResources` (replays keys then hydrates from cached resources, no Pulumi)
- `scenario/runnable.go` — wires the state store into `Create`/`RunAction`/`Destroy`

### Service

`cmd/scenario-service` (stub) builds and drives the `scenariorun` binary from
a caller-specified commit via the stable `describe`/`create`/`action`/`destroy`
protocol. This lets a long-running service drive scenarios at any pinned commit
without version coupling.

## Keeping this file accurate

This file is part of the `AGENTS.md` hierarchy (see root `AGENTS.md` §
"Keeping AI context accurate"). Update it when environments, provisioners,
agentparams, or key APIs change. AI agents should fix inaccuracies they
encounter during tasks.
