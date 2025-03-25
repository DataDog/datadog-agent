// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"
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
	s.AssertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().Version())

	s.installCurrentAgentVersion()
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().GetNumberAndPre())
}

// TestUpgradeAgentPackage tests that the daemon can upgrade the Agent
// through the experiment (start/promote) workflow.
func (s *testAgentUpgradeSuite) TestUpgradeAgentPackage() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().GetNumberAndPre())
	s.Installer().PromoteExperiment(consts.AgentPackage)
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().GetNumberAndPre())

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
	s.AssertSuccessfulAgentStartExperiment(s.StableAgentVersion().Version())
	s.Installer().PromoteExperiment(consts.AgentPackage)
	s.AssertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().Version())

	// Act
	s.installCurrentAgentVersion(
		installerwindows.WithMSILogFile("install-current-version-again.log"),
	)
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().GetNumberAndPre())
}

// TestUpgradeAgentPackage tests that the daemon can downgrade the Agent
// through the experiment (start/promote) workflow.
func (s *testAgentUpgradeSuite) TestDowngradeAgentPackage() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	s.MustStartExperimentPreviousVersion()
	s.AssertSuccessfulAgentStartExperiment(s.StableAgentVersion().Version())
	s.Installer().PromoteExperiment(consts.AgentPackage)
	s.AssertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().Version())

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
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().GetNumberAndPre())
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().Version())

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
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().GetNumberAndPre())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
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
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().GetNumberAndPre())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
		})
}

func (s *testAgentUpgradeSuite) TestStopWithoutExperiment() {
	// Arrange
	s.setAgentConfig()
	s.installCurrentAgentVersion()

	// Act
	s.Installer().StopExperiment(consts.AgentPackage)

	// Assert
	s.assertSuccessfulAgentStopExperiment(s.CurrentAgentVersion().GetNumberAndPre())
	s.Require().Host(s.Env().RemoteHost).
		HasBinary(consts.BinaryPath).
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().GetNumberAndPre())
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
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().GetNumberAndPre())
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
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().Version())
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
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().GetNumberAndPre())

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
	s.Installer().StopExperiment(consts.AgentPackage)
	s.assertSuccessfulAgentStopExperiment(s.StableAgentVersion().Version())
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
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().GetNumberAndPre())
	s.Installer().PromoteExperiment(consts.AgentPackage)
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().GetNumberAndPre())

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
		installerwindows.WithOption(installerwindows.WithURLFromPipeline("58948204")),
		installerwindows.WithMSIDevEnvOverrides("PREVIOUS_AGENT"),
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
	agentVersion := s.CurrentAgentVersion().GetNumberAndPre()

	options := []installerwindows.MsiOption{
		installerwindows.WithMSIDevEnvOverrides("CURRENT_AGENT"),
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
