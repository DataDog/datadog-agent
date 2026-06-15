# E2E Framework

## Overview

The E2E framework provisions real cloud infrastructure (AWS, Azure, GCP), installs
the Datadog Agent on it, and exposes typed environments for tests to interact with.
The framework is split into three independent layers:

1. **Pulumi provisioning** — creates VMs, clusters, FakeIntake. No agent install.
2. **Installer layer** — installs the agent via SSH/Helm/AWS SDK, no Pulumi required.
3. **Test assertions** — standard `testify.Suite` tests against the running agent.

This split enables a CI two-job pattern: provision once, then run many install+test
jobs in parallel. It also allows manual QA to reuse the same scenarios as automated tests.

Tests live in `test/new-e2e/tests/` and import this framework.

## Directory structure

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
│   ├── installers/       # Pulumi-free agent installers (Layer 2)
│   │   ├── hostagent/    # SSH-based agent install: Linux, Windows, macOS
│   │   ├── helmagent/    # Helm Go SDK agent install for Kubernetes
│   │   ├── dockeragent/  # Docker compose agent install via SSH
│   │   ├── ecsagent/     # AWS SDK-based ECS daemon service install
│   │   └── workloads/    # K8s workload deployment (nginx, redis, etc.)
│   ├── envdesc/          # Env descriptor: serialize/deserialize provisioned envs
│   ├── installspec/      # JSON-serializable install specification
│   ├── cliutil/          # common.Context implementation for CLI programs
│   └── components/       # Test-side wrappers: RemoteHost, Agent, FakeIntake
├── cmd/
│   └── e2e-install/      # Standalone install binary (reads env.json + spec.json)
├── scenarios/
│   └── aws/              # Pulumi programs: ec2, ec2docker, ecs, eks, kindvm
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
└── common/
    └── config/           # Configuration (AWS account, key pairs, agent params)
```

## Key concepts

### Three-layer architecture

```
Layer 1 (Pulumi)    provision VM/cluster/fakeintake → RawResources
                                        │
                              env populated (in-process) OR serialized to env.json
                                        │
Layer 2 (installer) hostagent / helmagent / dockeragent / ecsagent
                                        │
Layer 3 (test)      BaseSuite assertions via fakeintake
```

**Why three layers?**
- Infra (Layer 1) changes infrequently; agent install (Layer 2) is fast (~30s).
- Tests can run against a pre-provisioned env without touching Pulumi, enabling
  rapid iteration and the CI two-job pattern.
- QA and automated tests use identical scenarios: same provisioner options,
  same installer packages.

### Environments

An environment defines what infrastructure a test needs:

| Type | Components | Provisioner | Use when |
|------|-----------|-------------|----------|
| `environments.Host` | VM + Agent + FakeIntake | `awshost.Provisioner()` | System checks, agent commands, file-based config |
| `environments.WindowsHost` | Windows VM + Agent + FakeIntake | `winawshost.Provisioner()` | Windows-specific checks |
| `environments.DockerHost` | VM + Docker + FakeIntake | `awsdocker.Provisioner()` | Container checks, Docker integrations |
| `environments.Kubernetes` | K8s cluster + Agent + FakeIntake | various (eks, gke, aks, kind…) | K8s checks, DaemonSet, Cluster Agent |
| `environments.ECS` | ECS cluster + Agent + FakeIntake | `awsecs.Provisioner()` | ECS-specific tests |
| custom environment | user-defined struct | `e2e.WithPulumiProvisioner()` | Multi-VM, extra services |

### Provisioners

Provisioners create the environment's infrastructure. Built-in provisioners live in
`testing/provisioners/` organized by cloud provider. Every provisioner follows the same
pattern: Pulumi runs first (infra only), then PostProvision runs the installer.

```go
// Standard host on AWS EC2 — agent installed via SSH in PostProvision
awshost.Provisioner(
    awshost.WithRunOptions(
        ec2.WithEC2InstanceOptions(ec2.WithOS(e2eos.Ubuntu2204)),
        ec2.WithAgentOptions(
            agentparams.WithAgentConfig(config),
            agentparams.WithIntegration("check.d", checkConfig),
        ),
    ),
)

