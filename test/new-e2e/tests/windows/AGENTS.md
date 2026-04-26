# Windows Agent E2E Tests

Tests for the Windows Agent. Scenarios include MSI installation,
uninstallation, upgrades, service lifecycle, file/registry/permission
correctness, FIPS mode, domain/AD-joined hosts, and Windows-specific
components (NPM, APM, certificates). Shared Windows utilities (services,
registry, ACL, event logs, crash dumps, etc.) live in `common/`.

Tests run on AWS-provisioned Windows VMs (no pre-installed agent, no
fakeintake). The VM is created fresh per test run unless DevMode is active.

## Subdirectories

Each test area has its own `AGENTS.md`. Non-obvious ones:

**`domain-test/`** tests MSI install and upgrade on a domain controller, using
the e2e-framework's `activedirectory` Pulumi component to stand up Active
Directory before the agent is installed. The tests follow the same patterns as
`install-test/` and reuse its `Tester` helper. Domain *client* tests (joining
a non-DC host to the domain) are not yet implemented.

**`components/`** contains Pulumi components that configure small aspects of
the host environment (Windows Defender exclusions, FIPS mode, test-signing,
certificate host). They have no tests of their own. Prefer `common/` utilities
for anything that can be done after provisioning; these components are only
necessary when the host must be configured before Pulumi installs the agent.

## Base suite

`BaseAgentInstallerSuite` (`base_agent_installer_suite.go`) is the base for
all MSI installer tests. It resolves the MSI package from environment
variables at suite startup (stored in `s.AgentPackage`) and provides
`s.InstallAgent(host, opts...)` and `s.NewTestClientForHost(host)`.
Individual test packages embed this (directly or through a further local base)
and add their own `BeforeTest`/`AfterTest` hooks for snapshots, event log
capture, crash dumps, and diagnostics.

## Package resolution

The MSI under test is resolved from environment variables at suite startup.

| Env var | Effect |
|---|---|
| `CURRENT_AGENT_MSI_URL` | Direct URL (local `file://` or HTTPS); skips all other resolution |
| `CURRENT_AGENT_PIPELINE` | Fetches MSI from S3 pipeline artifacts |
| `CURRENT_AGENT_SOURCE_VERSION` | Looks up MSI in `installers_v2.json` |
| `CURRENT_AGENT_ASSERT_VERSION` | Version string for assertions (e.g. `7.66.0-devel`) |
| `CURRENT_AGENT_ASSERT_PACKAGE_VERSION` | URL-safe package version for assertions |

For upgrade tests, the same pattern applies with the `STABLE_AGENT_` prefix.
`GetLastStablePackageFromEnv()` reads `STABLE_AGENT_*` variables.

Resolution priority: direct URL > pipeline ID > version lookup. If none are
set, `GetPackageFromEnv()` falls back to the latest stable MSI.

## Test entry point pattern

Tests use [testify suites](https://pkg.go.dev/github.com/stretchr/testify/suite).
Each test file defines a suite struct that embeds the appropriate base suite,
and a top-level `TestXxx(t *testing.T)` function as the Go test entry point.

Tests in `install-test/` use the package-level `Run` helper (defined in
`install-test/base.go`) which sets the provisioner, stack naming, and
parallelism:

```go
func TestInstall(t *testing.T) {
    installtest.Run(t, &testInstallSuite{})
}

type testInstallSuite struct {
    baseAgentMSISuite
}

func (s *testInstallSuite) TestInstall() {
    vm := s.Env().RemoteHost
    remoteMSIPath := s.installAgentPackage(vm, s.AgentPackage)
    s.Require().NoError(...)
    s.Run("subtest name", func() { ... })
}
```

Suites outside `install-test/` that extend `BaseAgentInstallerSuite` directly
call `e2e.Run` with an explicit provisioner:

```go
func TestFoo(t *testing.T) {
    e2e.Run(t, &testFooSuite{},
        e2e.WithProvisioner(awsHostWindows.ProvisionerNoAgentNoFakeIntake()))
}
```

## Where to add a new test

- **MSI install/uninstall/upgrade/repair** — `tests/windows/install-test/`.
  Uses the MSI installer helpers (`baseAgentMSISuite`, `Tester`).
- **Fleet Automation / OCI packages** (`datadog-installer.exe`,
  `Install-Datadog.ps1`, experiment lifecycle) —
  `tests/installer/windows/`. Uses the installer helpers (`BaseSuite`,
  `DatadogInstallerRunner`).
- **Agent functional or integration tests** that need a pre-installed running
  agent and fakeintake — prefer the Pulumi provisioner to install and
  configure the agent, then use fakeintake for assertions (see
  `tests/windows/service-test/` for an example, or the parent
  `test/new-e2e/` framework docs).
- **Domain controller / AD-joined host** — `tests/windows/domain-test/`.
- **FIPS mode** — `tests/windows/fips-test/`.

Each subdirectory's `AGENTS.md` has a step-by-step "Adding a new test"
recipe with code templates and CI matrix instructions.

## Running tests

See `running_tests.md` in this directory for setup-env, local builds,
and `go test` invocations.

## CI configuration

CI jobs for these tests live in two files under `.gitlab/windows/test/`:

**`e2e/windows.yml`** — general Windows e2e jobs (service tests, certificate,
FIPS compliance, system probe, etc.). Jobs use `EXTRA_PARAMS: --run TestFoo`
to select a specific test function or suite.

**`e2e_install_packages/windows.yml`** — MSI install/upgrade/domain jobs.
Because each test function provisions its own VM, these jobs use a
`parallel: matrix` with one entry per test function so they run concurrently:

```yaml
parallel:
  matrix:
    - E2E_MSI_TEST: TestInstall
    - E2E_MSI_TEST: TestUpgrade
    - E2E_MSI_TEST: TestRepair
```

The matrix value is passed through as `EXTRA_PARAMS: --run "$E2E_MSI_TEST$"`.

## Common utilities

Shared Windows helpers live in `common/`. **Before writing one-off
PowerShell commands in a test, check `common/` and `common/agent/` for
an existing helper** — many operations already have Go wrappers.

Key areas: Windows services, registry, ACL/permissions, local users, event
logs, WER crash dumps, procdump, filesystem snapshots, process management,
network diagnostics, and PowerShell command builder.

Agent-specific helpers (package resolution, MSI install/uninstall, config
root, product codes) live in `common/agent/`.

API reference (for humans):
- [common](https://pkg.go.dev/github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common)
- [common/agent](https://pkg.go.dev/github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent)
