# End-to-end tests

End-to-End (E2E) tests validate complete user workflows in production-like environments with real infrastructure and external services. The Datadog Agent uses the <<<repo("test/e2e-framework", "E2E framework")>>> in `test/e2e-framework` (formerly the separate `test-infra-definitions` repository) to provision and manage test environments. Tests are stored in the <<<repo("test/new-e2e", "test/new-e2e")>>> folder.

## In this section

- [Running tests](running.md) — execute an existing suite, iterate with dev mode, test a locally-built Agent
- [Writing tests](writing.md) — provisioners, environments, and assertions
- [Test dependencies](dependencies.md) — how to get a tool, image, or artifact onto a test host without reaching the public internet
- [Custom AMIs](custom-amis.md) — how machine images are selected, and how to add, bump or pin one

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

## See Also

- [Test Categories](../../../guidelines/testing/test-categories.md) - Understanding different test types
- [Unit Testing](../unit.md) - Running unit tests
- [Manual QA](../manual-qa/index.md) - Provisioning the same infrastructure for manual inspection
- [Using Developer Environments](../../../tutorials/dev/env.md) - Setting up development environments
- <<<repo("test/new-e2e/codereview_guideline.md", "E2E test writing guidelines")>>> - The rules a new test is reviewed against
- <<<repo("test/e2e-framework", "test/e2e-framework")>>> - Infrastructure provisioning framework
