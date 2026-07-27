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
	e2e.Run(t, &testAgentKeepRightsFleetSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		),
	)
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
	s.installPreviousAgentVersion(WithMSIArg("DDAGENTUSER_KEEP_RIGHTS=1"))

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
// passed via StartExperimentMSIArgs (a test-only injection point for the experiment install's
// args, read by getStartExperimentMSIArgs). The explicit value must win over the stale registry
// value, i.e. SeDenyNetworkLogonRight must be reapplied.
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
