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
│   ├── provisioners/     # Provisioner interfaces + Pulumi implementation
│   │   └── aws/          # AWS provisioners (host, docker, ecs, kubernetes)
│   └── components/       # Test-side wrappers: RemoteHost, Agent, FakeIntake
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
├── common/
│   └── config/           # Configuration (AWS account, key pairs, agent params)
└── README.md             # Full setup and troubleshooting guide
```

## Key concepts

### Environments

An environment defines what infrastructure a test needs:

| Type | Components | Use when |
|------|-----------|----------|
| `environments.Host` | VM + Agent + FakeIntake | System checks, agent commands, file-based config |
| `environments.DockerHost` | VM + Docker + FakeIntake | Container checks, Docker integrations |
| `environments.Kubernetes` | K8s cluster + Agent + FakeIntake | K8s checks, DaemonSet, Cluster Agent |
| `environments.ECS` | ECS cluster + Agent + FakeIntake | ECS-specific tests |

### Provisioners

Provisioners create the environment's infrastructure:

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
- `s.EventuallyWithT(fn, timeout, interval)` — retry assertions

## Agent configuration

Use `agentparams` to configure the agent on provisioned infrastructure:

- `WithAgentConfig(yaml)` — override `datadog.yaml`
- `WithIntegration(name, yaml)` — add check config under `conf.d/`
- `WithLogs()` — enable log collection
- `WithSystemProbeConfig(yaml)` — system-probe config
- `WithFile(path, content, useSudo)` — place arbitrary files on the host

## Beyond stock environments

Not all tests fit the four stock environments. Common advanced patterns:

- **Custom environment structs** — define your own struct with extra components
  (e.g., a second `RemoteHost`, multiple fakeintakes, an HTTPBin service).
  Use `e2e.WithPulumiProvisioner()` to wire it up with inline Pulumi code.
  See `test/new-e2e/tests/npm/` and `test/new-e2e/tests/ha-agent/` for examples.
- **`e2e.WithUntypedPulumiProvisioner()`** — escape hatch for fully custom Pulumi
  programs when no typed environment fits.
- **`s.UpdateEnv(provisioner)`** — re-provision the agent mid-suite (e.g., change
  config, toggle features) without destroying the underlying infra. Widely used.
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

## Adding a new provisioner or component

### New cloud resource

1. Add Pulumi resource in `resources/<cloud>/`
2. Wire it into a scenario in `scenarios/<cloud>/`
3. Export outputs matching the environment type's interface

### New environment type

1. Define struct in `testing/environments/`
2. Implement `Initializable`, `Diagnosable` interfaces
3. Create a matching provisioner in `testing/provisioners/`

### New agent deployment method

1. Add component in `components/datadog/`
2. Create `agentparams` options if needed
3. Wire into an existing or new scenario

## Key files

- `testing/e2e/suite.go` — `BaseSuite` and `Run()` (test entry point)
- `testing/e2e/suite_params.go` — `SuiteOption` (WithProvisioner, WithDevMode, etc.)
- `testing/environments/host.go` — Host environment definition
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
