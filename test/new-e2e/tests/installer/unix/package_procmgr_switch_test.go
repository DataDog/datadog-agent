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

// TestProcmgrSwitch installs the agent directly under the manager selected by the matrix-driven
// DD_PROCESS_MANAGER_ENABLED, with the DDOT extension enabled, then exercises the opposite
// transition first and switches back, checking at each step that both the agent's own units/
// processes and the DDOT extension are managed correctly by whichever manager is active:
//   - started under procmgr (true): disable (systemd takes over), then re-enable (procmgr is back).
//   - started under systemd (false): enable (procmgr takes over), then disable (systemd is back).
func (s *packageProcmgrSwitchSuite) TestProcmgrSwitch() {
	initialEnabled := os.Getenv("DD_PROCESS_MANAGER_ENABLED") != "false"

	s.RunInstallScript("DD_REMOTE_UPDATES=true", "DD_PROCESS_MANAGER_ENABLED="+strconv.FormatBool(initialEnabled))
	defer s.Purge()

	// Install the ddot extension (not the standalone datadog-agent-ddot package) so its lifecycle,
	// which is managed by the agent's own service definition, can be checked under both managers.
	agentPackageURL := "oci://installtesting.datad0g.com.internal.dda-testing.com/agent-package:pipeline-" + os.Getenv("E2E_PIPELINE_ID")
	s.host.Run("sudo datadog-agent otel install --url " + agentPackageURL)

	s.assertManagerState(initialEnabled)

	if initialEnabled {
		s.runProcessManagerCommand("disable")
		s.assertManagerState(false)

		s.runProcessManagerCommand("enable")
		s.assertManagerState(true)
	} else {
		s.runProcessManagerCommand("enable")
		s.assertManagerState(true)

		s.runProcessManagerCommand("disable")
		s.assertManagerState(false)
	}
}

func (s *packageProcmgrSwitchSuite) runProcessManagerCommand(subcommand string) {
	_, err := s.Env().RemoteHost.Execute("sudo datadog-installer process-manager " + subcommand)
	require.NoError(s.T(), err, "Failed to run process-manager %s: datadog-agent-installer journalctl:\n%s",
		subcommand,
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer.service --no-pager"),
	)
}

// assertManagerState asserts that the agent units and the DDOT extension are active under
// whichever manager procmgrEnabled selects.
func (s *packageProcmgrSwitchSuite) assertManagerState(procmgrEnabled bool) {
	if procmgrEnabled {
		s.host.WaitForUnitActive(s.T(), agentUnit, procmgrUnit)
		s.host.WaitForProcessesRunning(s.T(), ddotProcess)
	} else {
		s.host.WaitForUnitActive(s.T(), agentUnit, ddotUnit)
	}
}
