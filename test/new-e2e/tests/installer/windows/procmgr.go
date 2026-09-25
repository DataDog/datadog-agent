// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

import (
	"path/filepath"
	"time"

	"github.com/stretchr/testify/assert"

	windowscommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// Processes supervised by dd-procmgrd. Each one is declared by a processes.d config named after it.
const (
	ddotProcmgrProcess         = "datadog-agent-ddot"
	processAgentProcmgrProcess = "datadog-agent-process"
)

// assertManagedByProcmgr verifies processName has a processes.d config and that dd-procmgrd
// reports it stably running.
func (s *BaseSuite) assertManagedByProcmgr(processName string) {
	s.T().Helper()
	s.Require().Host(s.Env().RemoteHost).FileExists(s.procmgrConfigPath(processName),
		"%s should have a processes.d config", processName)

	cli := s.procmgrCLIPath()
	var runningSince time.Time
	const minRunningDuration = 5 * time.Second
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		state, err := windowscommon.ProcmgrDescribeField(s.Env().RemoteHost, cli, processName, "State")
		if !assert.NoError(c, err) ||
			!assert.Equal(c, "Running", state, "%s should be running under dd-procmgrd", processName) {
			runningSince = time.Time{}
			return
		}
		if runningSince.IsZero() {
			runningSince = time.Now()
		}
		// EventuallyWithT treats a tick with no recorded failures as an immediate success, so the
		// stability window must be enforced via an assertion rather than a silent early return.
		assert.GreaterOrEqual(c, time.Since(runningSince), minRunningDuration,
			"%s has not been running long enough yet", processName)
	}, 3*time.Minute, 5*time.Second)
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
	return filepath.Join(s.procmgrInstallRoot(), "bin", "agent", "dd-procmgr.exe")
}

func (s *BaseSuite) procmgrConfigPath(processName string) string {
	return filepath.Join(s.procmgrInstallRoot(), "processes.d", processName+".yaml")
}
