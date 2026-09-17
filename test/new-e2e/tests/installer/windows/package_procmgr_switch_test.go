// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

// Package procmgrswitchtests implements E2E tests for switching between dd-procmgrd and the
// legacy Windows SCM services, mirroring test/new-e2e/tests/installer/unix/package_procmgr_switch_test.go.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v7"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// packageProcmgrSwitchSuite exercises DD_PROCESS_MANAGER_ENABLED toggling via the running
// installer daemon on Windows, the counterpart to the Linux packageProcmgrSwitchSuite.
type packageProcmgrSwitchSuite struct {
	BaseSuite
}

// TestProcmgrSwitch installs the agent directly under the manager selected by the matrix-driven
// DD_PROCESS_MANAGER_ENABLED, with the DDOT extension enabled, then exercises the opposite
// transition first and switches back, checking at each step that both the agent's own services
// and the DDOT extension are managed correctly by whichever manager is active:
//   - started under procmgr (true): disable (SCM takes over), then re-enable (procmgr is back).
//   - started under SCM (false): enable (procmgr takes over), then disable (SCM is back).
func TestProcmgrSwitch(t *testing.T) {
	e2e.Run(t, &packageProcmgrSwitchSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake(),
		))
}

func (s *packageProcmgrSwitchSuite) AfterTest(_suiteName, _testName string) {
	s.Installer().Purge()
}

func (s *packageProcmgrSwitchSuite) TestSwitchesBetweenProcmgrAndSCM() {
	initialEnabled := os.Getenv("DD_PROCESS_MANAGER_ENABLED") != "false"

	output, err := s.InstallScript().Run(WithExtraEnvVars(map[string]string{
		"DD_PROCESS_MANAGER_ENABLED": strconv.FormatBool(initialEnabled),
	}))
	s.Require().NoErrorf(err, "failed to install the Datadog Agent: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService()

	installPath, err := windowsagent.GetInstallPathFromRegistry(s.Env().RemoteHost)
	s.Require().NoError(err)
	agentExe := filepath.Join(installPath, "bin", "agent.exe")
	agentPackageURL := "oci://" + consts.PipelineOCIRegistry + "/agent-package:pipeline-" + s.Env().Environment.PipelineID()
	cmd := fmt.Sprintf(`& "%s" otel install --url %s`, agentExe, agentPackageURL)
	output, err = s.Env().RemoteHost.Execute(cmd)
	s.Require().NoErrorf(err, "failed to install ddot extension via subcommand: %s", output)

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
// point, the same one Linux uses, since the switch is only exposed there and must execute inside
// the running daemon.
func (s *packageProcmgrSwitchSuite) runProcessManagerCommand(subcommand string) {
	s.Require().NoError(s.WaitForInstallerService("Running"))
	_, err := s.Installer().ProcessManager(subcommand)
	s.Require().NoErrorf(err, "failed to run process-manager %s", subcommand)
}

// assertManagerState asserts that the base agent's own SCM services and the DDOT extension are
// active under whichever manager procmgrEnabled selects.
func (s *packageProcmgrSwitchSuite) assertManagerState(procmgrEnabled bool) {
	if procmgrEnabled {
		s.Require().NoError(s.WaitForServicesWithBackoff("Running",
			[]string{"dd-procmgr-service"},
			backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)),
		))
		AssertDDOTManagedByProcmgrWindows(s.T(), s.Env().RemoteHost)
	} else {
		s.Require().NoError(s.WaitForServicesWithBackoff("Stopped",
			[]string{"dd-procmgr-service"},
			backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second)),
		))
		AssertWindowsDDOTRunningLegacySCM(s.T(), s.Env().RemoteHost)
	}
}
