# postgres Agent E2E Lab

This lab deploys `aws/postgres` through the Datadog Agent E2E framework.

## Commands

```bash
dda inv aws.postgres.create
dda inv aws.postgres.status
dda inv aws.postgres.check
dda inv aws.postgres.reload-check
dda inv aws.postgres.exec --role aws-agent-host --command 'sudo datadog-agent status'
dda inv aws.postgres.ssh --role aws-agent-host
dda inv aws.postgres.destroy
```

status runs the full `datadog-agent status` command on the Agent host and returns the complete output.

## Lab setup boundary

The invoke task module is generated from the `agint:generate-lab` standard template. Integration-specific setup belongs in the E2E scenario/component code for `aws/postgres`.

## Check

The default check command runs:

```bash
sudo -u dd-agent datadog-agent check postgres
```
