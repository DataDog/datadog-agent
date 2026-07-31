# End-to-end tests

End-to-End (E2E) tests validate complete user workflows in production-like environments with real infrastructure and external services. The Datadog Agent uses the <<<repo("test/e2e-framework", "test-infra-definitions")>>> framework to provision and manage test environments. Tests are stored in the <<<repo("test/new-e2e", "test/new-e2e")>>> folder.

## In this section

- [Running tests](running.md) — execute an existing suite, iterate with dev mode, test a locally-built Agent
- [Writing tests](writing.md) — provisioners, environments, and assertions
- [Test dependencies](dependencies.md) — how to get a tool, image, or artifact onto a test host without reaching the public internet
- [Custom AMIs](custom-amis.md) — how machine images are selected, and how to add, bump or pin one

## Prerequisites

/// admonition | Datadog Employees Only
    type: warning

E2E testing requires access to Datadog's internal cloud infrastructure and is currently limited to Datadog employees. This limitation is temporary and may be expanded in the future.
///


### Software Requirements

Before running E2E tests, ensure you have the following installed:

- **Go 1.22 or later**
- **Python 3.9+**
- **dda tooling** - Install by following the [development requirements](../../../setup/required.md)

### Cloud Provider Setup

You need access to the `account-admin-8h` role on the `agent-sandbox` AWS account, with the SSO profile (`sso-agent-sandbox-account-admin-8h`) already in your `~/.aws/config` and an active aws-vault session. AWS authentication is handled outside of `e2e.setup` — typically by your org's onboarding tooling, or manually with `aws-vault login`.

For Azure / GCP tests, pass `--with-azure` / `--with-gcp` when running the setup task (see below).

### One-time setup

Run the setup task once on a fresh machine. The default path is AWS-only and asks at most one question (your GitHub team, used to tag resources). It auto-creates the EC2 keypair (using your existing aws-vault session) and generates a Pulumi passphrase.

```bash
dda inv e2e.setup
```

For Azure or GCP support:

```bash
dda inv e2e.setup --with-azure --with-gcp
```

The configuration is persisted to `~/.test_infra_config.yaml` (chmod `0600`, since it contains the auto-generated Pulumi passphrase). Re-running `dda inv e2e.setup` is idempotent — it prints `✓ already configured` checks and exits.

## See Also

- [Test Categories](../../../guidelines/testing/test-categories.md) - Understanding different test types
- [Unit Testing](../unit.md) - Running unit tests
- [Manual QA](../manual-qa/index.md) - Provisioning the same infrastructure for manual inspection
- [Using Developer Environments](../../../tutorials/dev/env.md) - Setting up development environments
- <<<repo("test/new-e2e/codereview_guideline.md", "E2E test writing guidelines")>>> - The rules a new test is reviewed against
- <<<repo("test/e2e-framework", "test-infra-definitions")>>> - Infrastructure provisioning framework
