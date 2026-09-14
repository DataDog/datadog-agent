// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
)

type packageProcmgrSwitchSuite struct {
	packageBaseSuite
}

func testProcmgrSwitch(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &packageProcmgrSwitchSuite{
		packageBaseSuite: newPackageSuite("procmgr_switch", os, arch, method),
	}
}

// RunInstallScript goes through the installer script instead of install_script_agent7.sh: the
// latter forwards only a fixed allowlist of DD_* variables to the package manager, so
// DD_PROCESS_MANAGER_ENABLED would never reach the Agent's postinst hook and the host would come
// up under procmgr whichever manager the matrix asked for.
func (s *packageProcmgrSwitchSuite) RunInstallScript(params ...string) {
	err := s.RunInstallScriptWithError(params...)
	require.NoErrorf(s.T(), err, "installer not properly installed. logs: \n%s\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/datadog-installer-stdout.log || true"),
		s.Env().RemoteHost.MustExecute("cat /tmp/datadog-installer-stderr.log || true"),
	)
}

func (s *packageProcmgrSwitchSuite) RunInstallScriptWithError(params ...string) error {
	scriptURL := "https://" + InstallerScriptBaseURL() + "/scripts/install.sh"
	_, err := s.Env().RemoteHost.Execute(
		fmt.Sprintf(`%s bash -c "$(curl -L %s)" > /tmp/datadog-installer-stdout.log 2> /tmp/datadog-installer-stderr.log`, strings.Join(params, " "), scriptURL),
		client.WithEnvVariables(InstallInstallerScriptEnvWithPackages()),
	)
	return err
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

// runProcessManagerCommand goes through the daemon's internal `daemon process-manager` entry
// point (like `daemon start-experiment`): the switch is only exposed there, not as a top-level
// datadog-installer command, since it must execute inside the running daemon.
func (s *packageProcmgrSwitchSuite) runProcessManagerCommand(subcommand string) {
	previousPID := s.installerDaemonPID()
	_, err := s.Env().RemoteHost.Execute("sudo datadog-installer daemon process-manager " + subcommand)
	require.NoError(s.T(), err, "Failed to run process-manager %s: datadog-agent-installer journalctl:\n%s",
		subcommand,
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer.service --no-pager"),
	)
	s.waitForInstallerDaemonRestart(previousPID)
}

// waitForInstallerDaemonRestart waits until the daemon that served the switch has been replaced
// by one that answers on the socket again.
//
// The switch ends by restarting datadog-agent.service, which systemd propagates to
// datadog-agent-installer.service as a try-restart because that unit BindsTo the main one, so the
// daemon tears itself down just after replying. assertManagerState only waits on the agent's own
// units, so without this the next command reaches the socket mid-restart and gets ECONNREFUSED.
// Same shape as Backend.runDaemonCommandWithRestart, which covers the experiment commands.
func (s *packageProcmgrSwitchSuite) waitForInstallerDaemonRestart(previousPID string) {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		pid := s.installerDaemonPID()
		if !assert.NotEqual(c, previousPID, pid, "%s still runs the daemon that served the switch", installerUnit) {
			return
		}
		_, err := s.Env().RemoteHost.Execute("sudo datadog-installer daemon rc-status")
		assert.NoError(c, err, "daemon %s is not serving the local API yet", pid)
	}, 2*time.Minute, 2*time.Second)
}

func (s *packageProcmgrSwitchSuite) installerDaemonPID() string {
	return strings.TrimSpace(s.host.Run("systemctl show -p MainPID --value " + installerUnit))
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
