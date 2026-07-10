# Windows Fleet Automation / Installer E2E Tests

Tests for the Datadog Fleet Automation stack on Windows: installation and
upgrade of the Agent and its OCI packages via `datadog-installer.exe` and the
`Install-Datadog.ps1` setup script. Tests cover the happy path (install,
upgrade, uninstall) and edge cases (rollback, stopping experiments, custom
agent user/path, config experiments, persisting extensions). DDOT is
currently the only Agent extension and has its own tests.

Tests run on AWS-provisioned Windows VMs with no pre-installed agent or
fakeintake. After every test, WER crash dumps are collected; on failure,
agent logs, installer logs, and Windows event logs are downloaded.

## What's tested

Tests are flat `*_test.go` files directly under `installer/windows/` (package
`installer`); the old `suites/<package>/` subdirectories were flattened in
#47161 to speed up pre-building. Each `TestXxx` entry point is selected by name
with `-run`. The areas covered:

- **Agent package** — install, upgrade (across versions and from GA), downgrade,
  rollback, config experiments, custom agent user / alternate install dir,
  hostname change, and domain-controller / GMSA scenarios.
- **Install script & exe** — `Install-Datadog.ps1` and `datadog-installer.exe`
  bootstrap/setup, including custom agent user, proxy, and domain-controller hosts.
- **DDOT extension** — install via MSI, install script, and `agent` subcommand;
  MSI upgrade; persistence across upgrades.
- **APM auto-injection** — IIS and Java injection via MSI and install script,
  injector stats, and system-probe config interplay.
- **.NET APM library package** — install via MSI and script, with and without IIS.
- **Installer itself** — `datadog-installer.exe` install/rollback, the experiment
  lifecycle (see below), and OCI dev-env overrides.

Shared code lives alongside the tests: `base_suite.go`, `installer.go`,
`install_script.go`, and `agent_package.go` (described below), plus `consts/`
(package names, paths, service name, registry keys, OCI registries),
`remote-host-assertions/` and `suite-assertions/` (fluent host/suite assertions),
and `resources/` / `fixtures/` (test data). `doc.go` has a setup-env quick start.

## Base suite

`BaseSuite` (in `base_suite.go`) extends `e2e.BaseSuite[environments.WindowsHost]`.
The three key things it sets up:

**Current and stable agent versions** — resolved from environment variables
into `*AgentVersionManager` values, accessible via `s.CurrentAgentVersion()`
and `s.StableAgentVersion()`. Suites that need non-default artifacts can
override resolution by setting `s.CreateCurrentAgent` or `s.CreateStableAgent`
before `SetupSuite` runs.

**Install script** — a `DatadogInstallScript` / `DatadogInstallExe` is
created per test and accessible via `s.InstallScript()`. Suites can replace
the implementation with `s.SetInstallScriptImpl()`.

**Assertions** — `s.Require()` returns `*SuiteAssertions` (from
`suite-assertions/`), which wraps testify's `Require` with fluent host
assertions:

```go
s.Require().Host(s.Env().RemoteHost).HasARunningDatadogAgentService()
s.Require().Host(s.Env().RemoteHost).
    HasDatadogInstaller().Status().
    HasPackage("datadog-agent").
    WithStableVersionMatchPredicate(func(v string) {
        s.Require().Contains(v, s.CurrentAgentVersion().Version())
    })
```

## Experiment model

Fleet Automation upgrades OCI packages through an *experiment* slot alongside
the *stable* slot. The full lifecycle and hook sequence is documented in
`pkg/fleet/installer/packages/README.md`. The short version:

- **`StartExperiment`** — installs the new version into the experiment slot.
- **`PromoteExperiment`** — makes the experiment the new stable; the old
  version is removed.
- **`StopExperiment`** — discards the experiment; the original stable is
  restored.

**Windows difference:** both the stable and experiment package directories
exist on disk simultaneously (as on other platforms), but the Windows Agent
package is an MSI rather than a set of loose files. `StartExperiment` and
`StopExperiment` therefore perform a full MSI uninstall of the current version
followed by a full MSI install of the target version, rather than an in-place
file swap.

The agent upgrade tests (`agent_upgrade_test.go`, `TestAgentUpgrades`) exercise
all three paths, including rollback scenarios where an experiment is stopped
after a failed upgrade.

## Key types

**`DatadogInstallerRunner`** (interface in `installer.go`) — wraps
`datadog-installer.exe` subcommands executed on the remote host:
`InstallPackage`, `StartExperiment`, `Status`, etc. plus `Install`/`Uninstall` for the MSI itself.
Access via `s.Installer()`.

