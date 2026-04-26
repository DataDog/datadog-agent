# Windows Agent MSI Install Tests

Tests for the Windows Agent MSI installer. Covers install, uninstall,
repair, upgrade (including from v5/v6), custom install paths, agent user
configuration, sub-service options, NPM, autologger, and persisting
integrations. After every test, WER crash dumps are collected; if a test
fails, agent logs and Windows event logs are also downloaded.

## What is tested

The `Tester` helper (`installtester.go`) validates the expected post-install
and post-uninstall state. Checks include:

- File and directory layout (install path, config root)
- ACLs and permissions on installed files and directories
- System paths are not modified by install or uninstall
- Windows service configuration (names, start type, account)
- Agent user creation, SID, and group membership
- Registry keys written by the installer
- Python version and that Python check commands work
- Code signing on agent binaries

## Files

```
install-test/
├── base.go                        # baseAgentMSISuite: BeforeTest/AfterTest hooks, install helpers
├── installtester.go               # Tester: post-install expectation helpers
├── assert.go                      # Standalone assertion functions (user, version, permissions)
├── system_file_tester.go          # Validates system files are not modified by install/uninstall
├── install_test.go                # TestInstall, TestRepair, TestInstallOpts, TestInstallFail, ...
├── upgrade_test.go                # TestUpgrade, TestUpgradeRollback, TestUpgradeChangeUser, ...
├── agent_user_test.go             # TestAgentUser (user-only, dotslash, hostname, LocalSystem, SYSTEM)
├── npm_test.go                    # TestNPM* (install with NPM, upgrades)
├── install_subservices_test.go    # TestSubServicesOpts (all/no subservices)
├── persisting_integrations_test.go # TestPersistingIntegrations*
├── autologger_test.go             # TestInstallWithAutologger*
└── service-test/                  # Sub-package: expected service state definitions used by Tester
                                   #   (not a runnable test package — only provides data for assertions)
```

## Base suite

`baseAgentMSISuite` (defined in `base.go`) embeds
`windows.BaseAgentInstallerSuite[environments.WindowsHost]` and adds
`BeforeTest`/`AfterTest` hooks, the `installAgentPackage()` helper (which
wraps MSI install with xperf tracing and procdump), and cleanup utilities.

## Test entry point

All tests use `installtest.Run` (defined in `base.go`) rather than calling
`e2e.Run` directly. It sets the provisioner, stack naming, and parallelism.
Suite structs embed `baseAgentMSISuite`; use `s.newTester(vm)` to get a
`Tester` and call `t.TestInstallExpectations` / `t.TestUninstallExpectations`
to validate the full post-install/post-uninstall state.

## Running tests

See `../running_tests.md` for environment setup and `go test` invocations.

## CI

Jobs for this package are in
`.gitlab/windows/test/e2e_install_packages/windows.yml`. Each test function
runs as its own parallel job via the `E2E_MSI_TEST` matrix (see parent
`AGENTS.md` for details).

## Adding a new test

1. **Decide where the test belongs.** See the parent `AGENTS.md` "Where to
   add a new test" section. MSI install/uninstall/upgrade tests belong here.

2. **Add to an existing suite or create a new one.**
   - If the test fits an existing test area (e.g. upgrades go in
     `upgrade_test.go`, agent user scenarios in `agent_user_test.go`), add a
     new method to the existing suite struct.
   - If it covers a new area, create a new `*_test.go` file with a new suite
     struct that embeds `baseAgentMSISuite` and a top-level `TestXxx` entry
     point using `installtest.Run`:

   ```go
   func TestMyFeature(t *testing.T) {
       installtest.Run(t, &testMyFeatureSuite{})
   }

   type testMyFeatureSuite struct {
       baseAgentMSISuite
   }

   func (s *testMyFeatureSuite) TestMyFeature() {
       vm := s.Env().RemoteHost
       s.installAgentPackage(vm, s.AgentPackage)
       t := s.newTester(vm)
       t.TestInstallExpectations(s.T())
       // ... additional assertions ...
   }
   ```

3. **Add the test to the CI matrix.** In
   `.gitlab/windows/test/e2e_install_packages/windows.yml`, add a new entry to
   the `parallel: matrix` for the appropriate job
   (`new-e2e-windows-agent-msi-windows-server-a7-x86_64`):

   ```yaml
   - E2E_MSI_TEST: TestMyFeature
   ```

4. **Use existing helpers.** Check `common/` and `common/agent/` for existing
   Go wrappers (services, registry, ACL, event logs) before writing one-off
   PowerShell commands. Use `s.newTester(vm)` for post-install/uninstall
   validation.
