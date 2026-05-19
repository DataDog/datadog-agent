# Integration Labs with the Agent E2E Framework

This note captures an alternative implementation path for the `ddev lab` idea described in the integrations-core design PR: https://github.com/DataDog/integrations-core/pull/23682.

The PR proposes a `ddev lab` workflow for provisioning remote, seeded, load-generating integration environments with a Datadog Agent already configured to run the integration. The current prototype in this branch demonstrates how the same outcome can be achieved by leveraging the Agent DevX team's existing `test/e2e-framework` instead of building a new infrastructure layer from scratch.

## Summary

Use `datadog-agent/test/e2e-framework` as the execution backend for integration labs:

- Pulumi scenarios live in `test/e2e-framework/scenarios/`.
- Invoke tasks expose simple commands such as `dda inv aws.create-milvus`.
- The scenario provisions cloud infrastructure, starts the workload, starts a load generator, and deploys a Datadog Agent.
- The Agent is configured through the existing Agent E2E framework primitives, Docker Compose manifests, or Autodiscovery labels.
- Cleanup uses the matching destroy task, for example `dda inv aws.destroy-milvus`.

The Milvus lab in this branch is the first concrete example of that approach.

## How a lab maps to the E2E framework

A lab is split into three pieces:

1. **Workload component**
   - Location: `test/e2e-framework/components/datadog/apps/<integration>/`
   - Contains Docker Compose files, seed scripts, load containers, or other workload artifacts.
   - For Milvus, this includes Milvus standalone, etcd, MinIO, and a Python load generator.

2. **Pulumi scenario**
   - Location: `test/e2e-framework/scenarios/<cloud>/<integration>/`
   - Provisions the host or cluster using existing E2E framework resources.
   - Starts Docker, workload containers, fakeintake if requested, and the Agent.
   - Exports standard stack outputs so existing tooling can discover hosts and Agent containers.

3. **Invoke task**
   - Location: `tasks/e2e_framework/<cloud>/<integration>.py`
   - Exposes a user-facing command like `dda inv aws.create-<integration>`.
   - Passes standard options for stack name, Agent image, fakeintake, architecture, and environment variables.
   - Prints useful SSH, Docker, and Agent status commands after creation.

## Example lifecycle

Create a Milvus lab:

```bash
dda inv aws.create-milvus --stack-name my-milvus-lab
```

Create it with fakeintake:

```bash
dda inv aws.create-milvus --stack-name my-milvus-lab --use-fakeintake
```

Use a specific Agent image:

```bash
dda inv aws.create-milvus \
  --stack-name my-milvus-lab \
  --full-image-path gcr.io/datadoghq/agent:latest
```

Destroy the lab:

```bash
dda inv aws.destroy-milvus --stack-name my-milvus-lab
```

The destroy command delegates to Pulumi destroy and removes the stack state.

## Relationship to the `ddev lab` design

The integrations-core PR describes two layers:

- an AI-assisted research phase that generates lab artifacts;
- a deterministic execution layer that provisions infrastructure and runs the lab.

The Agent E2E framework can serve as the deterministic execution layer today. Instead of creating a new Pulumi Automation API backend, `ddev lab create <integration>` could eventually shell out to or wrap the corresponding E2E framework task:

```bash
ddev lab create milvus
# internally maps to something like:
dda inv aws.create-milvus --stack-name <derived-name> ...
```

The research phase could still live in integrations-core and generate reviewed artifacts, but the generated execution target would be an Agent E2E framework scenario rather than a separate infrastructure implementation.

## Benefits

- **Reuses existing infrastructure**: Pulumi setup, stack naming, AWS account configuration, SSH keys, fakeintake support, and Agent image selection already exist.
- **Consistent Agent behavior**: Labs use the same Agent deployment paths as Agent E2E tests.
- **Fast path to prototypes**: A new lab can start as a Docker Compose workload plus a small AWS scenario and invoke task.
- **Known cleanup model**: Destroy tasks already call `pulumi destroy --remove`.
- **Optional fakeintake**: Labs can validate metric/log payloads without requiring a real Datadog API key.

## Trade-offs and open questions

- **Repository ownership**: Integration lab artifacts may naturally belong in integrations-core, while the execution backend lives in datadog-agent.
- **User experience**: Integration developers use `ddev`; Agent E2E tasks use `dda inv`. A thin `ddev lab` wrapper may be needed.
- **Generated code review**: If AI research generates scenarios, reviewers need clear conventions for where generated files live and how they are maintained.
- **Multi-node topologies**: The E2E framework supports custom Pulumi scenarios, but complex clusters such as Lustre still require careful scenario-specific modeling.
- **Garbage collection**: The current model relies on explicit destroy tasks. Additional TTL or registry-based cleanup would still need to be designed if labs are expected to be long-lived.

## Suggested convention for future labs

For each new integration lab:

```text
test/e2e-framework/components/datadog/apps/<integration>/
  docker.go
  docker-compose.yaml

test/e2e-framework/scenarios/aws/<integration>/
  run.go
  BUILD.bazel

tasks/e2e_framework/aws/<integration>.py
```

The task should be named:

```bash
dda inv aws.create-<integration>
dda inv aws.destroy-<integration>
```

The scenario should support:

- custom stack names;
- fakeintake;
- Agent image overrides;
- architecture selection where practical;
- clear connection commands including SSH identity when available;
- a load generator that continuously exercises non-empty integration metrics.
