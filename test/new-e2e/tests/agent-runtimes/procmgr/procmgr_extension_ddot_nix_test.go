// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/internal/procmgrtest"
)

const (
	extensionDDOTConfigPath          = linuxConfigDir + "/datadog-agent-ddot.yaml"
	ddotRestartTestRuntimeSuccessSec = 5
)

type procmgrExtensionDDOTLinuxSuite struct {
	baseProcmgrSuite
	hasExtensionDDOT bool
	ddotInstallMsg   string // why install was skipped or failed (for requireExtensionDDOT Skip message)
}

func TestProcmgrExtensionDDOTLinuxSuite(t *testing.T) {
	t.Parallel()
	s := &procmgrExtensionDDOTLinuxSuite{}
	s.platform = linuxPlatform
	e2e.Run(t, s, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithFile(linuxConfigDir+"/test-sleep.yaml", linuxTestProcessConfig, true),
					agentparams.WithFile(linuxConfigDir+"/missing-binary.yaml", linuxMissingBinaryConfig, true),
				),
			),
		),
	))
}

func (s *procmgrExtensionDDOTLinuxSuite) SetupSuite() {
	s.baseProcmgrSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	s.hasExtensionDDOT = s.installDDOTExtension()
}

func datadogAgentCLI() string {
	if os.Getenv("E2E_FIPS") != "" {
		return "datadog-fips-agent"
	}
	return "datadog-agent"
}

func e2ePipelineIDForOCI() string {
	if id := strings.TrimSpace(os.Getenv("E2E_PIPELINE_ID")); id != "" {
		return id
	}
	return strings.TrimSpace(os.Getenv("CI_PIPELINE_ID"))
}

func (s *procmgrExtensionDDOTLinuxSuite) installDDOTExtension() bool {
	pipelineID := e2ePipelineIDForOCI()
	if pipelineID == "" {
		s.ddotInstallMsg = "E2E_PIPELINE_ID and CI_PIPELINE_ID unset (need pipeline id for agent-package OCI URL)"
		s.T().Log(s.ddotInstallMsg + "; skipping DDOT tests")
		return false
	}

	agent := datadogAgentCLI()
	_, err := s.Env().RemoteHost.Execute("command -v " + agent + " >/dev/null 2>&1")
	if err != nil {
		s.ddotInstallMsg = agent + " not on PATH"
		s.T().Logf("%s; skipping DDOT tests", s.ddotInstallMsg)
		return false
	}

	agentPackageURL := "oci://installtesting.datad0g.com.internal.dda-testing.com/agent-package:pipeline-" + pipelineID
	out, err := s.Env().RemoteHost.Execute(
		"sudo " + agent + " otel install --url " + agentPackageURL)
	if err != nil {
		s.ddotInstallMsg = "otel install failed: " + err.Error()
		if trimmed := strings.TrimSpace(out); trimmed != "" {
			s.ddotInstallMsg += ": " + trimmed
		}
		s.T().Logf("DDOT extension install failed: %v\n%s", err, strings.TrimSpace(out))
		return false
	}

	return true
}

func (s *procmgrExtensionDDOTLinuxSuite) TestDDOTProcessRunning() {
	s.requireExtensionDDOT()

	pid := s.waitForDDOTRunning().PID

	pidFileContent := strings.TrimSpace(
		s.Env().RemoteHost.MustExecute("cat /opt/datadog-agent/run/otel-agent.pid"))
	assert.Equal(s.T(), pid, pidFileContent, "PID file should match procmgrd-reported PID")

	unitState := strings.TrimSpace(
		s.Env().RemoteHost.MustExecute("systemctl is-active datadog-agent-ddot.service || true"))
	assert.NotEqual(s.T(), "active", unitState, "systemd unit should not be active; procmgrd manages DDOT")
}

func (s *procmgrExtensionDDOTLinuxSuite) TestExtensionDDOTManagedByProcmgrNotSystemdByDefault() {
	s.requireExtensionDDOT()

	// Same contract as TestStandaloneDDOTManagedByProcmgrNotSystemdByDefault: processes.d YAML,
	// global procmgr marker, systemd must not own DDOT, then dd-procmgr describe Running + stable
	// command line matches the extension layout (vs standalone datadog-agent-ddot package paths).
	stableYAML := procmgrtest.StableDDOTProcmgrYAMLPath(s.T(), s)

	_, err := s.Env().RemoteHost.Execute("test -f /etc/datadog-agent/.procmgr-enabled")
	require.NoError(s.T(), err, "installer should create the global procmgr marker on systemd hosts")

	cond := strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		"systemctl show datadog-agent-ddot.service -p ConditionResult --value"))
	assert.Equal(s.T(), "no", cond,
		"ConditionPathExists=!…/processes.d/datadog-agent-ddot.yaml should fail so systemd does not own DDOT")

	s.requireCLI()
	procmgrCLI := procmgrtest.CLIBinForLinuxHost(s.T(), s)
	procmgrtest.WaitForProcess(s.T(), s, procmgrtest.WaitForProcessArgs{
		ProcmgrCLIBin:  procmgrCLI,
		ProcessName:    procmgrtest.DDOTProcessName,
		ExpectedBinary: procmgrtest.DDOTOtelAgentExtensionBinary,
		DesiredState:   procmgrtest.ProcessStateRunning,
	})

	cmdLine, gerr := s.Env().RemoteHost.Execute(`sudo grep -E '^command:' "` + stableYAML + `"`)
	require.NoError(s.T(), gerr)
	require.Contains(s.T(), strings.TrimSpace(cmdLine), "/ext/ddot/",
		"processes.d command should use agent extension paths (ext/ddot); got %q", strings.TrimSpace(cmdLine))
}

