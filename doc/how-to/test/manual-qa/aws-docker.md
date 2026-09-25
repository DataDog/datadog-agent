# AWS Docker VM

Provisions an EC2 instance with Docker installed and the Datadog Agent running
as a container. Use this to test container-specific agent behavior or Docker
integrations.

## Create

```bash
dda inv aws.create-docker
```

### Key options

| Flag | Default | Description |
|------|---------|-------------|
| `--stack-name` | `aws-dockervm` | Suffix for the Pulumi stack name |
| `--architecture` | `x86_64` | CPU architecture: `x86_64` or `arm64` |
| `--agent-version` | latest | Container image tag (e.g. `7.58.0-rc.3`) |
| `--full-image-path` | — | Full registry path to a custom agent image |
| `--agent-flavor` | — | Agent flavor (e.g. `datadog-fips-agent`) |
| `--agent-env` | — | Extra env vars for the agent container (`VAR1=val1,VAR2=val2`) |
| `--use-fakeintake` | `false` | Deploy a local mock intake alongside the agent |
| `--install-agent` | `true` | Set to `false` to provision a Docker host without the agent |

### Examples

```bash
# Docker host with a custom image
dda inv aws.create-docker --full-image-path=my-registry/agent:my-tag

# ARM Docker host
dda inv aws.create-docker --architecture=arm64
```

## Connect

SSH connection details are printed after the stack is created. The agent runs
as a container; use `docker ps` / `docker logs` once connected.

## Destroy

```bash
dda inv aws.destroy-docker
```

## Limitations

- The agent runs in a Docker container, not as a host package. Host-agent
  behavior (systemd, OS-level checks) is not testable with this scenario — use
  [AWS VM](aws-vm.md) instead.
