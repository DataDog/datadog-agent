# End-to-end tests

End-to-End (E2E) tests validate complete user workflows in production-like environments with real infrastructure and external services. The Datadog Agent uses the <<<repo("test/e2e-framework", "E2E framework")>>> in `test/e2e-framework` (formerly the separate `test-infra-definitions` repository) to provision and manage test environments. Tests are stored in the <<<repo("test/new-e2e", "test/new-e2e")>>> folder.

## In this section

- [Running tests](running.md) — execute an existing suite, iterate with dev mode, test a locally-built Agent
- [Writing tests](writing.md) — provisioners, environments, and assertions
- [Test dependencies](dependencies.md) — how to get a tool, image, or artifact onto a test host without reaching the public internet
- [Custom AMIs](custom-amis.md) — how machine images are selected, and how to add, bump or pin one
- [Performance benchmark](benchmarkeks/index.md) — `benchmarkeks`, which compares the resource usage of two Agent versions on EKS

## Prerequisites

/// admonition | Datadog Employees Only
    type: warning

E2E testing requires access to Datadog's internal cloud infrastructure and is currently limited to Datadog employees.
///

### Software Requirements

The simplest path is to run E2E tests from a [developer environment](../../../tutorials/dev/env.md) — it already has everything below installed.

/// details | Setting up outside a developer environment
    type: info
    open: false

- **Go 1.22 or later**
- **Python 3.9+**
- **dda tooling** - Install by following the [development requirements](../../../setup/required.md)
///

### Cloud Provider Access

You need access to the `account-admin-8h` role on the `agent-sandbox` AWS account. If `dda inv e2e.setup` (below) reports it can't reach that account, you don't have access yet — ask #agent-devx-help for it; the request process is documented internally on Confluence.

Kubernetes tests also need the `agent-dev` role on the `agent-qa` AWS account, to pull images from the ECR pull-through cache (see [Dependencies](dependencies.md)).

For Azure / GCP tests, pass `--with-azure` / `--with-gcp` when running the setup task (see below).

### One-time setup

Run the setup task once on a fresh machine:

```bash
dda inv e2e.setup
```

On the default (AWS-only) path, it:

- Checks for the AWS CLI, installing/updating Pulumi if needed, and configures a local Pulumi backend (`pulumi login --local`).
- Adds the `agent-sandbox` SSO profile to `~/.aws/config` if it isn't there yet, and creates an EC2 keypair via `aws-vault` (which may prompt you to authenticate).
- Asks one question — your GitHub team, used to tag AWS resources for cost attribution — and generates a Pulumi passphrase.

For Azure or GCP support (each requires that provider's CLI — `az` / `gcloud` — already installed):

```bash
dda inv e2e.setup --with-azure --with-gcp
```

The configuration is persisted to `~/.test_infra_config.yaml` (chmod `0600`, since it contains the auto-generated Pulumi passphrase). Re-running `dda inv e2e.setup` is idempotent — it prints `✓ already configured` checks and exits.

### GCP authentication

Local GCP provisioning uses Application Default Credentials (ADC), configured with
`gcloud auth application-default login`. GKE access through `gke-gcloud-auth-plugin`
also requires a valid gcloud CLI login, configured separately with `gcloud auth login`.
The framework checks the CLI credentials before creating a local GKE cluster and
returns instructions to log in again if they have expired.

CI supplies a service account key through `GOOGLE_APPLICATION_CREDENTIALS`; the
framework activates that account for gcloud as before and skips the local CLI check.

### Workspace support
The e2e test framework tooling supports being run from Datadog workspaces just like developer laptops. 
/// warning | Workspace compatibility of the e2e tooling
- **The workspace must be in the `us-east-1` region**. The VPN bridging the AWS account holding the workspace machines and the `datadog-agent-qa` account actually running the tests only exists in `us-east-1`.
- **You must use `zsh` in the workspace**. The `fish`-based workspace config lacks some setup that is required for the e2e tooling.
///
/// danger
The `e2e.*` Invoke tasks will **NOT WORK** in a workspace provisioned in `eu-west-3`.
///

- Stacks, EC2 key pairs and SSH keys are named after `$REAL_USER` and `$WORKSPACE_NAME`, not the OS user (which is `bits` for everyone on a workspace). Two workspaces owned by the same developer therefore get their own key pair and their own stacks, and `dda inv aws.destroy-vm` only ever sees the stacks created from that workspace.
- Workspaces are headless, so desktop notifications and the "copy to clipboard" prompts are skipped automatically. The connection command or password is printed instead.

/// warning | Processes that didn't start from an SSH login
The workspace's settings (`REAL_USER`, `WORKSPACE_NAME`, `AWS_VAULT_BACKEND=pass`, …) are in `/etc/environment`, which is only loaded when you log in over SSH. A process started some other way, such as an AI agent session or an IDE task, may not have them. The `e2e.*` tasks then fail in confusing ways: keypairs and stacks are named after `bits`, which every workspace shares, and `aws-vault` fails with `Specified keyring backend not available`. Check with `echo $WORKSPACE_NAME`, and if it's empty, load the file first:

```bash
set -a; . /etc/environment; set +a
```
///

#### Using a keypair created on another machine

The keypair name depends on the machine you ran `dda inv e2e.setup` on. If you already have one from your laptop, you can reuse it on the workspace instead of creating a new one:

1. Copy its `.pem` and `.pub` files from the laptop's `~/.ssh/` to the workspace's `~/.ssh/`.
1. Copy `configParams.aws.privateKeyPassword` too: `e2e.setup` encrypts the private keys it creates with a random passphrase and stores it in `~/.test_infra_config.yaml`.
1. Set `configParams.aws.keyPairName`, `privateKeyPath`, `publicKeyPath` and `privateKeyPassword` in the workspace's `~/.test_infra_config.yaml`, then run `dda inv e2e.setup --team=<github-team>` to fill in the rest. It reports `✓ AWS keypair '<name>' present on disk and in AWS`.

`e2e.setup` refuses to continue when AWS has a keypair with the expected name but its private key isn't on the machine (`AWS already has keypair ... local private/public key files ... are missing`). To check which AWS keypair a local key belongs to, compare the public keys: `aws ec2 describe-key-pairs --include-public-key` returns them.

## See Also

- [Test Categories](../../../guidelines/testing/test-categories.md) - Understanding different test types
- [Unit Testing](../unit.md) - Running unit tests
- [Manual QA](../manual-qa/index.md) - Provisioning the same infrastructure for manual inspection
- [Using Developer Environments](../../../tutorials/dev/env.md) - Setting up development environments
- <<<repo("test/new-e2e/codereview_guideline.md", "E2E test writing guidelines")>>> - The rules a new test is reviewed against
- <<<repo("test/e2e-framework", "test/e2e-framework")>>> - Infrastructure provisioning framework