func (s *procmgrExtensionDDOTLinuxSuite) TestDDOTRestartAfterKill() {
	s.requireExtensionDDOT()
	s.configureDDOTRuntimeSuccess(ddotRestartTestRuntimeSuccessSec)

	originalPID := s.waitForDDOTRunning().PID

	s.Env().RemoteHost.MustExecute("sudo kill -9 " + originalPID)

	newResult := s.waitForDDOTRunning()
	newPID, newRestarts := newResult.PID, newResult.Restarts

	require.NotEqual(s.T(), originalPID, newPID,
		"PID should differ after restart (was %s)", originalPID)
	assert.Equal(s.T(), 1, newRestarts,
		"Restarts should be 1 on the second running describe after kill (new %d)", newRestarts)
}

func (s *procmgrExtensionDDOTLinuxSuite) TestDDOTProcessDescribe() {
	s.requireExtensionDDOT()
	procmgrCLI := procmgrtest.CLIBinForLinuxHost(s.T(), s)
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(fmt.Sprintf(`sudo -u dd-agent -- %q describe %s`, procmgrCLI, procmgrtest.DDOTProcessName))
		assertField(t, out, "Name", procmgrtest.DDOTProcessName)
		assertField(t, out, "State", "Running")
		assertField(t, out, "Command", procmgrtest.DDOTOtelAgentExtensionBinary)
		assertField(t, out, "Restart Policy", "on-failure")
		assertHasField(t, out, "PID")
		assertHasField(t, out, "UUID")
	}, 60*time.Second, 2*time.Second)
}

func (s *procmgrExtensionDDOTLinuxSuite) waitForDDOTRunning() procmgrtest.WaitForProcessResult {
	s.T().Helper()
	procmgrCLI := procmgrtest.CLIBinForLinuxHost(s.T(), s)
	return procmgrtest.WaitForProcess(s.T(), s, procmgrtest.WaitForProcessArgs{
		ProcmgrCLIBin:  procmgrCLI,
		ProcessName:    procmgrtest.DDOTProcessName,
		ExpectedBinary: procmgrtest.DDOTOtelAgentExtensionBinary,
		DesiredState:   procmgrtest.ProcessStateRunning,
	})
}

func (s *procmgrExtensionDDOTLinuxSuite) ExecuteCommand(command string) (string, error) {
	return s.Env().RemoteHost.Execute(command)
}

func (s *procmgrExtensionDDOTLinuxSuite) configureDDOTRuntimeSuccess(seconds int) {
	s.T().Helper()
	require.Greater(s.T(), seconds, 0, "runtime_success_sec must be > 0")

	s.Env().RemoteHost.MustExecute(fmt.Sprintf(`sudo sh -c 'f=%q; if grep -q "^runtime_success_sec:" "$f"; then sed -i.bak -E "s/^runtime_success_sec:.*/runtime_success_sec: %d/" "$f"; else printf "\nruntime_success_sec: %d\n" >> "$f"; fi'`,
		extensionDDOTConfigPath, seconds, seconds))
	s.Env().RemoteHost.MustExecute("sudo systemctl restart datadog-agent-procmgr.service")
	s.Env().RemoteHost.MustExecute("sudo systemctl is-active datadog-agent-procmgr.service")
	s.waitForDDOTRunning()
}

func (s *procmgrExtensionDDOTLinuxSuite) requireExtensionDDOT() {
	s.T().Helper()
	if !s.hasExtensionDDOT {
		msg := "DDOT extension not installed"
		if s.ddotInstallMsg != "" {
			msg += " (" + s.ddotInstallMsg + ")"
		} else {
			msg += " (set E2E_PIPELINE_ID or CI_PIPELINE_ID and ensure datadog-agent otel install succeeds)"
		}
		s.T().Skip(msg)
	}
	s.requireCLI()
}
