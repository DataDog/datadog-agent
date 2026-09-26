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

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
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
// dd-procmgr has no restart command, and its stop returns once the stop is requested rather
// than once the child is gone, so the intermediate Stopped state has to be waited for
// explicitly. The explicit start is not subject to the process's config gate, which only
// governs auto-start and reload.
func (s *BaseSuite) restartUnderProcmgr(processName string) {
	s.T().Helper()
	cli := s.procmgrCLIPath()

	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`& '%s' stop %s`, strings.ReplaceAll(cli, `'`, `''`), processName))
	s.Require().NoErrorf(err, "failed to stop %s under dd-procmgrd", processName)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		state, err := procmgrDescribeField(s.Env().RemoteHost, cli, processName, "State")
		if !assert.NoError(c, err) {
			return
		}
		assert.Equal(c, "Stopped", state, "%s should have stopped under dd-procmgrd", processName)
	}, 1*time.Minute, 2*time.Second)

	_, err = s.Env().RemoteHost.Execute(fmt.Sprintf(`& '%s' start %s`, strings.ReplaceAll(cli, `'`, `''`), processName))
	s.Require().NoErrorf(err, "failed to start %s under dd-procmgrd", processName)
	s.assertManagedByProcmgr(processName)
}

// assertNotRunningUnderProcmgr verifies processName is declared to dd-procmgrd but is not
// running. The config existing is the point: asserting only that the legacy SCM service is
// stopped would pass for a process procmgr had happily started instead.
func (s *BaseSuite) assertNotRunningUnderProcmgr(processName string) {
	s.T().Helper()
	s.Require().Host(s.Env().RemoteHost).FileExists(s.procmgrConfigPath(processName),
		"%s should have a processes.d config", processName)

	state, err := procmgrDescribeField(s.Env().RemoteHost, s.procmgrCLIPath(), processName, "State")
	s.Require().NoError(err)
	s.Require().NotEqual("Running", state, "%s should not be running under dd-procmgrd", processName)
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

// procmgrDescribeField is the package-local name for the shared describe helper, kept so the
// callers that read fields other than State stay unchanged.
func procmgrDescribeField(host *components.RemoteHost, cli, processName, field string) (string, error) {
	return windowsagent.GetProcmgrProcessField(host, cli, processName, field)
}
