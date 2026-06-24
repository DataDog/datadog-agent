# aws_neuron Agent E2E Lab

This lab deploys `aws/aws_neuron` through the Datadog Agent E2E framework.

## Commands

```bash
dda inv aws.aws_neuron.create
dda inv aws.aws_neuron.status
dda inv aws.aws_neuron.check
dda inv aws.aws_neuron.reload-check
dda inv aws.aws_neuron.exec --role agent-host --command 'sudo datadog-agent status'
dda inv aws.aws_neuron.ssh --role agent-host
dda inv aws.aws_neuron.destroy
```

status runs the full `datadog-agent status` command on the Agent host and returns the complete output.

## Lab setup boundary

The invoke task module is generated from the `agint:generate-lab` standard template. Integration-specific setup belongs in the E2E scenario/component code for `aws/aws_neuron`.

## Check

The default check command runs:

```bash
sudo -u dd-agent datadog-agent check aws_neuron
```
