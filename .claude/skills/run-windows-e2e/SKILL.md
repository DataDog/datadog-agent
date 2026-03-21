---
name: run-windows-e2e
description: Run Windows E2E tests (MSI install tests or Fleet Automation/installer tests) locally against AWS-provisioned VMs
allowed-tools: Bash, Read, Glob, Grep, AskUserQuestion
argument-hint: "[suite] [TestFunctionName] [--build release|pipeline|local] [--pipeline-id <id>] [--version <version>]"
---

Run Windows E2E tests from `test/new-e2e/tests/windows/` or `test/new-e2e/tests/installer/windows/`.

## Instructions

### Step 1 — Parse `$ARGUMENTS`

Determine:
- **Suite**: which test suite to run (e.g. `install-test`, `service-test`, `agent-package`, `install-script`). If not provided, ask the user.
- **Test function**: specific `TestXxx` function to run. Most suites expect exactly one test per run — ask the user which one if not specified.
- **Artifact source**: `--build pipeline` (default), `--build local`, or `--build release`. If `--pipeline-id <id>` is given, pass it through.
- **Branch**: if the user mentions a specific branch (e.g. "from main", "latest main build"), note it — pass `--branch <name>` to `setup-env` in Step 3. The default is the current git branch, which may have no pipelines if it's a local feature branch.

Map suite names to Go package paths:

| Suite | Package path |
|-------|-------------|
| `install-test` | `./test/new-e2e/tests/windows/install-test/...` |
| `service-test` | `./test/new-e2e/tests/windows/service-test/...` |
| `fips-test` | `./test/new-e2e/tests/windows/fips-test/...` |
| `domain-test` | `./test/new-e2e/tests/windows/domain-test/...` |
| `agent-package` | `./test/new-e2e/tests/installer/windows/suites/agent-package/...` |
| `install-script` | `./test/new-e2e/tests/installer/windows/suites/install-script/...` |
| `install-exe` | `./test/new-e2e/tests/installer/windows/suites/install-exe/...` |
| `ddot-package` | `./test/new-e2e/tests/installer/windows/suites/ddot-package/...` |
| `apm-inject-package` | `./test/new-e2e/tests/installer/windows/suites/apm-inject-package/...` |

If the user gives a partial name or test function, search with Glob/Grep under `test/new-e2e/tests/windows/` and `test/new-e2e/tests/installer/windows/` to resolve it.

### Step 2 — Check prerequisites

**a) Check `~/.test_infra_config.yaml`:**
```bash
test -f ~/.test_infra_config.yaml && echo "EXISTS" || echo "MISSING"
```

**b) Check `pulumi`:**
```bash
pulumi version 2>/dev/null || echo "MISSING"
```

If either is missing, offer to walk the user through setup via:
```bash
dda inv e2e.setup
```
This task (defined in `tasks/e2e_framework/setup/`) validates CLI tools, installs Pulumi if missing, prompts for AWS keypair paths, Datadog API key, and Pulumi passphrase, then writes `~/.test_infra_config.yaml`. Run it and wait for the user to complete the interactive prompts before continuing.

If prerequisites exist but `devMode` is not set in `~/.test_infra_config.yaml`, mention that setting `devMode: true` in that file will reuse VMs across runs (much faster for iterative development).

### Step 3 — Resolve artifact environment variables

Run `setup-env` with `--fmt json` to capture the required env vars as JSON — no manual `eval` step needed.

**From a pipeline (most common):**

```bash
dda inv new-e2e-tests.setup-env --build pipeline --fmt json [--branch <branch>] [--pipeline-id <id>]
```

The task auto-resolves a GitLab token via `ddtool` if configured. If that fails it will print an error asking for `GITLAB_TOKEN` to be set — in that case ask the user to provide it and retry with `GITLAB_TOKEN=<token> dda inv new-e2e-tests.setup-env ...`.

**From a local build:**
```bash
dda inv new-e2e-tests.setup-env --build local --fmt json
```
If `--build local` and the user hasn't built yet, remind them to run `dda inv msi.build` first (plus `dda inv msi.package-oci` for installer/OCI tests).

**Parse the JSON output** — it will be a map of `"VAR_NAME": "value"` pairs. Collect all key=value pairs; you will prepend them inline to the `go test` command in Step 5 so no shell `eval` is required.

### Step 4 — Check for stale state (dev mode only)

If `devMode: true` is configured and the user is rerunning a test, the previous VM may still have the agent installed from the prior run. Ask whether they've cleaned up:

