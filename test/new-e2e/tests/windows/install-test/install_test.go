// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package installtest contains e2e tests for the Windows agent installer
package installtest

import (
	"fmt"
	"path/filepath"

	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"

	"testing"
)

type installTestSuite struct {
	baseAgentMSISuite
}

func TestInstall(t *testing.T) {
	s := &installTestSuite{}
	run(t, s)
}

func (s *installTestSuite) TestInstall() {
	vm := s.Env().RemoteHost
	s.prepareHost()

	t := s.installAgent(vm, nil)

	if !t.TestExpectations(s.T()) {
		s.T().FailNow()
	}

	t.TestUninstall(s.T(), filepath.Join(s.OutputDir, "uninstall.log"))
}

type upgradeTestSuite struct {
	baseAgentMSISuite
}

func TestUpgrade(t *testing.T) {
	s := &upgradeTestSuite{}
	run(t, s)
}

func (s *upgradeTestSuite) TestUpgrade() {
	vm := s.Env().RemoteHost
	s.prepareHost()

	_ = s.installLastStable(vm, nil)

	t, err := NewTester(s.T(), vm,
		WithAgentPackage(s.AgentPackage),
	)
	s.Require().NoError(err, "should create tester")

	if !s.Run(fmt.Sprintf("upgrade to %s", t.agentPackage.AgentVersion()), func() {
		err = t.InstallAgent(windowsAgent.WithInstallLogFile(filepath.Join(s.OutputDir, "upgrade.log")))
		s.Require().NoError(err, "should upgrade to agent %s", t.agentPackage.AgentVersion())
	}) {
		s.T().FailNow()
	}

	if !t.TestExpectations(s.T()) {
		s.T().FailNow()
	}

	t.TestUninstall(s.T(), filepath.Join(s.OutputDir, "uninstall.log"))
}

type upgradeRollbackTestSuite struct {
	baseAgentMSISuite
}

func TestUpgradeRollback(t *testing.T) {
	s := &upgradeRollbackTestSuite{}
	run(t, s)
}

// TC-INS-002
func (s *upgradeRollbackTestSuite) TestUpgradeRollback() {
	vm := s.Env().RemoteHost
	s.prepareHost()

	previousTester := s.installLastStable(vm, nil)

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

	if !previousTester.TestExpectations(s.T()) {
		s.T().FailNow()
	}

	previousTester.TestUninstall(s.T(), filepath.Join(s.OutputDir, "uninstall.log"))
}

type repairTestSuite struct {
	baseAgentMSISuite
}

func TestRepair(t *testing.T) {
	s := &repairTestSuite{}
	run(t, s)
}

// TC-INS-001
func (s *repairTestSuite) TestRepair() {
	vm := s.Env().RemoteHost
	s.prepareHost()

	t := s.installAgent(vm, nil)

	err := windowsCommon.StopService(t.host, "DatadogAgent")
	s.Require().NoError(err)

	// Corrupt the install
	err = t.host.Remove("C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe")
	s.Require().NoError(err)
	err = t.host.RemoveAll("C:\\Program Files\\Datadog\\Datadog Agent\\embedded3")
	s.Require().NoError(err)

	if !s.Run("repair install", func() {
		err = windowsAgent.RepairAllAgent(t.host, "", filepath.Join(s.OutputDir, "repair.log"))
		s.Require().NoError(err)
	}) {
		s.T().FailNow()
	}

	if !t.TestExpectations(s.T()) {
		s.T().FailNow()
	}

	t.TestUninstall(s.T(), filepath.Join(s.OutputDir, "uninstall.log"))
}
