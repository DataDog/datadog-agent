// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/activedirectory"
	scenwin "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const (
	TestDomain   = "datadogqalab.com"
	TestUser     = "TestUser"
	TestPassword = "Test1234#"
)

type testAgentUpgradeOnDCSuite struct {
	testAgentUpgradeSuite
}

// TestAgentUpgradesOnDC tests the usage of the Datadog installer to upgrade the Datadog Agent package on a Domain Controller.
func TestAgentUpgradesOnDC(t *testing.T) {
	// TODO: https://datadoghq.atlassian.net/browse/WINA-2075.
	flake.Mark(t)
	e2e.Run(t, &testAgentUpgradeOnDCSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(
				winawshost.WithRunOptions(scenwin.WithActiveDirectoryOptions(
					activedirectory.WithDomainController(TestDomain, TestPassword),
					activedirectory.WithDomainUser(TestUser, TestPassword),
				)),
			),
		),
	)
}

// TestUpgradeMSI tests manual upgrade using the Datadog Agent MSI package when a custom user/password is set.
//
// The expectation is that the MSI becomes the new stable package
func (s *testAgentUpgradeOnDCSuite) TestUpgradeMSI() {
	s.setAgentConfig()

	// Install the stable MSI artifact
	s.installPreviousAgentVersion(
		installerwindows.WithMSIArg("DDAGENTUSER_NAME="+TestUser),
		installerwindows.WithMSIArg("DDAGENTUSER_PASSWORD="+TestPassword),
	)
	s.AssertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().PackageVersion())

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService()

	// Upgrade the Agent using the MSI package
	// User and password properties are not set on command line, they
	// should be read from the registry and LSA, respectively.
	s.installCurrentAgentVersion()

	// Assert
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", TestUser)
	identity, err := windowscommon.GetIdentityForUser(s.Env().RemoteHost, TestUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		HasAService("datadogagent").
		WithIdentity(identity)

}

// TestUpgradeAgentPackage tests that the daemon can upgrade the Agent
// through the experiment (start/promote) workflow when a custom user/password is set.
func (s *testAgentUpgradeOnDCSuite) TestUpgradeAgentPackage() {
	// Arrange
	s.setAgentConfig()

	// Install the stable MSI artifact
	s.installPreviousAgentVersion(
		installerwindows.WithMSIArg("DDAGENTUSER_NAME="+TestUser),
		installerwindows.WithMSIArg("DDAGENTUSER_PASSWORD="+TestPassword),
	)
	s.AssertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().PackageVersion())

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService()

	// Act
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// Assert
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", TestUser)
	identity, err := windowscommon.GetIdentityForUser(s.Env().RemoteHost, TestUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		HasAService("datadogagent").
		WithIdentity(identity)
	windowsagent.TestAgentHasNoWorldWritablePaths(s.T(), s.Env().RemoteHost)
}

type testUpgradeWithMissingPasswordSuite struct {
	testAgentUpgradeSuite
}

// TestUpgradeWithMissingPassword tests that Agent is still installed and running
// after an upgrade fails because the Agent password is missing.
//
// Test runs on a domain controller because that scenario requires the Agent password.
//
// Test procedure:
//  1. Installs version without fleet support (7.64.3)
//  2. Upgrade to version with fleet support (7.66.1) with the MSI, without reproviding password option
//  3. Upgrade to current version, expect it to fail
//  4. Assert missing password error is reported and Agent+Installer are still running
func TestUpgradeWithMissingPassword(t *testing.T) {
	// TODO(WINA-2078): domain controller promotion is flaky
	flake.Mark(t)
	s := &testUpgradeWithMissingPasswordSuite{}
	s.testAgentUpgradeSuite.BaseSuite.CreateStableAgent = s.createStableAgent
	e2e.Run(t, s,
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(
				winawshost.WithRunOptions(scenwin.WithActiveDirectoryOptions(
					activedirectory.WithDomainController(TestDomain, TestPassword),
					activedirectory.WithDomainUser(TestUser, TestPassword),
				)),
			),
		),
	)
}

