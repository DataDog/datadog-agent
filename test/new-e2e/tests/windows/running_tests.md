# Running Windows E2E Tests

Instructions for running Windows E2E tests locally. Applies to both
MSI install tests (`tests/windows/`) and Fleet Automation / installer
tests (`tests/installer/windows/`).

If you are using Claude Code, the `/run-windows-e2e` skill can handle
the setup-env, environment variable wiring, and `go test` invocation
for you interactively.

## Prerequisites

Follow the general E2E setup guide first:
[docs/public/how-to/test/e2e.md](../../../../docs/public/how-to/test/e2e.md)

This covers:
- AWS `agent-sandbox` account access and `aws-vault` setup
- `PULUMI_CONFIG_PASSPHRASE` requirement
- Required tooling (Go, Python, `dda`)

The Windows tests provision real AWS EC2 Windows instances and require the
same AWS access as all other E2E tests. Without valid AWS credentials and
`PULUMI_CONFIG_PASSPHRASE` set, the test run will fail before any test code
executes.

## Persistent configuration

Most run parameters are easier to set once in `~/.test_infra_config.yaml`
rather than as environment variables each session. The relevant fields are
your AWS keypair paths, Datadog API key (required — the agent MSI install
needs a valid key), Pulumi passphrase (alternative to the
`PULUMI_CONFIG_PASSPHRASE` env var), and `devMode` (see below).

## Dev mode

By default each test run provisions a fresh VM and destroys it afterwards.
During local development this makes test-fix-rerun loops very slow. Set
`devMode: true` in `~/.test_infra_config.yaml` (shown above) to reuse the
same VM across runs. The VM is identified by the Pulumi stack name, which
is derived from the test package and suite name.

Alternatively set it per-run:

```bash
E2E_DEV_MODE=true go test -v -timeout 30m -tags test ./test/new-e2e/tests/windows/install-test/...
```

Or in VSCode:
```json
"go.testEnvVars": {
  "E2E_DEV_MODE": "true"
}
```

## Setting up artifact environment variables

Windows tests need to know which MSI and/or OCI package to use. The
`setup-env` helper configures the required environment variables.

### Using pipeline artifacts

Auto-detect the most recent successful pipeline on your current branch
(use `--branch main` to target `main` instead):

```powershell
# PowerShell
dda inv new-e2e-tests.setup-env --build pipeline --fmt powershell | Invoke-Expression
```

```bash
# Bash/WSL
eval "$(dda inv new-e2e-tests.setup-env --build pipeline)"
```

The task auto-resolves a GitLab token via `ddtool` if configured. If
that fails it will ask you to set `GITLAB_TOKEN` manually.

To target a specific pipeline:

```powershell
dda inv new-e2e-tests.setup-env --build pipeline --pipeline-id <id> --fmt powershell | Invoke-Expression
```

### Using local artifacts

Build the MSI first (add `msi.package-oci` if running installer tests):

```bash
dda inv msi.build
# produces omnibus/pkg/datadog-agent-<version>-x86_64.msi

dda inv msi.package-oci   # only needed for installer/windows tests
# produces omnibus/pkg/datadog-agent-<version>-windows-amd64.oci.tar
```

Then point the tests at the local build:

```powershell
dda inv new-e2e-tests.setup-env --build local --fmt powershell | Invoke-Expression
```

## Running a specific suite

The `-run` flag takes a Go regex. The trailing `$` anchors to an exact match —
important both at the suite level (preventing `TestInstall` from also matching
`TestInstallOpts`, `TestInstallFail`, etc.) and at the subtest level when
targeting a specific subtest (preventing `TestUpgradeAgentPackage` from also
matching `TestUpgradeAgentPackageOCIBootstrap` and other variants).

