# kueue Agent E2E Lab

This lab deploys `aws/kueue` through the Datadog Agent E2E framework.

## Commands

```bash
dda inv aws.kueue.create
dda inv aws.kueue.status
dda inv aws.kueue.check
dda inv aws.kueue.exec --command 'get pods -A'
dda inv aws.kueue.ssh
dda inv aws.kueue.destroy
```

create and ssh print an `export KUBECONFIG=<path>` command. status runs `kubectl exec -n datadog ds/datadog -- agent status`, check runs `kubectl exec -n datadog ds/datadog -- agent check <check>`, and exec runs explicit kubectl commands locally through the generated kubeconfig.

## Lab setup boundary

The invoke task module is generated from the `agint:generate-lab` standard template. Integration-specific setup belongs in the E2E scenario/component code for `aws/kueue`.

## Check

The default check command runs the Agent check through the Datadog DaemonSet:

```bash
kubectl exec -n datadog ds/datadog -- agent check kueue
```

Use `exec --command '<kubectl args>'` for explicit kubectl commands.