func (s *testUpgradeWithMissingPasswordSuite) TestUpgradeWithMissingPassword() {
	// Arrange
	s.setAgentConfig()

	// Install old Agent version (must be < 7.66.0, which is the first version with LSA support)
	// This is initial install so we must pass the username and passsord
	agentVersion := s.StableAgentVersion().Version()
	options := []installerwindows.MsiOption{
		installerwindows.WithOption(installerwindows.WithInstallerURL(s.StableAgentVersion().MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-previous-version.log"),
		installerwindows.WithMSIArg("DDAGENTUSER_NAME=" + TestUser),
		installerwindows.WithMSIArg("DDAGENTUSER_PASSWORD=" + TestPassword),
	}
	s.Require().NoError(s.Installer().Install(options...))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, agentVersion)
		})

	// Upgrade to 7.66.0 using the MSI, without providing the password option
	// Can be any version with LSA support, main point is that we don't provide
	// the password option again, so it can't be stored in the LSA.
	secondVersion, err := s.createSecondStableAgent()
	s.Require().NoError(err)
	s.T().Logf("second agent version: %s", secondVersion)
	options = []installerwindows.MsiOption{
		installerwindows.WithOption(installerwindows.WithInstallerURL(secondVersion.MSIPackage().URL)),
		installerwindows.WithMSILogFile("install-second-version.log"),
	}
	s.Require().NoError(s.Installer().Install(options...))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, secondVersion.Version())
		})
	// This version has fleet support so we should sanity check the package state
	s.AssertSuccessfulAgentPromoteExperiment(secondVersion.PackageVersion())

	// Act
	// now attempt upgrade to current version, expect it to fail
	// Expect the daemon to stay running, because this error should be caught
	// before the background worker stops any services.
	// This should allow the daemon to return the error to the user, too.
	s.assertDaemonStaysRunning(func() {
		_, err := s.StartExperimentCurrentVersion()
		s.Require().ErrorContains(err, "the Agent user password is not available. The password is required for domain accounts. Please reinstall the Agent with the password provided")
		// I'm not sure if backend sends stop-experiment here, but if it does
		// we want to make sure we assert it's a no-op.
		_, err = s.Installer().StopExperiment(consts.AgentPackage)
		s.Require().NoError(err)
		s.assertSuccessfulAgentStopExperiment(secondVersion.PackageVersion())
	})

	// Assert
	// TODO: If the local API is updated so it updates the task state then we
	// should assert that it contains the above error, too.
}

func (s *testUpgradeWithMissingPasswordSuite) createSecondStableAgent() (*installerwindows.AgentVersionManager, error) {
	return s.createStableAgentWithVersion("7.66.1", "7.66.1-1", "SECOND_STABLE_AGENT")
}

func (s *testUpgradeWithMissingPasswordSuite) createStableAgent() (*installerwindows.AgentVersionManager, error) {
	return s.createStableAgentWithVersion("7.64.3", "7.64.3-1", "STABLE_AGENT")
}

func (s *testUpgradeWithMissingPasswordSuite) createStableAgentWithVersion(version string, versionPackage string, devEnvOverride string) (*installerwindows.AgentVersionManager, error) {

	// Get previous version MSI package
	url, err := windowsagent.GetChannelURL("stable")
	s.Require().NoError(err)
	previousMSI, err := windowsagent.NewPackage(
		windowsagent.WithVersion(versionPackage),
		windowsagent.WithURLFromInstallersJSON(url, versionPackage),
		windowsagent.WithDevEnvOverrides(devEnvOverride),
	)
	s.Require().NoError(err, "Failed to lookup MSI for previous agent version")

	// Allow override of version and version package via environment variables
	// if not running in the CI, to reduce risk of accidentally using the wrong version in the CI.
	if os.Getenv("CI") == "" {
		if val := os.Getenv(devEnvOverride + "_VERSION"); val != "" {
			version = val
		}
		if val := os.Getenv(devEnvOverride + "_VERSION_PACKAGE"); val != "" {
			versionPackage = val
		}
	}

	// Setup previous Agent artifacts
	agent, err := installerwindows.NewAgentVersionManager(
		version,
		versionPackage,
		installerwindows.TestPackageConfig{},
		previousMSI,
	)
	s.Require().NoError(err, "Stable agent version was in an incorrect format")

	return agent, nil
}

type testAgentUpgradesAfterDCPromotionSuite struct {
	testAgentUpgradeSuite
}

// TestAgentUpgradesAfterDCPromotion tests that the agent can be upgraded after a domain controller promotion.
//
// This converts all accounts from local accounts to domain accounts, i.e. hostname\username -> domain\username,
// so we test that our username code handles this change appropriately.
func TestAgentUpgradesAfterDCPromotion(t *testing.T) {
	// TODO(WINA-2055): domain controller promotion is flaky
	flake.Mark(t)
	e2e.Run(t, &testAgentUpgradesAfterDCPromotionSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

func (s *testAgentUpgradesAfterDCPromotionSuite) TestUpgradeAfterDCPromotion() {
	// Arrange
	s.setAgentConfig()
	s.installPreviousAgentVersion()

	// Act
	// Install AD and promote host to domain controller
	s.UpdateEnv(
		winawshost.ProvisionerNoAgentNoFakeIntake(
			winawshost.WithRunOptions(scenwin.WithActiveDirectoryOptions(
				activedirectory.WithDomainController(TestDomain, TestPassword),
				activedirectory.WithDomainUser(TestUser, TestPassword),
			)),
		),
	)

	// start experiment
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err := s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())
}
