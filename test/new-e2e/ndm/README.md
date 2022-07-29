# Datadog Agent NDM e2e test PoC

This PoC does
* create one docker container based on agent's nightly build
* create one container based on snmpsim
* clone `integrations-core` to get access to snmprec from snmp/test/data
* runs an snmpwalk from the agent to the snmpsim container

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

Add a PULUMI_CONFIG_PASSPHRASE to your Terminal rc.

```bash
export PULUMI_CONFIG_PASSPHRASE=citest
```

Install aws plugin

```bash
pulumi plugin install resource aws
```

Initialize the test environment

```bash
go test --run TestSetup test/new-e2e/ndm/snmp_test.go -v
```

Run the test

```bash
go test --run TestAgentSNMP test/new-e2e/ndm/snmp_test.go -v
```

And tear down the stack

```bash
 pulumi destroy -s ci-agent-ndm
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

If you get

```bash
error: could not create stack: the stack is currently locked by 1 lock(s). Either wait for the other process(es) to end or manually delete the lock file(s).
```

Run

```bash
rm -rf ~/.pulumi/locks
```

If you get

```bash
dial tcp 172.29.139.15:22: connect: connection refused
```

Make sure you are connected through AppGate
