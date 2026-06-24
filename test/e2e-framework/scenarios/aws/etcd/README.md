# etcd Agent E2E Lab

This lab deploys `aws/etcd` through the Datadog Agent E2E framework.

## Commands

```bash
dda inv aws.etcd.create
dda inv aws.etcd.status
dda inv aws.etcd.check
dda inv aws.etcd.reload-check
dda inv aws.etcd.exec --role agent-host --command 'sudo datadog-agent status'
dda inv aws.etcd.ssh --role agent-host
dda inv aws.etcd.destroy
```

status runs the full `datadog-agent status` command on the Agent host and returns the complete output.

## Lab setup boundary

The invoke task module is generated from the `agint:generate-lab` standard template. Integration-specific setup belongs in the E2E scenario/component code for `aws/etcd`.

## Check

The default check command runs:

```bash
sudo -u dd-agent datadog-agent check etcd
```