- **MSI install tests** (`install-test/`, `domain-test/`, `fips-test/`): uninstall the agent via Add/Remove Programs or `MsiExec.exe /x {product-code}` on the remote VM.
- **Installer/Fleet Automation tests** (`installer/windows/`): run `datadog-installer.exe purge` on the remote VM.

If unsure, advise them to clean up to avoid state leaking between test runs.

### Step 5 — Build and confirm the `go test` command

The `-run` flag takes a Go regex. Always anchor with `$` — both at the suite
level and at the subtest level — to avoid accidentally running multiple tests
that share a name prefix (e.g. `TestUpgradeAgentPackage$` won't match
`TestUpgradeAgentPackageOCIBootstrap`).

```bash
go test -v -timeout 30m -tags test <package-path> -run <TestFunction>$
```

For example:
```bash
# MSI install test
go test -v -timeout 30m -tags test ./test/new-e2e/tests/windows/install-test/... -run TestInstall$

# Service lifecycle test
go test -v -timeout 30m -tags test ./test/new-e2e/tests/windows/service-test/... -run TestServiceBehaviorPowerShell$

# Fleet Automation — agent package
go test -v -timeout 30m -tags test ./test/new-e2e/tests/installer/windows/suites/agent-package/... -run "TestAgentUpgrades$/TestUpgradeAgentPackage$"
```

Show the full command to the user and confirm before running.

### Step 6 — Run the test

Warn the user that AWS SSO authentication may open a browser window automatically
when the test starts. The test will pause until the login is completed — this is
expected. If `AWS_PROFILE` is set to a non-sandbox profile it will cause auth
errors; advise them to `unset AWS_PROFILE` before running.

Run with `run_in_background: true` since tests provision real AWS VMs and can take 20–60 minutes.

```bash
go test -v -timeout 30m -tags test <package-path> -run <TestFunction>$
```

### Step 7 — Report results

When the test completes:
- Report pass/fail.
- On failure, point the user to test outputs at `~/e2e-output/latest/` (locally) — this directory contains WER crash dumps, agent logs, installer logs, and Windows event logs collected by the suite's `AfterTest` hook.
- If the stack is locked (`error: the stack is currently locked`), suggest:
  - `dda inv new-e2e-tests.clean` — remove Pulumi locks
  - `dda inv new-e2e-tests.clean -s` — also wipe local stack state
  - `dda inv new-e2e-tests.clean --output` — clear local test output
  - If cleanup says "Cleanup supported for local state only": run `pulumi login --local` first
- If there are AWS auth errors (`not authorized to perform: ecr:BatchGetImage`), check whether `AWS_PROFILE` is set to a non-sandbox profile — unset it and retry. The framework automatically uses `sso-agent-sandbox-account-admin`.

## Accessing the test VM (dev mode)

When `devMode` is enabled the VM persists after the test. To connect:

**Find the stack name** — either from test output (`Creating workspace for stack: user-e2e-<suitename>-<hash>`) or:
```bash
pulumi stack ls --all
```

**Get connection details (IP, username, password):**
```bash
pulumi stack --stack organization/e2elocal/<stack-name> output --json --show-secrets
```

**RDP** (opens RDP, prints credentials, copies password to clipboard):
```bash
dda inv aws.rdp-vm --stack-name <stack-name>
```
Note: `dda inv aws.show-vm` does NOT work for e2e test stacks — it prepends
the username twice and looks for the wrong output key (`"aws-vm"` instead of
`"dd-Host-aws-vm"`).

**SSH** — use the `privateKeyPath` from `~/.test_infra_config.yaml`:
```bash
ssh -i <privateKeyPath> -o StrictHostKeyChecking=no <username>@<ip> "<command>"
```

Check `osFamily` in the JSON output to determine the remote shell:
- `osFamily: 1` = Linux → use `&&` or `;` (bash)
- `osFamily: 2` = Windows → use `;` as separator (PowerShell, `&&` is invalid)

## Notes

- These tests provision real AWS EC2 Windows VMs — valid AWS credentials (`agent-sandbox` account) and `PULUMI_CONFIG_PASSPHRASE` (or equivalent in config) are required. These VMs can be accessed with SSH, the key is configued in `~/.test_infra_config.yaml`. The test's pulumi state contains the IP address, username, and password.
- Most tests install the agent as part of the test and expect a clean starting state. Run them one at a time.
- Use `E2E_DEV_MODE=true` (or `devMode: true` in `~/.test_infra_config.yaml`) to reuse the VM across runs during development.
- See `test/new-e2e/tests/windows/running_tests.md` for a full reference.
