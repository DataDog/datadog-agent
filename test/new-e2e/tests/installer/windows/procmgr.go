// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

import (
	"fmt"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"

	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// Processes supervised by dd-procmgrd. Each one is declared by a processes.d config named after it.
const (
	ddotProcmgrProcess         = "datadog-agent-ddot"
	processAgentProcmgrProcess = "datadog-agent-process"
	sysprobeProcmgrProcess     = "datadog-agent-sysprobe"
)

// assertManagedByProcmgr verifies processName has a processes.d config and that dd-procmgrd
// reports it stably running.
func (s *BaseSuite) assertManagedByProcmgr(processName string) {
	s.T().Helper()
	s.Require().Host(s.Env().RemoteHost).FileExists(s.procmgrConfigPath(processName),
		"%s should have a processes.d config", processName)

	windowsagent.AssertProcmgrProcessRunning(s.T(), s.Env().RemoteHost, processName)
}

// restartUnderProcmgr cycles a supervised process so it rereads its configuration.
//
// dd-procmgr has no restart command, so this is a stop followed by a start. The explicit start
// is not subject to the process's config gate, which only governs auto-start and reload.
func (s *BaseSuite) restartUnderProcmgr(processName string) {
	s.T().Helper()
	s.procmgrCommandUntilState(processName, "stop", "Stopped")
	s.procmgrCommandUntilState(processName, "start", "Running")
	s.assertManagedByProcmgr(processName)
}

// procmgrCommandUntilState issues `dd-procmgr <verb> <processName>` until dd-procmgrd reports
// wantState.
//
// Retrying rather than requiring the first request to succeed: these commands travel over
// dd-procmgrd's local gRPC channel, which has been seen aborting a call mid-flight, and the
// state dd-procmgrd reports afterwards is the only thing the test actually depends on.
func (s *BaseSuite) procmgrCommandUntilState(processName, verb, wantState string) {
	s.T().Helper()
	cli := s.procmgrCLIPath()
	command := fmt.Sprintf(`& '%s' %s %s`, strings.ReplaceAll(cli, `'`, `''`), verb, processName)

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		state, err := windowscommon.ProcmgrDescribeField(s.Env().RemoteHost, cli, processName, "State")
		if err != nil || state != wantState {
			// dd-procmgr rejects a verb the state has already reached, so this only runs while
			// the process still needs it, or while the state cannot be read at all.
			if _, execErr := s.Env().RemoteHost.Execute(command); execErr != nil {
				s.T().Logf("dd-procmgr %s %s failed: %v", verb, processName, execErr)
			}
			state, err = windowscommon.ProcmgrDescribeField(s.Env().RemoteHost, cli, processName, "State")
		}
		if !assert.NoError(c, err) {
			return
		}
		assert.Equalf(c, wantState, state, "%s should be %s under dd-procmgrd", processName, wantState)
	}, 2*time.Minute, 5*time.Second)
}

// assertNotRunningUnderProcmgr verifies processName is declared to dd-procmgrd but was never
// started. The config existing is the point: asserting only that the legacy SCM service is
// stopped would pass for a process procmgr had happily started instead.
//
// Created rather than "anything but Running", which a process that started and then exited
// or failed would also satisfy. A process dd-procmgrd never spawned because its config gate
// was closed stays in Created, so that is the state the claim actually rests on.
func (s *BaseSuite) assertNotRunningUnderProcmgr(processName string) {
	s.T().Helper()
	s.Require().Host(s.Env().RemoteHost).FileExists(s.procmgrConfigPath(processName),
		"%s should have a processes.d config", processName)

	// Only the read is retried, so a dd-procmgrd channel that aborts a call does not read as a
	// verdict on the state. The state itself gets one chance: Created is not a stage anything
	// passes through, so a retry could only mask a process that was started and stopped again.
	var state string
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		var err error
		state, err = windowscommon.ProcmgrDescribeField(s.Env().RemoteHost, s.procmgrCLIPath(), processName, "State")
		assert.NoError(c, err)
	}, 1*time.Minute, 5*time.Second, "should be able to read the %s state from dd-procmgrd", processName)

	s.Require().Equal("Created", state,
		"%s should never have been started by dd-procmgrd", processName)
}

// assertNoProcmgrConfig verifies processes.d has no config for processName.
func (s *BaseSuite) assertNoProcmgrConfig(processName string) {
	s.T().Helper()
	s.Require().Host(s.Env().RemoteHost).NoFileExists(s.procmgrConfigPath(processName),
		"%s should not have a processes.d config", processName)
}

func (s *BaseSuite) procmgrInstallRoot() string {
	s.T().Helper()
	installRoot, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	s.Require().NoError(err)
	return installRoot
}

func (s *BaseSuite) procmgrCLIPath() string {
	return windowsagent.ProcmgrCLIPath(s.procmgrInstallRoot())
}

func (s *BaseSuite) procmgrConfigPath(processName string) string {
	return windowsagent.ProcmgrProcessConfigPath(s.procmgrInstallRoot(), processName)
}
