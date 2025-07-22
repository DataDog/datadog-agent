// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/test-infra-definitions/components/activedirectory"

	"testing"
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
	e2e.Run(t, &testAgentUpgradeOnDCSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(
				winawshost.WithActiveDirectoryOptions(
					activedirectory.WithDomainController(TestDomain, TestPassword),
					activedirectory.WithDomainUser(TestUser, TestPassword),
				),
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
		installerwindows.WithMSIArg(fmt.Sprintf("DDAGENTUSER_NAME=%s", TestUser)),
		installerwindows.WithMSIArg(fmt.Sprintf("DDAGENTUSER_PASSWORD=%s", TestPassword)),
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
		installerwindows.WithMSIArg(fmt.Sprintf("DDAGENTUSER_NAME=%s", TestUser)),
		installerwindows.WithMSIArg(fmt.Sprintf("DDAGENTUSER_PASSWORD=%s", TestPassword)),
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
}
