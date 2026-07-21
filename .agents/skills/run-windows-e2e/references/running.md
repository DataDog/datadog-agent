# Artifacts & running — Windows E2E Tests

How to resolve which MSI/OCI package the tests use, and how to invoke
`go test`. See [setup.md](setup.md) for one-time environment setup.

## Setting up artifact environment variables

Windows tests need to know which MSI and/or OCI package to use. The `setup-env`
helper configures the required environment variables. Use `--fmt json` when
driving it programmatically (parse the `"VAR": "value"` map and prepend the
pairs inline to `go test`); use `--fmt powershell | Invoke-Expression` or
`eval "$(...)"` for an interactive shell.

### From a pipeline (most common)

Auto-detects the most recent successful pipeline on the current branch. Add
`--branch main` to target `main` instead — useful when the current branch is a
local feature branch with no pipelines. Add `--pipeline-id <id>` to pin a
specific pipeline.

```bash
# Programmatic (skill default): parse the "VAR": "value" map, prepend inline to `go test`
dda inv new-e2e-tests.setup-env --build pipeline --fmt json [--branch <branch>] [--pipeline-id <id>]

# Interactive shell — PowerShell
dda inv new-e2e-tests.setup-env --build pipeline --fmt powershell | Invoke-Expression

# Interactive shell — Bash/WSL
eval "$(dda inv new-e2e-tests.setup-env --build pipeline)"
```

The task auto-resolves a GitLab token via `ddtool` if configured. If that fails
it asks for `GITLAB_TOKEN`; retry with `GITLAB_TOKEN=<token> dda inv new-e2e-tests.setup-env ...`.

### From a local build

Build the MSI first (add `msi.package-oci` only for installer/OCI tests):

```bash
dda inv msi.build          # -> omnibus/pkg/datadog-agent-<version>-x86_64.msi
dda inv msi.package-oci     # -> omnibus/pkg/datadog-agent-<version>-windows-amd64.oci.tar
dda inv new-e2e-tests.setup-env --build local --fmt json
```

### Upgrade tests (stable / previous version)

Upgrade tests need a second package resolved under the `STABLE_AGENT_` prefix.
Run `setup-env` again with `--prefix STABLE_AGENT --build <mode>` (same modes:
`pipeline`, `local`, `release`) plus any relevant `--version` / `--pipeline-id`
flags, and merge those vars in, overriding the first run's defaults.

The variables themselves are documented per suite — see
`test/new-e2e/tests/windows/AGENTS.md` ("Package resolution") and
`test/new-e2e/tests/installer/windows/AGENTS.md` ("Package resolution").

## Running a specific suite

```bash
go test -v -timeout 30m -tags test <package-path> -run <TestFunction>$
```

Two rules that are easy to get wrong:

- **Anchor the `-run` regex with `$`**, at both the suite and subtest level. The
  flag is a Go regex; without the anchor `TestInstall` also matches
  `TestInstallOpts`/`TestInstallFail`, and `TestUpgradeAgentPackage` also
  matches `TestUpgradeAgentPackageOCIBootstrap`.
- **Use the exact package path, no trailing `/...`.** With `/...` Go buffers all
  output per-package and prints it only when the package finishes, making the
  test look silent. Without it, output streams live.

Examples:

```bash
# MSI install test
go test -v -timeout 30m -tags test ./test/new-e2e/tests/windows/install-test -run TestInstall$

# Service lifecycle test
go test -v -timeout 30m -tags test ./test/new-e2e/tests/windows/service-test -run TestServiceBehaviorPowerShell$

# Fleet Automation — agent package, specific subtest
go test -v -timeout 30m -tags test ./test/new-e2e/tests/installer/windows -run "TestAgentUpgrades$/TestUpgradeAgentPackage$"
```

See each suite's `AGENTS.md` for the full list of test functions and the CI job
names that map to them.

AWS SSO authentication may open a browser window when the test starts; the test
pauses until login completes. This is expected. If `AWS_PROFILE` is set to a
non-sandbox profile it causes auth errors — see [troubleshooting.md](troubleshooting.md).

## Clean state between runs

Most tests install the agent as part of the test and expect a clean starting
state, so run them one at a time. In dev mode (reused VM), clean up between runs
so state does not leak:

- **MSI install tests** (`install-test/`, `domain-test/`, `fips-test/`):
  uninstall via Add/Remove Programs or `MsiExec.exe /x {product-code}` on the VM.
- **Installer / Fleet Automation tests** (`installer/windows/`): run
  `datadog-installer.exe purge` on the VM.
