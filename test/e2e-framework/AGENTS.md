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
│   ├── os/               # OS descriptors (Ubuntu2204E2E, WindowsServer2025, etc.)
│   ├── kubernetes/       # K8s components (KinD, OpenShift, Helm addons)
│   ├── docker/           # Docker compose components
│   └── remote/           # Remote host SSH management
├── resources/
│   └── aws/              # Low-level Pulumi resources (EC2, ECS, EKS, IAM)
│                         # + platforms.json: descriptor -> AMI ID table
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
| `environments.Kubernetes` | K8s cluster + Agent + FakeIntake | `kindvm.Provisioner()` (kind; also `eks`, `kubeadm`) | K8s checks, DaemonSet, Cluster Agent |
| `environments.ECS` | ECS cluster + Agent + FakeIntake | `ecs.Provisioner()` | ECS-specific tests |
| custom environment | user-defined struct | `e2e.WithPulumiProvisioner()` | Agent on host + workloads on docker, multi-VM, extra services |

### Provisioners

Provisioners create the environment's infrastructure. Built-in provisioners
live in `testing/provisioners/` organized by cloud provider (aws, azure, gcp, local).
The Kubernetes provisioners live one level deeper, in `aws/kubernetes/{kindvm,eks,kubeadm}`;
tests commonly alias the kind one as `awskubernetes`, and the ECS one as `awsecs`, which are
import aliases rather than package names. Azure, GCP and local provisioners take their options
directly (`azurehost.WithAgentOptions(...)`) rather than nesting them inside `WithRunOptions`
as the AWS example below does.

```go
// Host on AWS EC2
awshost.Provisioner(
    awshost.WithRunOptions(
        ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.Ubuntu2204E2E)),
        ec2.WithAgentOptions(
            agentparams.WithAgentConfig(config),
            agentparams.WithIntegration("check.d", checkConfig),
        ),
    ),
)
```

Prefer the `-e2e` descriptors (`Ubuntu2204E2E`, not `Ubuntu2204`): they resolve to
Packer-built AMIs with Docker, the AWS CLI, `jq`, `ansible` and friends prebaked,
so a test never installs them at runtime. They are already the Linux defaults
(`UbuntuDefault = Ubuntu2204E2E`), so passing no OS at all is also correct.
Descriptors resolve through `resources/aws/platforms.json` via `aws.GetAMI`. See
`docs/public/how-to/test/e2e/custom-amis.md` and
`docs/public/how-to/test/e2e/dependencies.md`.

### Kubernetes resource ownership

Pulumi resource names and component parents only make Pulumi URNs unique. Kubernetes
resource identity is still the combination of kind, namespace, and metadata name. When
components can be installed together, give independently owned resources distinct
Kubernetes names or make one component the explicit owner; do not create the same
Kubernetes object under multiple Pulumi parents.

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

## Agent installers outside Pulumi

`testing/installers` exposes agent installation independently of a Pulumi program.
Packages are organized first by environment type, then by installation method:

- `testing/installers/kubernetes/helm` installs the Helm chart in an
  `environments.Kubernetes` through `helm.Install(ctx, env, params)`.
- `testing/installers/host/installscript` runs the official install script in an
  `environments.Host` through `installscript.Install(ctx, env, params)`. It configures
  the environment's FakeIntake automatically and accepts additional Agent YAML and
  integration configs through `installscript.Params`.

Installers resolve API and application keys through the active runner profile's
secret parameter store. They take initialized environments rather than state files or other
provisioner-specific representations and update `env.Agent`. The same installer
therefore works with Pulumi, `StaticStackProvisioner`, or another provisioner.
State serialization and persistence belong to the caller that owns that state.

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

## macOS hosts

`awshost.Provisioner` supports macOS (`ec2.WithOS(e2eos.MacOSDefault)`), but two
constraints shape how a macOS suite must be wired into CI:

- **Dedicated hosts.** `scenarios/aws/ec2/vm.go` allocates a `mac1.metal`
  (amd64) or `mac2.metal` (arm64) dedicated host, which AWS bills with a 24-hour
  minimum. Keep macOS jobs manual.
- **The agent comes from a DMG in the macOS testing bucket.** `host_macos.go`
  installs via the install script with `DD_REPO_URL` pointing at
  `pipeline-<id>-<arch>`, which only exists once `deploy_dmg_testing-a7_<arch>`
  has run. That job has its own rules, so a macOS e2e job must gate on
  `.on_deploy` and mark the `needs` optional. Note the arch segment there is
  `x64`, not the descriptor's `x86_64` — `macosPipelineArch` does the mapping.

Existing macOS suites: `tests/agent-platform/tests/macos_install_test.go`
(installs by hand, `ec2.WithoutAgent()`) and
`tests/agent-data-plane/preflight-mode` (stock provisioner with agentparams).

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
- `components/os/{linux,windows}_descriptors.go` — OS descriptors and flavor defaults
- `resources/aws/platforms.json` + `platforms.go` — descriptor -> AMI ID table and `GetAMI`
- `README.md` — setup guide, troubleshooting, examples

## Keeping this file accurate

This file is part of the `AGENTS.md` hierarchy (see root `AGENTS.md` §
"Keeping AI context accurate"). Update it when environments, provisioners,
agentparams, or key APIs change. AI agents should fix inaccuracies they
encounter during tasks.
