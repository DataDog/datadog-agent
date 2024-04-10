// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installtest

import (
	"fmt"
	"path/filepath"

	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"
)

func TestUpgrade(t *testing.T) {
	s := &testUpgradeSuite{}
	run(t, s)
}

type testUpgradeSuite struct {
	baseAgentMSISuite
}

func (s *testUpgradeSuite) TestUpgrade() {
	vm := s.Env().RemoteHost

	// install previous version
	_ = s.installLastStable(vm)

	// simulate upgrading from a version that didn't have the runtime-security.d directory
	// to ensure upgrade places new config files.
	configRoot, err := windowsAgent.GetConfigRootFromRegistry(vm)
	s.Require().NoError(err)
	err = vm.RemoveAll(filepath.Join(configRoot, "runtime-security.d"))
	s.Require().NoError(err)

	// upgrade to the new version
	if !s.Run(fmt.Sprintf("upgrade to %s", s.AgentPackage.AgentVersion()), func() {
		_, err := s.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithInstallLogFile(filepath.Join(s.OutputDir, "upgrade.log")),
		)
		s.Require().NoError(err, "should upgrade to agent %s", s.AgentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	// run tests
	t := s.newTester(vm)
	if !t.TestInstallExpectations(s.T()) {
		s.T().FailNow()
	}

	s.uninstallAgentAndRunUninstallTests(t)
}

func TestUpgradeRollback(t *testing.T) {
	s := &testUpgradeRollbackSuite{}
	run(t, s)
}

type testUpgradeRollbackSuite struct {
	baseAgentMSISuite
}

// TC-INS-002
func (s *testUpgradeRollbackSuite) TestUpgradeRollback() {
	vm := s.Env().RemoteHost

	// install previous version
	previousTester := s.installLastStable(vm)

	// upgrade to the new version, but intentionally fail
	if !s.Run(fmt.Sprintf("upgrade to %s with rollback", s.AgentPackage.AgentVersion()), func() {
		_, err := windowsAgent.InstallAgent(vm,
			windowsAgent.WithPackage(s.AgentPackage),
			windowsAgent.WithWixFailWhenDeferred(),
			windowsAgent.WithInstallLogFile(filepath.Join(s.OutputDir, "upgrade.log")),
		)
		s.Require().Error(err, "should fail to install agent %s", s.AgentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	// TODO: we shouldn't have to start the agent manually after rollback
	//       but the kitchen tests did too.
	err := windowsCommon.StartService(vm, "DatadogAgent")
	s.Require().NoError(err, "agent service should start after rollback")

	// the previous version should be functional
	if !previousTester.TestInstallExpectations(s.T()) {
		s.T().FailNow()
	}

	s.uninstallAgentAndRunUninstallTests(previousTester)
}
