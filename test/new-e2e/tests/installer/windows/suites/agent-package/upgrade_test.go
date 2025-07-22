// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"

	"github.com/cenkalti/backoff/v4"
)

type testAgentUpgradeSuite struct {
	installerwindows.BaseSuite
}

// TestAgentUpgrades tests the usage of the Datadog installer to upgrade the Datadog Agent package.
func TestAgentUpgrades(t *testing.T) {
	e2e.Run(t, &testAgentUpgradeSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// TestUpgradeMSI tests manual upgrade using the Datadog Agent MSI package.
//
// The expectation is that the MSI becomes the new stable package
func (s *testAgentUpgradeSuite) TestUpgradeMSI() {
	s.setAgentConfig()

	s.installPreviousAgentVersion()
	s.AssertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().PackageVersion())

	s.installCurrentAgentVersion()
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())
}

// TestUpgradeAgentPackage tests that the daemon can upgrade the Agent
// through the experiment (start/promote) workflow.
func (s *testAgentUpgradeSuite) TestUpgradeAgentPackage() {
	flake.Mark(s.T())
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
}

// TestUpgradeAgentPackageWithAltDir tests that an Agent installed with the MSI
// and custom paths maintains those paths when remotely upgraded
func (s *testAgentUpgradeSuite) TestUpgradeAgentPackageWithAltDir() {
	// Arrange
	altConfigRoot := `C:\ddconfig`
	altInstallPath := `C:\ddinstall`
	s.Installer().SetBinaryPath(altInstallPath + `\bin\` + consts.BinaryName)
	s.setAgentConfigWithAltDir(altConfigRoot)
	s.installPreviousAgentVersion(
		installerwindows.WithMSIArg("PROJECTLOCATION="+altInstallPath),
		installerwindows.WithMSIArg("APPLICATIONDATADIRECTORY="+altConfigRoot),
	)

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		NoDirExists(windowsagent.DefaultConfigRoot).
		NoDirExists(windowsagent.DefaultInstallPath).
		DirExists(altConfigRoot).
		DirExists(altInstallPath).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("ConfigRoot", altConfigRoot+`\`).
		WithValueEqual("InstallPath", altInstallPath+`\`)
}

// TestUpgradeAgentPackageAfterRollback tests that upgrade works after an initial upgrade failed.
//
// This is a regression test for WINA-1469, where the Agent account password and
// password from the LSA did not match after rollback to a version before LSA support was added.
func (s *testAgentUpgradeSuite) TestUpgradeAgentPackageAfterRollback() {
	flake.Mark(s.T())
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())

	// stop experiment to trigger rollback
	s.WaitForDaemonToStop(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should stop cleanly")
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 10))
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())

	// Try upgrade again
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
}

// TestRunAgentMSIAfterExperiment tests that the Agent can be upgraded after
// an experiment has been run.
//
// Since the MSI removes the `packages/datadog-agent` directory, we wanted to be sure
// that MSIs Source Resiliency wouldn't have the MSI in the stable dir, which may be
// run during RemoveExistingProducts, locked and unable to be removed.
func (s *testAgentUpgradeSuite) TestRunAgentMSIAfterExperiment() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()
	s.MustStartExperimentPreviousVersion()
	s.AssertSuccessfulAgentStartExperiment(s.StableAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().PackageVersion())

	// Act
	s.installCurrentAgentVersion(
		installerwindows.WithMSILogFile("install-current-version-again.log"),
	)
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())
}

// TestUpgradeAgentPackage tests that the daemon can downgrade the Agent
// through the experiment (start/promote) workflow.
func (s *testAgentUpgradeSuite) TestDowngradeAgentPackage() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	s.MustStartExperimentPreviousVersion()
	s.AssertSuccessfulAgentStartExperiment(s.StableAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().PackageVersion())

	// Assert
}

// TestStopExperiment tests that the daemon can stop the experiment
// and that it reverts to the stable version.
func (s *testAgentUpgradeSuite) TestStopExperiment() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	s.WaitForDaemonToStop(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should stop cleanly")
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 10))
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
}

// TestExperimentForNonExistingPackageFails tests that starting an experiment
// with a non-existing package version fails and can be stopped.
func (s *testAgentUpgradeSuite) TestExperimentForNonExistingPackageFails() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	_, err := s.Installer().StartExperiment(consts.AgentPackage, "unknown-version")
	s.Require().ErrorContains(err, "could not get package")

	// Assert
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().PackageVersion())
	})
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})
}

// TestExperimentCurrentVersionFails tests that starting an experiment
// with the same version as the current one fails and can be stopped.
func (s *testAgentUpgradeSuite) TestExperimentCurrentVersionFails() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	_, err := s.StartExperimentCurrentVersion()
	s.Require().ErrorContains(err, "cannot set new experiment to the same version as stable")

	// Assert
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().PackageVersion())
	})
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})
}

func (s *testAgentUpgradeSuite) TestStopWithoutExperiment() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().PackageVersion())
	})

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})
}

// TestRevertsExperimentWhenServiceDies tests that the watchdog will revert
// to stable version when the service dies.
func (s *testAgentUpgradeSuite) TestRevertsExperimentWhenServiceDies() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	windowscommon.StopService(s.Env().RemoteHost, consts.ServiceName)

	// Assert
	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)
	// original version should now be running
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
	// backend will send stop experiment now
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())
	})
}

// TestRevertsExperimentWhenServiceDies tests that the watchdog will revert
// to stable version when the timeout expires.
func (s *testAgentUpgradeSuite) TestRevertsExperimentWhenTimeout() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()
	// lower timeout to 2 minute
	s.setWatchdogTimeout(2)

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
	err := s.waitForInstallerVersion(s.StableAgentVersion().Version())
	s.Require().NoError(err)
	// wait till the services start
	err = s.WaitForInstallerService("Running")
	s.Require().NoError(err)
	// verify stable version contraints
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
	// backend will send stop experiment now
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())
	})
}

// TestExperimentMSIRollbackMaintainsCustomUserAndAltDir tests that the
// stable version is reinstalled with the custom user and alt dir when an experiment MSI rolls back.
// This is a regression test for WINA-1504, where remove-experiment subcommand used the wrong
// paths and failed to restore the stable version.
func (s *testAgentUpgradeSuite) TestExperimentMSIRollbackMaintainsCustomUserAndAltDir() {
	// Arrange
	altConfigRoot := `C:\ddconfig`
	altInstallPath := `C:\ddinstall`
	agentUser := "customuser"
	s.Installer().SetBinaryPath(altInstallPath + `\bin\` + consts.BinaryName)
	s.setAgentConfigWithAltDir(altConfigRoot)
	s.Require().NotEqual(windowsagent.DefaultAgentUserName, agentUser, "the custom user should be different from the default user")
	s.installPreviousAgentVersion(
		installerwindows.WithOption(installerwindows.WithAgentUser(agentUser)),
		installerwindows.WithMSIArg("PROJECTLOCATION="+altInstallPath),
		installerwindows.WithMSIArg("APPLICATIONDATADIRECTORY="+altConfigRoot),
	)
	s.setExperimentMSIArgs([]string{"WIXFAILWHENDEFERRED=1"})

	// Act
	s.WaitForDaemonToStop(func() {
		_, err := s.StartExperimentCurrentVersion()
		s.Require().NoError(err, "daemon should stop cleanly")
		// This returns while the upgrade is still running, so we need to wait for the service to stop
		// We can't use WaitForInstallerService here because it can be racy with MSI rollback,
		// the service could stop and then restart before we check the status again.
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), 100))

	// wait for upgrade to restart the service
	// this is racy, we'll either catch the new service running briefly before MSI rollback
	// triggers, or we'll catch the previous service running after MSI rollback completes
	// The next set of checks quiesce the race.
	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	// Now that the service is running, we know that the stable version has been removed,
	// so we can wait for the stable version to be placed on disk once again via MSI rollback
	err = s.waitForInstallerVersion(s.StableAgentVersion().Version())
	s.Require().NoError(err)
	// and wait again to ensure the stable service is running
	err = s.WaitForInstallerService("Running")
	s.Require().NoError(err)

	// Assert

	// original version should now be running
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})

	// backend will send stop experiment to the daemon
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())
	})

	identity, err := windowscommon.GetIdentityForUser(s.Env().RemoteHost, agentUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		NoDirExists(windowsagent.DefaultConfigRoot).
		NoDirExists(windowsagent.DefaultInstallPath).
		DirExists(altConfigRoot).
		DirExists(altInstallPath).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", agentUser).
		WithValueEqual("ConfigRoot", altConfigRoot+`\`).
		WithValueEqual("InstallPath", altInstallPath+`\`).
		HasAService("datadogagent").
		WithIdentity(identity)
}

// TestExperimentMSIRollbackMaintainsCustomUserAndAltDir tests that the
// stable version is reinstalled with the custom user and alt dir when an experiment MSI rolls back.
// This is a regression test for WINA-1504, where remove-experiment subcommand used the wrong
// paths and failed to restore the stable version.
func (s *testAgentUpgradeSuite) TestRevertsExperimentWhenServiceDiesMaintainsCustomUserAndAltDir() {
	// Arrange
	altConfigRoot := `C:\ddconfig`
	altInstallPath := `C:\ddinstall`
	agentUser := "customuser"
	s.Installer().SetBinaryPath(altInstallPath + `\bin\` + consts.BinaryName)
	s.setAgentConfigWithAltDir(altConfigRoot)
	s.Require().NotEqual(windowsagent.DefaultAgentUserName, agentUser, "the custom user should be different from the default user")
	s.installPreviousAgentVersion(
		installerwindows.WithOption(installerwindows.WithAgentUser(agentUser)),
		installerwindows.WithMSIArg("PROJECTLOCATION="+altInstallPath),
		installerwindows.WithMSIArg("APPLICATIONDATADIRECTORY="+altConfigRoot),
	)

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	windowscommon.StopService(s.Env().RemoteHost, consts.ServiceName)

	// Assert
	err := s.WaitForInstallerService("Running")
	s.Require().NoError(err)
	// original version should now be running
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})

	// backend will send stop experiment to the daemon
	s.assertDaemonStaysRunning(func() {
		_, err := s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err, "daemon should respond to request")
		s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())
	})

	identity, err := windowscommon.GetIdentityForUser(s.Env().RemoteHost, agentUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		NoDirExists(windowsagent.DefaultConfigRoot).
		NoDirExists(windowsagent.DefaultInstallPath).
		DirExists(altConfigRoot).
		DirExists(altInstallPath).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", agentUser).
		WithValueEqual("ConfigRoot", altConfigRoot+`\`).
		WithValueEqual("InstallPath", altInstallPath+`\`).
		HasAService("datadogagent").
		WithIdentity(identity)
}

// TestUpgradeWithAgentUser tests that the agent user is preserved across remote upgrades.
func (s *testAgentUpgradeSuite) TestUpgradeWithAgentUser() {
	// Arrange
	s.setAgentConfig()
	agentUser := "customuser"
	s.Require().NotEqual(windowsagent.DefaultAgentUserName, agentUser, "the custom user should be different from the default user")
	s.installPreviousAgentVersion(
		installerwindows.WithOption(installerwindows.WithAgentUser(agentUser)),
	)
	// sanity check that the agent is running as the custom user
	identity, err := windowscommon.GetIdentityForUser(s.Env().RemoteHost, agentUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", agentUser).
		HasAService("datadogagent").
		WithIdentity(identity)

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err = s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", agentUser).
		HasAService("datadogagent").
		WithIdentity(identity)
}

func (s *testAgentUpgradeSuite) setWatchdogTimeout(timeout int) {
	// Set HKEY_LOCAL_MACHINE\SOFTWARE\Datadog\Datadog Agent\WatchdogTimeout to timeout
	err := windowscommon.SetRegistryDWORDValue(s.Env().RemoteHost, `HKLM:\SOFTWARE\Datadog\Datadog Agent`, "WatchdogTimeout", timeout)
	s.Require().NoError(err)
}

func (s *testAgentUpgradeSuite) installPreviousAgentVersion(opts ...installerwindows.MsiOption) {
	agentVersion := s.StableAgentVersion().Version()
	options := []installerwindows.MsiOption{
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.StableAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-previous-version.log"),
	}
	options = append(options, opts...)
	s.Require().NoError(s.Installer().Install(options...))

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		// Don't check the binary signature because it could have been updated since the last stable was built
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}

func (s *testAgentUpgradeSuite) installCurrentAgentVersion(opts ...installerwindows.MsiOption) {
	agentVersion := s.CurrentAgentVersion().Version()

	options := []installerwindows.MsiOption{
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.CurrentAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-current-version.log"),
	}
	options = append(options, opts...)
	s.Require().NoError(s.Installer().Install(
		options...,
	))

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasDatadogInstaller().
		// Don't check the binary signature because it could have been updated since the last stable was built
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}

func (s *testAgentUpgradeSuite) setAgentConfig() {
	s.setAgentConfigWithAltDir("C:\\ProgramData\\Datadog")
}

func (s *testAgentUpgradeSuite) setAgentConfigWithAltDir(path string) {
	s.Env().RemoteHost.MkdirAll(path)
	configPath := path + `\datadog.yaml`
	// Ensure the API key is set for telemetry
	apiKey := os.Getenv("DD_API_KEY")
	if apiKey == "" {
		var err error
		apiKey, err = runner.GetProfile().SecretStore().Get(parameters.APIKey)
		if apiKey == "" || err != nil {
			apiKey = "deadbeefdeadbeefdeadbeefdeadbeef"
		}
	}

	s.Env().RemoteHost.WriteFile(configPath, []byte(`
api_key: `+apiKey+`
site: datadoghq.com
remote_updates: true
`))
}

func (s *testAgentUpgradeSuite) assertSuccessfulAgentStopExperiment(version string) {
	// conditions are same as promote, except the stable version should be unchanged.
	// since version is an input we can reuse.
	s.AssertSuccessfulAgentPromoteExperiment(version)
}

func (s *testAgentUpgradeSuite) waitForInstallerVersion(version string) error {
	return s.waitForInstallerVersionWithBackoff(version,
		// usually waiting after MSI runs so we have to wait awhile
		// max wait is 30*30 -> 900 seconds (15 minutes)
		backoff.WithMaxRetries(backoff.NewConstantBackOff(30*time.Second), 30))
}

func (s *testAgentUpgradeSuite) waitForInstallerVersionWithBackoff(version string, b backoff.BackOff) error {
	return backoff.Retry(func() error {
		actual, err := s.Installer().Version()
		if err != nil {
			return err
		}
		if !strings.Contains(actual, version) {
			return fmt.Errorf("expected version %s, got %s", version, actual)
		}
		return nil
	}, b)
}

// assertDaemonStaysRunning asserts that the daemon service PID is the same before and after the function is called.
//
// For example, used to verify that "stop-experiment" does not reinstall stable when it is already installed.
func (s *testAgentUpgradeSuite) assertDaemonStaysRunning(f func()) {
	s.T().Helper()

	originalPID, err := windowscommon.GetServicePID(s.Env().RemoteHost, consts.ServiceName)
	s.Require().NoError(err)
	s.Require().Greater(originalPID, 0)

	f()

	newPID, err := windowscommon.GetServicePID(s.Env().RemoteHost, consts.ServiceName)
	s.Require().NoError(err)
	s.Require().Equal(originalPID, newPID, "daemon should not have been restarted")
}

type testAgentUpgradeFromGASuite struct {
	testAgentUpgradeSuite
}

// TestAgentUpgradesFromGA tests that we can upgrade from GA release (7.65.0) to current
//
// It embeds testAgentUpgradeSuite so it can run any of the upgrade tests.
func TestAgentUpgradesFromGA(t *testing.T) {
	s := &testAgentUpgradeFromGASuite{}
	s.testAgentUpgradeSuite.BaseSuite.CreateStableAgent = s.createStableAgent
	e2e.Run(t, s,
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// BeforeTest wraps the installer in the DatadogInstallerGA type to handle the special cases for 7.65.x
func (s *testAgentUpgradeFromGASuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	// Wrap the installer in the InstallerGA type to handle the special cases for 7.65.x
	s.SetInstaller(&installerwindows.DatadogInstallerGA{
		DatadogInstaller: s.Installer().(*installerwindows.DatadogInstaller),
	})
}

// createStableAgent provides AgentVersionManager for the 7.65.0 Agent release to the suite
func (s *testAgentUpgradeFromGASuite) createStableAgent() (*installerwindows.AgentVersionManager, error) {
	previousVersion := "7.65.0-rc.10"
	previousVersionPackage := "7.65.0-rc.10-1"

	// Get previous version OCI package
	previousOCI, err := installerwindows.NewPackageConfig(
		installerwindows.WithName(consts.AgentPackage),
		installerwindows.WithVersion(previousVersion),
		installerwindows.WithRegistry("install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithDevEnvOverrides("STABLE_AGENT"),
	)
	s.Require().NoError(err, "Failed to lookup OCI package for previous agent version")

	// Get previous version MSI package
	url, err := windowsagent.GetChannelURL("beta")
	s.Require().NoError(err)
	previousMSI, err := windowsagent.NewPackage(
		windowsagent.WithVersion(previousVersionPackage),
		windowsagent.WithURLFromInstallersJSON(url, previousVersionPackage),
		windowsagent.WithDevEnvOverrides("STABLE_AGENT"),
	)
	s.Require().NoError(err, "Failed to lookup MSI for previous agent version")

	// Allow override of version and version package via environment variables
	// if not running in the CI, to reduce risk of accidentally using the wrong version in the CI.
	if os.Getenv("CI") == "" {
		if val := os.Getenv("STABLE_AGENT_VERSION"); val != "" {
			previousVersion = val
		}
		if val := os.Getenv("STABLE_AGENT_VERSION_PACKAGE"); val != "" {
			previousVersionPackage = val
		}
	}

	// Setup previous Agent artifacts
	agent, err := installerwindows.NewAgentVersionManager(
		previousVersion,
		previousVersionPackage,
		previousOCI,
		previousMSI,
	)
	s.Require().NoError(err, "Stable agent version was in an incorrect format")

	return agent, nil
}

// setExperimentMSIArgs stores a list of MSI options for the installer to provide to the MSI when starting an experiment.
func (s *testAgentUpgradeSuite) setExperimentMSIArgs(args []string) {
	err := windowscommon.SetRegistryMultiString(s.Env().RemoteHost, `HKLM:SOFTWARE\Datadog\Datadog Agent`, "StartExperimentMSIArgs", args)
	s.Require().NoError(err)
}
