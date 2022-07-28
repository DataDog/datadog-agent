# Datadog Agent e2e tests

## Run locally

Ensure you are connected to AppGate.

Login to your AWS account with your IAM credentials

```bash
aws-vault exec sandbox-account-admin --
```

Install Pulumi

```bash
brew install pulumi/tap/pulumi
```

Create a local Pulumi state manager

```bash
pulumi login --local
```

Add a PULUMI_CONFIG_PASSPHRASE to your Terminal rc a passphrase

```bash
export PULUMI_CONFIG_PASSPHRASE=citest
```

Install aws plugin

```bash
pulumi plugin install resource aws
```

Run

```bash
go test <name of the test>
```

Example

```bash
go test test/new-e2e/containers/hello_world_test.go
```

## Troubleshoot

If you get

```bash
aws-vault: error: exec: aws-vault sessions should be nested with care, unset $AWS_VAULT to force
```

Run

```bash
unset AWS_VAULT && aws-vault exec sandbox-account-admin --
```
