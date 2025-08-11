// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttests

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/DataDog/test-infra-definitions/components/activedirectory"

	"testing"
)

const (
	TestGMSAUser = "svc-datadog$"
)

type testAgentUpgradeOnDCWithGMSASuite struct {
	testAgentUpgradeSuite
}

// TestAgentUpgradesOnDCWithGMSA tests the usage of the Datadog installer to upgrade the Datadog Agent package on a Domain Controller
// with a Agent gMSA.
func TestAgentUpgradesOnDCWithGMSA(t *testing.T) {
	e2e.Run(t, &testAgentUpgradeOnDCWithGMSASuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(
				winawshost.WithActiveDirectoryOptions(
					activedirectory.WithDomainController(TestDomain, TestPassword),
				),
			),
		),
	)
}

// SetupSuite configures the gMSA account on the Domain Controller.
func (s *testAgentUpgradeOnDCWithGMSASuite) SetupSuite() {
	s.testAgentUpgradeSuite.SetupSuite()

	host := s.Env().RemoteHost

	// Check and configure the KDS root key if not already configured
	s.T().Log("Checking and configuring KDS root key...")
	err := createKDSRootKey(host)
	s.Require().NoError(err, "should check and create KDS root key")

	// Configure the gMSA account
	s.T().Log("Configuring gMSA account...")
	err = createGMSAAccount(host, TestGMSAUser, TestDomain)
	s.Require().NoError(err, "should configure gMSA account")
}

// TestUpgradeMSI tests manual upgrade using the Datadog Agent MSI package with an Agent gMSA.
//
// The expectation is that the MSI becomes the new stable package
func (s *testAgentUpgradeOnDCWithGMSASuite) TestUpgradeMSI() {
	s.setAgentConfig()

	// Install the stable MSI artifact
	s.installPreviousAgentVersion(
		installerwindows.WithMSIArg(fmt.Sprintf("DDAGENTUSER_NAME=%s", TestGMSAUser)),
	)
	s.AssertSuccessfulAgentPromoteExperiment(s.StableAgentVersion().PackageVersion())

	// sanity check: make sure we did indeed install the stable version
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService()

	// Upgrade the Agent using the MSI package
	// User property not set on command line, should be read from the registry.
	s.installCurrentAgentVersion()

	// Assert
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogAgentService().
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("installedUser", TestGMSAUser)
	identity, err := windowscommon.GetIdentityForUser(s.Env().RemoteHost, TestGMSAUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		HasAService("datadogagent").
		WithIdentity(identity)

}

// TestUpgradeAgentPackage tests that the daemon can upgrade the Agent
// through the experiment (start/promote) workflow with an Agent gMSA.
func (s *testAgentUpgradeOnDCWithGMSASuite) TestUpgradeAgentPackage() {
	// Arrange
	s.setAgentConfig()

	// Install the stable MSI artifact
	s.installPreviousAgentVersion(
		installerwindows.WithMSIArg(fmt.Sprintf("DDAGENTUSER_NAME=%s", TestGMSAUser)),
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
		WithValueEqual("installedUser", TestGMSAUser)
	identity, err := windowscommon.GetIdentityForUser(s.Env().RemoteHost, TestGMSAUser)
	s.Require().NoError(err)
	s.Require().Host(s.Env().RemoteHost).
		HasAService("datadogagent").
		WithIdentity(identity)
}

// createKDSRootKey creates a new KDS root key.
// It skips creation if the key already exists.
func createKDSRootKey(host *components.RemoteHost) error {
	cmdCheck := "Get-KdsRootKey"
	output, err := host.Execute(cmdCheck)
	if err == nil && strings.Contains(output, "KeyDistributionService") {
		// KDS root key already exists, skip creation
		return nil
	}

	cmdCreate := "Add-KdsRootKey -EffectiveTime ((get-date).addhours(-10))"
	_, err = host.Execute(cmdCreate)
	if err != nil {
		return fmt.Errorf("failed to create KDS root key: %w", err)
	}
	return nil
}

// createGMSAAccount creates a gMSA account with the specified name and domain on the given host.
// It skips creation if the account already exists.
func createGMSAAccount(host *components.RemoteHost, accountName, domain string) error {
	userWithoutSuffix := strings.TrimSuffix(accountName, "$")

	// Check if the gMSA account already exists
	checkCmd := fmt.Sprintf("Get-ADServiceAccount -Identity %s", userWithoutSuffix)
	_, err := host.Execute(checkCmd)
	if err == nil {
		// Account already exists, skip creation
		return nil
	}

	// Create the gMSA account
	cmd := fmt.Sprintf(
		`New-ADServiceAccount -Name "%s" -DNSHostName "%s.%s" -PrincipalsAllowedToRetrieveManagedPassword 'Domain Controllers','Domain Computers'`,
		userWithoutSuffix, userWithoutSuffix, domain,
	)
	_, err = host.Execute(cmd)
	if err != nil {
		return fmt.Errorf("failed to create gMSA account: %w", err)
	}
	return nil
}