**`DatadogInstallScript`** / **`DatadogInstallExe`** (in `install_script.go`)
— runs `Install-Datadog.ps1` or `datadog-installer.exe setup` on the remote
host. Access via `s.InstallScript()`.

**`AgentVersionManager`** (in `agent_package.go`) — holds the version string,
OCI package config, and MSI package for a given agent version. Used by suites
that need to install the stable version first and then upgrade to the current.

## Package resolution

Two agent versions are resolved at suite startup from environment variables:

| Prefix | Role | Key env vars |
|---|---|---|
| `CURRENT_AGENT` | Agent built by the current pipeline | Assertions: `CURRENT_AGENT_ASSERT_VERSION`, `CURRENT_AGENT_ASSERT_PACKAGE_VERSION`. Resolution: `CURRENT_AGENT_PIPELINE` or `CURRENT_AGENT_SOURCE_VERSION` (falls back to `E2E_PIPELINE_ID`). |
| `STABLE_AGENT` | Baseline for upgrade tests (default: 7.79.2) | Assertions: `STABLE_AGENT_ASSERT_VERSION`, `STABLE_AGENT_ASSERT_PACKAGE_VERSION`. Resolution: `STABLE_AGENT_SOURCE_VERSION` or `STABLE_AGENT_PIPELINE`. |

These are exactly the variables `dda inv new-e2e-tests.setup-env` emits.
`getAgentVersionVars()` reads the `_ASSERT_*` pair (the `_ASSERT_VERSION` is
required; locally the package version defaults to the version), and
`WithArtifactOverrides(prefix)` reads the resolution vars plus per-artifact
overrides (`<prefix>_OCI_URL` / `_OCI_PIPELINE` / `_OCI_VERSION`,
`<prefix>_MSI_*`). Both MSI and OCI artifacts are resolved per version.
`WithDevEnvOverrides(prefix)` applies these overrides locally but is a no-op in
CI (when `CI` is set).

## Running tests

See the `run-windows-e2e` skill references for environment setup and `go test`
invocations: `.claude/skills/run-windows-e2e/references/setup.md` and
`.../running.md` (or invoke the `/run-windows-e2e` skill). Also see `doc.go` in
this package for installer-specific quick-start instructions.

## CI

Jobs are in `.gitlab/windows/test/e2e_install_packages/windows.yml`.
The `new-e2e-installer-windows` job runs the full `./tests/installer/windows`
target. Individual package jobs (e.g. `new-e2e-windows-ddot-package-a7-x86_64`)
have specific `needs` for the OCI artifacts they require.

## Adding a new test

1. **Decide where the test belongs.** See `tests/windows/AGENTS.md` "Where to
   add a new test" section. Fleet Automation / OCI package tests belong here.

2. **Add a `TestXxx` to a new or existing `*_test.go` file** directly under
   `installer/windows/` (package `installer`).
   - If it fits an existing area, add a method to that suite struct (e.g. agent
     upgrades in `agent_upgrade_test.go`, install-script scenarios in
     `install_script_test.go`).
   - For a new package or feature area, add a new `*_test.go` file with a suite
     struct that embeds `BaseSuite` and a `TestXxx` entry point:

   ```go
   func TestMyPackage(t *testing.T) {
       e2e.Run(t, &testMyPackageSuite{},
           e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake()))
   }

   type testMyPackageSuite struct {
       BaseSuite
   }

   func (s *testMyPackageSuite) TestInstallMyPackage() {
       s.InstallScript().Run(s.Env().RemoteHost)
       s.Require().Host(s.Env().RemoteHost).HasARunningDatadogAgentService()
       // ... package-specific assertions ...
   }
   ```

3. **Add the test to the CI matrix.** In
   `.gitlab/windows/test/e2e_install_packages/windows.yml`, add a new entry to
   the `parallel: matrix` for the `new-e2e-installer-windows` job:

   ```yaml
   - EXTRA_PARAMS: --run "TestMyPackage$/TestInstallMyPackage$"
   ```

   If the test requires new OCI artifacts, also add the corresponding
   `deploy_*_oci` job to the `needs` list.

4. **Use existing helpers.** Use `s.Installer()` for `datadog-installer.exe`
   commands, `s.InstallScript()` for setup scripts, and the fluent assertions
   from `remote-host-assertions/` and `suite-assertions/`. Check
   `tests/windows/common/` for Go wrappers before writing one-off
   PowerShell commands.
