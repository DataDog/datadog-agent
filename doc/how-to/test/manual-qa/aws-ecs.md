# AWS ECS

Provisions an ECS cluster with the Datadog Agent running as a container task.
Use this to test ECS-specific agent behavior or Fargate integrations.

## Create

```bash
dda inv aws.create-ecs
```

### Key options

| Flag | Default | Description |
|------|---------|-------------|
| `--stack-name` | `aws-ecs` | Suffix for the Pulumi stack name |
| `--use-fargate` | `true` | Use Fargate as the capacity provider |
| `--linux-node-group` | `true` | Add an ECS-optimized Linux node group (EC2-backed) |
| `--linux-arm-node-group` | `false` | Add an ECS-optimized Linux ARM node group (EC2-backed) |
| `--bottlerocket-node-group` | `true` | Add a Bottlerocket node group (EC2-backed) |
| `--windows-node-group` | `false` | Add a Windows LTSC node group (EC2-backed) |
| `--agent-version` | latest | Container image tag |
| `--full-image-path` | — | Full registry path to a custom agent image |
| `--agent-flavor` | — | Agent flavor (e.g. `datadog-fips-agent`) |
| `--agent-env` | — | Extra env vars for the agent container (`VAR1=val1,VAR2=val2`) |
| `--no-interactive` | — | Disable clipboard prompt and desktop notification |

### Examples

```bash
# Fargate-only cluster (default)
dda inv aws.create-ecs

# ECS cluster with EC2-backed Windows nodes (no Fargate)
dda inv aws.create-ecs --use-fargate=false --windows-node-group=true --linux-node-group=false --bottlerocket-node-group=false

# ECS with a custom agent image
dda inv aws.create-ecs --full-image-path=my-registry/agent:my-tag
```

## Connect

The agent runs as an ECS task. Use the ECS console or the AWS CLI to inspect
task logs and container status:

```bash
aws-vault exec sso-agent-sandbox-account-admin-8h -- aws ecs list-tasks --cluster <cluster-name>
```

## Destroy

```bash
dda inv aws.destroy-ecs
```

## Limitations

- The agent runs as a container task; host-agent behavior is not testable.
- FakeIntake is not supported for ECS.
