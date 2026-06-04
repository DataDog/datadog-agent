# Manual QA Infrastructure

The E2E framework can provision real cloud infrastructure for manual QA without
running automated tests. Environments stay alive until you explicitly destroy
them (or our cleaner destroys it, every week), giving you direct access (SSH, kubectl, RDP) to inspect the agent in a
realistic environment.

## Prerequisites

Complete the [one-time setup](../e2e.md#one-time-setup) from the E2E testing guide
before creating any environment.

If you do not want to rely on fake intake, and send data to the real Datadog backend, you should make sure you set a valid API in ~/.test_infra_config.yaml

## Stack lifecycle

Each environment is a named Pulumi stack. Stacks are automatically prefixed
with your OS username:

```
<username>-<stack-name>
# e.g.  alice-aws-vm    (default for the aws/vm scenario)
#       alice-my-qa     (with --stack-name my-qa)
```

Stacks **persist until you destroy them**. Run the matching `destroy-*` task
when you are done to avoid leaving cloud resources running.

Agents are automatically tagged `stackid:<stack-name>` so you can filter
metrics and logs in the Datadog UI to a specific environment.

## Scenarios

| Scenario | Command | What it creates |
|----------|---------|-----------------|
| [AWS VM](aws-vm.md) | `dda inv aws.create-vm` | EC2 instance + Agent |
| [AWS Docker VM](aws-docker.md) | `dda inv aws.create-docker` | EC2 + Docker + containerized Agent |
| [AWS EKS](aws-eks.md) | `dda inv aws.create-eks` | EKS cluster + Agent DaemonSet |
| [AWS ECS](aws-ecs.md) | `dda inv aws.create-ecs` | ECS cluster + Agent |
| [AWS KinD](aws-kind.md) | `dda inv aws.create-kind` | EC2 + KinD cluster + Agent |
| [Azure VM](azure-vm.md) | `dda inv az.create-vm` | Azure VM + Agent |
| [GCP VM](gcp-vm.md) | `dda inv gcp.create-vm` | GCP VM + Agent |

Pass `--no-interactive` to any scenario to disable the clipboard prompt and desktop notification, which is useful when running from an AI agent or CI context.

## See Also

- [Running E2E tests](../e2e.md) — automated test execution against the same infrastructure
