// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
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
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	s.Installer().PromoteExperiment(consts.AgentPackage)
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
	s.Installer().PromoteExperiment(consts.AgentPackage)
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
	s.Installer().PromoteExperiment(consts.AgentPackage)
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
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
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
		s.Installer().StopExperiment(consts.AgentPackage)
		s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().PackageVersion())
	})
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
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
	s.Require().ErrorContains(err, "cannot set new experiment to the same version as the current experiment")

	// Assert
	s.assertDaemonStaysRunning(func() {
		s.Installer().StopExperiment(consts.AgentPackage)
		s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().PackageVersion())
	})
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
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
		s.Installer().StopExperiment(consts.AgentPackage)
		s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().PackageVersion())
	})

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
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
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
	// backend will send stop experiment now
	s.assertDaemonStaysRunning(func() {
		s.Installer().StopExperiment(consts.AgentPackage)
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
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.StableAgentVersion().Version())
		})
	// backend will send stop experiment now
	s.assertDaemonStaysRunning(func() {
		s.Installer().StopExperiment(consts.AgentPackage)
		s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())
	})
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
	s.Installer().PromoteExperiment(consts.AgentPackage)
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
		HasBinary(consts.BinaryPath).
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
		HasBinary(consts.BinaryPath).
		// Don't check the binary signature because it could have been updated since the last stable was built
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})
}

func (s *testAgentUpgradeSuite) setAgentConfig() {
	s.Env().RemoteHost.MkdirAll("C:\\ProgramData\\Datadog")
	s.Env().RemoteHost.WriteFile(consts.ConfigPath, []byte(`
api_key: aaaaaaaaa
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
func TestAgentUpgradesFromGA(t *testing.T) {
	s := &testAgentUpgradeFromGASuite{}
	s.testAgentUpgradeSuite.BaseSuite.CreateStableAgent = s.createStableAgent
	e2e.Run(t, s,
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// createStableAgent provides AgentVersionManager for the 7.65.0 Agent release to the suite
func (s *testAgentUpgradeFromGASuite) createStableAgent() (*installerwindows.AgentVersionManager, error) {
	previousVersion := "7.65.0-rc.10"
	previousVersionPackage := "7.65.0-rc.10-1"

	// Get previous version OCI package
	previousOCI, err := installerwindows.NewPackageConfig(
		installerwindows.WithName(consts.AgentPackage),
		installerwindows.WithVersion(previousVersion),
		installerwindows.WithRegistry("install.datad0g.com"),
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

// TestUpgradeAgentPackage tests that upgrade from GA release (7.65.0) to current works
func (s *testAgentUpgradeFromGASuite) TestUpgradeAgentPackage() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.mustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	s.Installer().PromoteExperiment(consts.AgentPackage)
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
}

// TestUpgradeAgentPackageAfterRollback tests that upgrade from GA release (7.65.0) to current works
// even after an initial upgrade failed.
//
// This is a regression test for WINA-1469, where the Agent account password and
// password from the LSA did not match after rollback.
func (s *testAgentUpgradeFromGASuite) TestUpgradeAgentPackageAfterRollback() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.mustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())

	// stop experiment to trigger rollback
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().PackageVersion())

	// Try upgrade again
	s.mustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	s.Installer().PromoteExperiment(consts.AgentPackage)
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
}

// mustStartExperimentCurrentVersion is like MustStartExperimentCurrentVersion but is specific to the 7.65 daemon
// which must use the start-installer-experiment subcommand to start an experiment for the Agent package.
func (s *testAgentUpgradeFromGASuite) mustStartExperimentCurrentVersion() {
	packageConfig, err := s.SetCatalogWithCustomPackage(
		installerwindows.WithPackage(s.CurrentAgentVersion().OCIPackage()),
	)
	s.Require().NoError(err)

	// 7.65 must use the start-installer-experiment subcommand to start an experiment for the Agent package
	s.Installer().StartInstallerExperiment(consts.AgentPackage, packageConfig.Version)
	// can't check error of start-installer-experiment because the process will be killed by the MSI "files in use"

	// have to wait for experiment to finish installing
	err = s.WaitForInstallerService("Running")

	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		}).
		DirExists(consts.GetExperimentDirFor(consts.AgentPackage))
}
