# Dynamic infrastructures for test

This repository contains IaC code based on Pulumi to provision dynamic test infrastructures for testing.

## Prerequisites

To run scripts and code in this repository, you will need:

- [Go](https://golang.org/doc/install) 1.22 or later. You'll also need to set your `$GOPATH` and have `$GOPATH/bin` in your path.
- Python 3.9+ along with development libraries for tooling.
- `account-admin` role on AWS `agent-sandbox` account. Ensure it by running `aws-vault login sso-agent-sandbox-account-admin`
- [dda](https://datadoghq.dev/datadog-agent/setup/#tooling) installed on your laptop

  ```bash
  aws-vault login sso-agent-sandbox-account-admin
  ```

This guide is tested on **MacOS**.

##### List of Linux dependencies

```bash
sudo apt install libnotify-bin
```

## Quick start guide

1. Clone this repository

```bash
cd ~/dd && git clone git@github.com:DataDog/datadog-agent.git
```

2. Install Python dependencies

```bash
cd ~/dd/datadog-agent/test/e2e-framework && dda -v self dep sync -f legacy-e2e -f legacy-github
```

3. Add a PULUMI_CONFIG_PASSPHRASE to your Terminal rc file. Create a random password using 1Password and store it there

```bash
export PULUMI_CONFIG_PASSPHRASE=<random password stored in 1Password>
```

4. Run and follow the setup script

```bash
dda inv setup
```

### Create an environment for manual tests

Invoke tasks help deploying most common environments - VMs, Docker, ECS, EKS. Run `dda inv -l` to learn more.

To get the list of available tasks
```bash
❯ dda inv -l
```

Run any `-h` on any of the available tasks for more information

## MacOS support

The `aws.create-vm` task should allow you to spin up a MacOS instance using `-o macos` flag. Note that spinning such an instance is expensive because it requires a dedicated host. When you have one running please reuse it instead of creating new instances every time you need it. The cleaner will automatically get rid of the dedicated hosts.
## Troubleshooting

### Environment and configuration

The `setup.debug` invoke task will check for common mistakes such as key unavailable in configured AWS region, ssh-agent not running, invalid key format, and more.

```
aws-vault exec sso-agent-sandbox-account-admin -- dda inv setup.debug
aws-vault exec sso-agent-sandbox-account-admin -- dda inv setup --debug --no-interactive
```


## Interact with Pulumi

Note: For most of the users interacting with Pulumi directly should not be required, you should be able to rely exclusively on the available tasks.

### Pulumi: Stack & Storage

Pulumi requires to store/retrieve the state of your `Stack`.
In Pulumi, `Stack` objects represent your actual deployment:

- A `Stack` references a `Project` (a folder with a `Pulumi.yaml`, for instance root folder of this repo)
- A `Stack` references a configuration file called `Pulumi.<stack_name>.yaml`
  This file holds your `Stack` configuration.
  If it does not exist, it will be created.
  If it exists and you input some settings through the command line, using `-c`, it will update the `Stack` file.

When performing operations on a `Stack`, Pulumi will need to store a state somewhere (the Stack state).
Normally the state should be stored in a durable storage (e.g. S3-like), but for testing purposes
local filesystem could be used.

To choose a default storage provider, use `pulumi login` (should be only done once):

```
# Using local filesystem (state will be stored in ~/.pulumi)
pulumi login --local

# Using storage on Cloud Storage (GCP)
# You need to create the bucket on sandbox.
# You also need to have sandbox as your current tenant in gcloud CLI.
pulumi login gs://<your_name>-pulumi
```

More information about state can be retrieved at: https://www.pulumi.com/docs/intro/concepts/state/

Finally, Pulumi is encrypting secrets in your `Pulumi.<stack_name>.yaml` (if entered as such).
To do that, it requires a password. For dev purposes, you can simply store the password in the `PULUMI_CONFIG_PASSPHRASE` variable in your `~/.zshrc`.

### Creating a stack with `pulumi up`

In this example, we're going to create an ECS Cluster:

```
# You need to have a DD APIKey in variable DD_API_KEY
pulumi up -C test/e2e-framework/run -c scenario=aws/ecs -c ddinfra:aws/defaultKeyPairName=<your_exisiting_aws_keypair_name> -c ddinfra:env=aws/agent-sandbox -c ddagent:apiKey=$DD_API_KEY -s <your_name>-ecs-test
```

In case of failure, you may update some parameters or configuration and run the command again.
Note that all `-c` parameters have been set in your `Pulumi.<stack_name>.yaml` file.

**NOTE:** Do not commit your Stack file.

### Destroying a stack

Once you're finished with the test environment you've created, you can safely delete it.
To do this, we'll use the `destroy` operation referencing our `Stack` file:

```
pulumi destroy -s <your_name>-ecs-test
```

Note that we don't need to use `-c` again as the configuration values were put into the `Stack` file.
This will destroy all cloud resources associated to the Stack, but the state itself (mostly empty) will still be there.
To remove the stack state:

```
pulumi stack rm <your_name>-ecs-test
```

## Quick start: A VM with Docker(/Compose) with Agent deployed

```
# You need to have a DD APIKey in variable DD_API_KEY
pulumi up -C test/e2e-framework/run -c scenario=aws/dockervm -c ddinfra:aws/defaultKeyPairName=<your_exisiting_aws_keypair_name> -c ddinfra:env=aws/agent-sandbox -c ddagent:apiKey=$DD_API_KEY -c ddinfra:aws/defaultPrivateKeyPath=$HOME/.ssh/id_rsa -s <your_name>-docker
```

## Quick start: Create an ECS EC2 (Windows/Linux) + Fargate (Linux) Cluster

```
# You need to have a DD APIKey in variable DD_API_KEY
pulumi up -C test/e2e-framework/run -c scenario=aws/ecs -c ddinfra:aws/defaultKeyPairName=<your_exisiting_aws_keypair_name> -c ddinfra:env=aws/agent-sandbox -c ddagent:apiKey=$DD_API_KEY -s <your_name>-ecs
```

## Quick start: Create an EKS (Linux/Windows) + Fargate (Linux) Cluster + Agent (Helm)

```
# You need to have a DD APIKey AND APPKey in variable DD_API_KEY / DD_APP_KEY
pulumi up -C test/e2e-framework/run -c scenario=aws/eks -c ddinfra:aws/defaultKeyPairName=<your_exisiting_aws_keypair_name> -c ddinfra:env=aws/agent-sandbox -c ddagent:apiKey=$DD_API_KEY -c ddagent:appKey=$DD_APP_KEY -s <your_name>-eks
```

## Quick start: Create a GKE Standard + Agent (Helm) or a GKE Autopilot + Agent (Helm)
**Prerequisites:**
- Install the GKE authentication plugin: `gcloud components install gke-gcloud-auth-plugin`
- Add the plugin to your PATH: `export PATH="/opt/homebrew/share/google-cloud-sdk/bin:$PATH"`
- Authenticate with GCP: `gcloud auth application-default login`
```
# You need to have a DD APIKey AND APPKey in variable DD_API_KEY / DD_APP_KEY
# GKE Standard
pulumi up -C test/e2e-framework/run -c scenario=gcp/gke -c ddinfra:env=gcp/agent-sandbox -c ddinfra:gcp/defaultPublicKeyPath=$HOME/.ssh/id_ed25519.pub -c ddagent:apiKey=$DD_API_KEY -c ddagent:appKey=$DD_APP_KEY -s <your_name>-gke

# GKE Autopilot
pulumi up -C test/e2e-framework/run -c scenario=gcp/gke -c ddinfra:env=gcp/agent-sandbox -c ddinfra:gcp/defaultPublicKeyPath=$HOME/.ssh/id_ed25519.pub -c ddinfra:gcp/gke/enableAutopilot=true -c ddagent:apiKey=$DD_API_KEY -c ddagent:appKey=$DD_APP_KEY -s <your_name>-gke-autopilot
```

## Quick start: Create an OpenShift Cluster + Agent (Helm) on an OpenShift Cluster

```
# You need to have a DD APIKey AND APPKey in variable DD_API_KEY / DD_APP_KEY
pulumi up -C test/e2e-framework/run -c scenario=gcp/openshiftvm -c ddinfra:env=gcp/agent-sandbox -c ddinfra:gcp/openshift/pullSecretPath=<your_pull_secret_path -c ddinfra:gcp/enableNestedVirtualization=true -c ddinfra:gcp/defaultInstanceType=n2-standard-8 -c ddinfra:gcp/defaultPublicKeyPath=$HOME/.ssh/id_ed25519.pub -c ddagent:apiKey=$DD_API_KEY  -c ddagent:appKey=$DD_APP_KEY -s <your_name>-openshift
```