```bash
# MSI install tests — run one test function
go test -v -timeout 30m -tags test ./test/new-e2e/tests/windows/install-test/... -run TestInstall$

# Service lifecycle tests
go test -v -timeout 30m -tags test ./test/new-e2e/tests/windows/service-test/... -run TestServiceBehaviorPowerShell$

# Installer / Fleet Automation — agent package (install, upgrade, rollback, experiments)
go test -v -timeout 30m -tags test ./test/new-e2e/tests/installer/windows/suites/agent-package/... -run TestAgentInstalls$
```

AWS SSO authentication may open a browser window automatically when the test
starts — the test will pause until login completes. This is expected.

Most tests are designed to run one at a time — they install the agent as part
of the test and expect a clean starting state. When using dev mode and reusing
a VM, uninstall the agent (MSI tests) or run `datadog-installer.exe purge`
(installer tests) between runs to avoid state leaking between tests.

See each suite's AGENTS.md for the full list of test functions and the CI job
names that map to them.

## Test outputs

Test output files (logs, crash dumps, event logs, agent logs) are written via
`runner.GetProfile().CreateOutputSubDir()` to:

```
~/e2e-output/<suite-name>/<timestamp>/
~/e2e-output/latest/                    ← symlink to the most recent run
```

When a test fails, the suite's `AfterTest` hook automatically collects
diagnostics into the output directory — WER crash dumps, Windows event logs
(`.evtx`), agent logs, and installer logs depending on the suite. Check
`~/e2e-output/latest/` first when investigating a failure.

In CI, outputs are stored as GitLab job artifacts and accessible from the
job's artifact browser in the pipeline UI.

## Accessing the test VM

When `devMode` is enabled the VM stays alive after the test. You can find
connection details via the Pulumi stack.

### Finding the stack name

Two ways:
- From test output: look for `Creating workspace for stack: user-e2e-<suitename>-<hash>`
- From the CLI: `pulumi stack ls --all` — find the entry matching the suite name

The stack name format is `user-e2e-<suitename-lowercase>-<hash>`.

### Getting connection details

```bash
# Full JSON: IP address, username, password
pulumi stack --stack organization/e2elocal/<stack-name> output --json --show-secrets
```

The output includes `address` (private IP), `username` (`Administrator`), and
`password`. The OS family and flavor are also present as integers — see
`test/e2e-framework/components/os/const.go` for the enum values
(`WindowsFamily = 2`, `WindowsServer = 509`, `WindowsClient = 510`).

### RDP

```bash
# Opens RDP and prints IP + password (copies password to clipboard)
dda inv aws.rdp-vm --stack-name <stack-name>
```

Note: `dda inv aws.show-vm` does **not** work for e2e test stacks — it is only
for VMs created with `dda inv aws.create-vm`.

### SSH

The SSH key is the `privateKeyPath` from `~/.test_infra_config.yaml`.
Windows VMs run PowerShell over SSH, so use `;` as the command separator
(`&&` is not valid in PowerShell):

```bash
ssh -i <privateKeyPath> -o StrictHostKeyChecking=no <username>@<ip> "<commands>"
```

## Troubleshooting

### Pulumi lock errors

If you see `error: the stack is currently locked by 1 lock(s)`:

```bash
dda inv new-e2e-tests.clean        # remove local Pulumi locks
dda inv new-e2e-tests.clean -s     # also clean local stack state
dda inv new-e2e-tests.clean --output  # clear local test output
```

If cleanup reports "Cleanup supported for local state only":

```bash
pulumi login --local
```

### AWS account

Local runs must use the `agent-sandbox` AWS account (CI uses `agent-qa`).
The e2e framework automatically uses the `sso-agent-sandbox-account-admin`
profile, so no explicit `aws-vault` invocation is needed.

However, if `AWS_PROFILE` is already set in your environment to a different
profile, it will override the automatic selection and cause auth errors. Unset
it before running:

```bash
unset AWS_PROFILE
```

A common symptom of the wrong account:
`User: arn:aws:sts::... is not authorized to perform: ecr:BatchGetImage`
