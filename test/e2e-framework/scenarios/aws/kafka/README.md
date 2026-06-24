# kafka Agent E2E Lab

This lab deploys `aws/kafka` through the Datadog Agent E2E framework.

## Commands

```bash
dda inv aws.kafka.create
dda inv aws.kafka.status
dda inv aws.kafka.check
dda inv aws.kafka.reload-check
dda inv aws.kafka.exec --role agent-host --command 'sudo datadog-agent status'
dda inv aws.kafka.ssh --role agent-host
dda inv aws.kafka.destroy
```

status runs the full `datadog-agent status` command on the Agent host and returns the complete output.

## Lab setup boundary

The invoke task module is generated from the `agint:generate-lab` standard template. Integration-specific setup belongs in the E2E scenario/component code for `aws/kafka`.

## Check

The default check command runs:

```bash
sudo -u dd-agent datadog-agent check kafka
```
