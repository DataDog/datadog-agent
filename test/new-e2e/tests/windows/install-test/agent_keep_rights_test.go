// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"path/filepath"
	"testing"

	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// TestAgentUserKeepRights verifies the DDAGENTUSER_KEEP_RIGHTS opt-out flow:
//
//  1. Default install grants the service account the four expected rights
//     (SeServiceLogonRight + the three SeDeny*LogonRight assignments).
//  2. After an operator removes the account from SeDenyNetworkLogonRight to
//     allow the Agent to reach network resources, reinstalling with
//     DDAGENTUSER_KEEP_RIGHTS=1 must preserve that customization while still
//     ensuring SeServiceLogonRight is present so the service can start.
func TestAgentUserKeepRights(t *testing.T) {
	s := &agentKeepRightsSuite{}
	Run(t, s)
}

type agentKeepRightsSuite struct {
	baseAgentMSISuite
}

func (s *agentKeepRightsSuite) TestKeepRightsPreservesCustomDeny() {
	vm := s.Env().RemoteHost
	const username = "ddagentuser"

	// 1. Default install — installer grants the standard rights baseline.
	_ = s.installAgentPackage(vm, s.AgentPackage)

	t := s.newTester(vm)
	if !t.TestInstallExpectations(s.T()) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}

	// Sanity check: service account starts with all four expected rights.
	rights, err := windowsCommon.GetUserRightsForUser(vm, username)
	s.Require().NoError(err, "should read service account rights after default install")
	s.Require().Contains(rights, "SeDenyNetworkLogonRight",
		"baseline install must grant SeDenyNetworkLogonRight before opt-out scenario")
	s.Require().Contains(rights, "SeServiceLogonRight",
		"baseline install must grant SeServiceLogonRight")

	// 2. Simulate operator customization: remove the account from SeDenyNetworkLogonRight.
	err = windowsCommon.RemoveUserFromRight(vm, username, "SeDenyNetworkLogonRight")
	s.Require().NoError(err, "should remove %s from SeDenyNetworkLogonRight", username)

	// Confirm the customization stuck before re-applying the MSI.
	rights, err = windowsCommon.GetUserRightsForUser(vm, username)
	s.Require().NoError(err, "should re-read service account rights after customization")
	s.Require().NotContains(rights, "SeDenyNetworkLogonRight",
		"customization step must drop SeDenyNetworkLogonRight from the service account")

	// 3. Reinstall (same MSI) with DDAGENTUSER_KEEP_RIGHTS=1. The installer must
	// skip re-applying the SeDeny* rights so the operator customization survives.
	if !s.Run("reinstall with DDAGENTUSER_KEEP_RIGHTS=1", func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithKeepUserRights("1"),
			windowsAgent.WithInstallLogFile(filepath.Join(s.SessionOutputDir(), "keep-rights-reinstall.log")),
		)
		s.Require().NoError(err, "should reinstall agent with DDAGENTUSER_KEEP_RIGHTS=1")
	}) {
		s.Require().FailNow("stopping test after a required assertion or subtest failed")
	}

	// 4. The customization must persist; SeServiceLogonRight must still be granted.
	rights, err = windowsCommon.GetUserRightsForUser(vm, username)
	s.Require().NoError(err, "should read service account rights after keep-rights reinstall")
	s.Assert().NotContains(rights, "SeDenyNetworkLogonRight",
		"DDAGENTUSER_KEEP_RIGHTS=1 must not re-add the service account to SeDenyNetworkLogonRight")
	s.Assert().Contains(rights, "SeServiceLogonRight",
		"SeServiceLogonRight must always be granted, even with DDAGENTUSER_KEEP_RIGHTS=1")

	s.uninstallAgentAndRunUninstallTests(t)
}