// Kubernetes (EKS) — agent installed via Helm in PostProvision
proveks.Provisioner(
    proveks.WithRunOptions(
        eks.WithAgentOptions(
            kubernetesagentparams.WithHelmValues(myHelmValues),
        ),
    ),
)
```

### PostProvisioner interface

The connection between Pulumi provisioning and the installer layer:

```go
// provisioners/postprovision.go
type PostProvisioner[Env any] interface {
    PostProvision(t *testing.T, env *Env)
}

// Wrap any TypedProvisioner with a post-Pulumi install step:
provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *environments.Host) {
    hostagent.Install(t, env, agentparams.WithAgentConfig("log_level: debug"))
})
```

`BaseSuite.SetupSuite` calls `PostProvision` after Pulumi finishes. `UpdateEnv`
(mid-suite reconfiguration) also calls `PostProvision`.

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
- `s.UpdateEnv(provisioner)` — re-provision mid-suite (triggers PostProvision)
- `s.Env().Agent.Configure(t, opts...)` — reconfigure running agent via SSH/Helm
  (faster than UpdateEnv for agent-only changes)
- `s.EventuallyWithT(fn, timeout, interval)` — retry assertions

### Attach mode (provision/install/test split)

Run the provision and install+test steps as separate jobs:

```bash
# Job 1: provision infra only, dump env descriptor
dda inv new-e2e-tests.run --targets=./tests/... --dump-env-descriptor=env.json

# Job 2: install agent + run tests (no Pulumi)
dda inv new-e2e-tests.run --targets=./tests/... --env-descriptor=env.json
```

Or via `SuiteOption`:
```go
e2e.Run(t, suite, e2e.WithProvisioner(awshost.Provisioner(...)),
    e2e.WithPreProvisionedEnv("/path/to/env.json"))
```

When `E2E_ENV_DESCRIPTOR` is set, `SetupSuite` loads the env from the descriptor
(skipping Pulumi), then runs PostProvision (agent install) + tests. Teardown is
skipped — the provision job owns the infrastructure lifetime.

## Agent configuration

Use `agentparams` to configure the host agent:

- `WithAgentConfig(yaml)` — override `datadog.yaml`
- `WithIntegration(name, yaml)` — add check config under `conf.d/`
- `WithLogs()` — enable log collection
- `WithSystemProbeConfig(yaml)` — system-probe config
- `WithFile(path, content, useSudo)` — place arbitrary files on the host

Use `kubernetesagentparams` for Kubernetes:

- `WithHelmValues(yaml)` — Helm values override
- `WithNamespace(ns)` — deploy to a custom namespace
- `WithDeployWindows()` — enable Windows node agent

## Advanced patterns

### Mid-suite reconfiguration (fast, no Pulumi cycle)

```go
func (s *mySuite) TestWithDebugLogging() {
    // Change config and restart agent via SSH — no Pulumi re-run
    s.Env().Agent.Configure(s.T(),
        agentparams.WithAgentConfig("log_level: debug"),
    )
    // ... assertions
}
```

### Custom environments (extra hosts/services)

Implement the `Initializable` and `Importable` interfaces on your custom struct,
then use `e2e.WithPulumiProvisioner`. See `test/new-e2e/tests/npm/` for examples.

### Using the e2e-install CLI

The `e2e-install` binary installs the agent on a pre-provisioned environment:

```bash
# Build:
cd test/e2e-framework && go build -o bin/e2e-install ./cmd/e2e-install/

# Use:
e2e-install --env env.json --spec spec.json
```

`env.json` is an `envdesc.Descriptor` (written by `WithDumpEnvDescriptor` or QA tasks).
`spec.json` is an `installspec.Spec` (agent version, config, Helm values, etc.).

## Keeping this file accurate

This file is part of the `AGENTS.md` hierarchy (see root `AGENTS.md` §
"Keeping AI context accurate"). Update it when environments, provisioners,
installers, or key APIs change.
