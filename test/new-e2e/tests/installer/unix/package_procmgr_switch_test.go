// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"os"

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

// TestProcmgrSwitch installs the agent, reaches the service manager state selected by
// DD_PROCESS_MANAGER_ENABLED (matrix-driven, defaults to true to match the installer's own
// default) via `datadog-installer process-manager enable|disable`, verifies the expected manager
// is active, flips it, verifies the flip, then flips back and verifies the original state is
// restored.
func (s *packageProcmgrSwitchSuite) TestProcmgrSwitch() {
	initialEnabled := os.Getenv("DD_PROCESS_MANAGER_ENABLED") != "false"

	// The install script does not forward DD_PROCESS_MANAGER_ENABLED to the package manager
	// invocation, so it can't be used to select the initial state here. Install with the (procmgr)
	// default and use the CLI to reach the desired initial state instead.
	s.RunInstallScript("DD_REMOTE_UPDATES=true", "DD_OTELCOLLECTOR_ENABLED=true")
	defer s.Purge()

	if !initialEnabled {
		s.runProcessManagerCommand("disable")
	}
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

// assertManagerState asserts the units matching procmgrEnabled are active. procmgrEnabled comes
// from DD_PROCESS_MANAGER_ENABLED (read once at test start, or flipped by the CLI call this
// tracks) rather than from any host-side probe: with WriteProcesses a no-op outside of ProcmgrType
// (see agentService.WriteProcesses), a stale processes.d/datadog-agent-ddot.yaml can survive a
// switch to systemd, so its presence can't be used to detect the current state.
func (s *packageProcmgrSwitchSuite) assertManagerState(procmgrEnabled bool) {
	if procmgrEnabled {
		s.host.WaitForUnitActive(s.T(), agentUnit, procmgrUnit)
		s.host.WaitForProcessesRunning(s.T(), ddotProcess)
	} else {
		s.host.WaitForUnitActive(s.T(), agentUnit, ddotUnit)
	}
}
