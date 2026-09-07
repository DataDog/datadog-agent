// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"os"
	"strconv"

	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

type packageProcmgrSwitchSuite struct {
	packageBaseSuite
}

func testProcmgrSwitch(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &packageProcmgrSwitchSuite{
		packageBaseSuite: newPackageSuite("procmgr_switch", os, arch, method),
	}
}

// TestProcmgrSwitch installs the agent under the service manager selected by
// DD_PROCESS_MANAGER_ENABLED (matrix-driven, defaults to true to match the
// installer's own default), verifies the expected manager is active, flips it
// via `datadog-installer process-manager enable|disable`, verifies the flip,
// then flips back and verifies the original state is restored.
func (s *packageProcmgrSwitchSuite) TestProcmgrSwitch() {
	initialEnabled := os.Getenv("DD_PROCESS_MANAGER_ENABLED") != "false"

	s.RunInstallScript("DD_REMOTE_UPDATES=true", "DD_PROCESS_MANAGER_ENABLED="+strconv.FormatBool(initialEnabled))
	defer s.Purge()

	s.assertManagerState(initialEnabled)

	s.runProcessManagerCommand(flipCommand(initialEnabled))
	s.assertManagerState(!initialEnabled)

	s.runProcessManagerCommand(flipCommand(!initialEnabled))
	s.assertManagerState(initialEnabled)
}

// flipCommand returns the process-manager subcommand that flips away from the
// given state.
func flipCommand(currentlyEnabled bool) string {
	if currentlyEnabled {
		return "disable"
	}
	return "enable"
}

func (s *packageProcmgrSwitchSuite) runProcessManagerCommand(subcommand string) {
	_, err := s.Env().RemoteHost.Execute("sudo datadog-installer process-manager " + subcommand)
	require.NoError(s.T(), err, "Failed to run process-manager %s: datadog-agent-installer journalctl:\n%s",
		subcommand,
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer.service --no-pager"),
	)
}

func (s *packageProcmgrSwitchSuite) assertManagerState(procmgrEnabled bool) {
	require.Equal(s.T(), procmgrEnabled, s.host.ProcmgrEnabled())
	if procmgrEnabled {
		s.host.WaitForUnitActive(s.T(), agentUnit, procmgrUnit)
		s.host.WaitForProcessesRunning(s.T(), ddotProcess)
	} else {
		s.host.WaitForUnitActive(s.T(), agentUnit, ddotUnit)
	}
}
