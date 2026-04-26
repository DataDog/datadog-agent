# Windows Fleet Automation / Installer E2E Tests

Tests for the Datadog Fleet Automation stack on Windows: installation and
upgrade of the Agent and its OCI packages via `datadog-installer.exe` and the
`Install-Datadog.ps1` setup script. Tests cover the happy path (install,
upgrade, uninstall) and edge cases (rollback, stopping experiments, custom
agent user/path, config experiments, persisting extensions). DDOT is
currently the only Agent extension and has its own suite.

Tests run on AWS-provisioned Windows VMs with no pre-installed agent or
fakeintake. After every test, WER crash dumps are collected; on failure,
agent logs, installer logs, and Windows event logs are downloaded.

## Directory structure

```
installer/windows/
├── base_suite.go          # BaseSuite: artifact resolution, WER dumps, per-test setup
├── installer.go           # DatadogInstallerRunner interface + DatadogInstaller impl
├── install_script.go      # DatadogInstallScript / DatadogInstallExe: runs setup scripts
├── agent_package.go       # AgentVersionManager: holds MSI + OCI package pair for a version
├── consts/                # Package names, binary paths, service name, registry keys, OCI registries
├── remote-host-assertions/ # Chainable assertions targeting a remote Windows host
├── suite-assertions/      # Suite-level Require() wrapper (returns *SuiteAssertions)
└── suites/
    ├── agent-package/     # Agent install, upgrade, rollback, config experiments, domain/GMSA, persisting extensions
    ├── apm-inject-package/ # APM auto-injection (IIS + Java)
    ├── apm-library-dotnet-package/ # .NET APM library package
    ├── ddot-package/      # DDOT Agent extension package
    ├── install-exe/       # datadog-installer.exe bootstrap
    ├── install-script/    # Install-Datadog.ps1 setup script and custom agent user
    └── installer-package/ # (deprecated) Legacy installer MSI tests
```

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

Tests in `suites/agent-package/` exercise all three paths, including rollback
scenarios where an experiment is stopped after a failed upgrade.

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
| `CURRENT_AGENT` | Agent built by the current pipeline | `CURRENT_AGENT_VERSION`, `CURRENT_AGENT_VERSION_PACKAGE` |
| `STABLE_AGENT` | Baseline for upgrade tests (default: 7.75.0) | `STABLE_AGENT_VERSION`, `STABLE_AGENT_VERSION_PACKAGE` |

Both MSI and OCI artifacts are resolved per version. `WithDevEnvOverrides(prefix)`
applies env var overrides locally but is a no-op in CI (when `CI` is set).

## Running tests

See `tests/windows/running_tests.md` for environment setup and `go test`
invocations. Also see `doc.go` in this package for installer-specific
quick-start instructions.

## CI

Jobs are in `.gitlab/windows/test/e2e_install_packages/windows.yml`.
The `new-e2e-installer-windows` job runs the full `./tests/installer/windows`
target. Individual package suites (e.g. `new-e2e-windows-ddot-package-a7-x86_64`)
have their own jobs with specific `needs` for the OCI artifacts they require.

## Adding a new test

1. **Decide where the test belongs.** See `tests/windows/AGENTS.md` "Where to
   add a new test" section. Fleet Automation / OCI package tests belong here.

2. **Add to an existing suite or create a new one.**
   - If the test fits an existing suite (e.g. agent upgrades go in
     `suites/agent-package/`, install script scenarios in `suites/install-script/`),
     add a new method to the existing suite struct.
   - If it covers a new OCI package or a distinct feature area, create a new
     suite under `suites/<package-name>/`. The suite struct should embed
     `BaseSuite` and define a `TestXxx` entry point:

   ```go
   func TestMyPackage(t *testing.T) {
       e2e.Run(t, &testMyPackageSuite{},
           e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake()))
   }

   type testMyPackageSuite struct {
       windows.BaseSuite
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
