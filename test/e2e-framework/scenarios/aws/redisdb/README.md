# redisdb Agent E2E Lab

This lab deploys `aws/redisdb` through the Datadog Agent E2E framework.

## Commands

```bash
dda inv aws.redisdb.create
dda inv aws.redisdb.status
dda inv aws.redisdb.check
dda inv aws.redisdb.reload-check
dda inv aws.redisdb.exec --role agent-host --command 'sudo datadog-agent status'
dda inv aws.redisdb.ssh --role agent-host
dda inv aws.redisdb.destroy
```

status runs the full `datadog-agent status` command on the Agent host and returns the complete output.

## Lab setup boundary

The invoke task module is generated from the `agint:generate-lab` standard template. Integration-specific setup belongs in the E2E scenario/component code for `aws/redisdb`.

## Check

The default check command runs:

```bash
sudo -u dd-agent datadog-agent check redisdb
```
