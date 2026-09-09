// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

const keepRightsAgentUser = windowsagent.DefaultAgentUserName

type testAgentKeepRightsFleetSuite struct {
	testAgentUpgradeSuite
}

// TestAgentKeepRightsFleetUpgrade tests DDAGENTUSER_KEEP_RIGHTS through the real Fleet
// Automation upgrade path (StartExperiment/PromoteExperiment). Regression test for #53958
// and the follow-up #54125.
func TestAgentKeepRightsFleetUpgrade(t *testing.T) {
	s := &testAgentKeepRightsFleetSuite{}
	s.testAgentUpgradeSuite.BaseSuite.CreateStableAgent = s.createStableAgentWithKeepRights
	e2e.Run(t, s,
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
}

// createStableAgentWithKeepRights pins the suite's "previous"/"stable" version to 7.81.0, the
// first release supporting DDAGENTUSER_KEEP_RIGHTS. The default "last stable" pointer used
// elsewhere in this suite can resolve to an older release that doesn't understand the property
// at all, which would silently no-op it.
func (s *testAgentKeepRightsFleetSuite) createStableAgentWithKeepRights() (*AgentVersionManager, error) {
	previousVersion := "7.81.0"
	previousVersionPackage := "7.81.0-1"

	previousOCI, err := NewPackageConfig(
		WithName(consts.AgentPackage),
		WithVersion(previousVersion),
		WithRegistry(consts.BetaS3OCIRegistry),
		WithDevEnvOverrides("STABLE_AGENT"),
	)
	s.Require().NoError(err, "Failed to lookup OCI package for previous agent version")

	previousMSI, err := windowsagent.NewPackage(
		windowsagent.WithVersion(previousVersionPackage),
		windowsagent.WithDevEnvOverrides("STABLE_AGENT"),
	)
	s.Require().NoError(err, "Failed to lookup MSI for previous agent version")

	agent, err := NewAgentVersionManager(previousVersion, previousVersionPackage, previousOCI, previousMSI)
	s.Require().NoError(err, "Stable agent version was in an incorrect format")

	return agent, nil
}

// TestKeepRightsSurvivesFleetUpgrade: install, remove SeDenyNetworkLogonRight (simulating an
// operator customization), reinstall with DDAGENTUSER_KEEP_RIGHTS=1, then fleet-upgrade
// without passing the flag again. The customization and SeServiceLogonRight must survive.
func (s *testAgentKeepRightsFleetSuite) TestKeepRightsSurvivesFleetUpgrade() {
	vm := s.Env().RemoteHost
	s.setAgentConfig()

	// Default install - baseline rights are applied.
	s.installPreviousAgentVersion()

	rights, err := windowscommon.GetUserRightsForUser(vm, keepRightsAgentUser)
	s.Require().NoError(err, "should read service account rights after default install")
	s.Require().Contains(rights, "SeDenyNetworkLogonRight",
		"baseline install must grant SeDenyNetworkLogonRight before the opt-out scenario")

	// Simulate an operator customization.
	err = windowscommon.RemoveUserFromRight(vm, keepRightsAgentUser, "SeDenyNetworkLogonRight")
	s.Require().NoError(err, "should remove %s from SeDenyNetworkLogonRight", keepRightsAgentUser)

	// Reinstall with DDAGENTUSER_KEEP_RIGHTS=1 to persist the opt-out to the registry.
	// installPreviousAgentVersion() defaults to a fixed log filename, so override it here
	// to avoid colliding with the baseline install's log above.
	s.installPreviousAgentVersion(
		WithMSIArg("DDAGENTUSER_KEEP_RIGHTS=1"),
		WithMSILogFile("install-previous-version-keep-rights.log"),
	)

	s.Require().Host(vm).
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("keepRights", "1")
	rights, err = windowscommon.GetUserRightsForUser(vm, keepRightsAgentUser)
	s.Require().NoError(err, "should read service account rights after keep-rights reinstall")
	s.Require().NotContains(rights, "SeDenyNetworkLogonRight",
		"reinstalling with DDAGENTUSER_KEEP_RIGHTS=1 must preserve the customization")

	// Fleet-upgrade without passing DDAGENTUSER_KEEP_RIGHTS again.
	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err = s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// The customization must survive the fleet upgrade.
	s.Require().Host(vm).
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("keepRights", "1")
	rights, err = windowscommon.GetUserRightsForUser(vm, keepRightsAgentUser)
	s.Require().NoError(err, "should read service account rights after fleet upgrade")
	s.Assert().NotContains(rights, "SeDenyNetworkLogonRight",
		"fleet upgrade must not silently drop the DDAGENTUSER_KEEP_RIGHTS opt-out")
	s.Assert().Contains(rights, "SeServiceLogonRight",
		"SeServiceLogonRight must always be granted, even with DDAGENTUSER_KEEP_RIGHTS=1")
}

// TestKeepRightsExplicitArgWinsOnFleetUpgrade: regression test for #54125. Install with
// DDAGENTUSER_KEEP_RIGHTS=1, then fleet-upgrade with an explicit DDAGENTUSER_KEEP_RIGHTS=0
// via StartExperimentMSIArgs (test-only injection point for the experiment install's args).
// The explicit value must win: SeDenyNetworkLogonRight must be reapplied.
func (s *testAgentKeepRightsFleetSuite) TestKeepRightsExplicitArgWinsOnFleetUpgrade() {
	vm := s.Env().RemoteHost
	s.setAgentConfig()

	// Install with the opt-out set from the start.
	s.installPreviousAgentVersion(WithMSIArg("DDAGENTUSER_KEEP_RIGHTS=1"))

	s.Require().Host(vm).
		HasRegistryKey(consts.RegistryKeyPath).
		WithValueEqual("keepRights", "1")
	rights, err := windowscommon.GetUserRightsForUser(vm, keepRightsAgentUser)
	s.Require().NoError(err, "should read service account rights after keep-rights install")
	s.Require().NotContains(rights, "SeDenyNetworkLogonRight",
		"install with DDAGENTUSER_KEEP_RIGHTS=1 must not grant SeDenyNetworkLogonRight")

	// Override the opt-out for the upcoming experiment install only.
	err = windowscommon.SetRegistryMultiString(vm, consts.RegistryKeyPath, "StartExperimentMSIArgs",
		[]string{"DDAGENTUSER_KEEP_RIGHTS=0"})
	s.Require().NoError(err, "should set StartExperimentMSIArgs")

	s.MustStartExperimentCurrentVersion()
	s.AssertSuccessfulAgentStartExperiment(s.CurrentAgentVersion().PackageVersion())
	_, err = s.Installer().PromoteExperiment(consts.AgentPackage)
	s.Require().NoError(err, "daemon should respond to request")
	s.AssertSuccessfulAgentPromoteExperiment(s.CurrentAgentVersion().PackageVersion())

	// The explicit arg must win over the stale registry value.
	rights, err = windowscommon.GetUserRightsForUser(vm, keepRightsAgentUser)
	s.Require().NoError(err, "should read service account rights after fleet upgrade")
	s.Assert().Contains(rights, "SeDenyNetworkLogonRight",
		"explicit DDAGENTUSER_KEEP_RIGHTS=0 install arg must win over the stale registry value")
}
