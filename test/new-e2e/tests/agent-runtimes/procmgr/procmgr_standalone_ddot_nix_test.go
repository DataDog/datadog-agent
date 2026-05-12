// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/internal/procmgrtest"
)

type procmgrStandaloneDDOTLinuxSuite struct {
	baseProcmgrSuite
	hasStandaloneDDOT    bool
	standaloneInstallMsg string
}

func TestProcmgrStandaloneDDOTLinuxSuite(t *testing.T) {
	t.Parallel()
	s := &procmgrStandaloneDDOTLinuxSuite{}
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

func (s *procmgrStandaloneDDOTLinuxSuite) SetupSuite() {
	s.baseProcmgrSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	s.hasStandaloneDDOT, s.standaloneInstallMsg = s.installStandaloneDDOTPackage()
	if s.hasStandaloneDDOT {
		s.T().Cleanup(func() {
			_, _ = s.Env().RemoteHost.Execute("sudo datadog-installer remove datadog-agent-ddot")
		})
	}
}

func (s *procmgrStandaloneDDOTLinuxSuite) installStandaloneDDOTPackage() (ok bool, msg string) {
	pipelineID := e2ePipelineIDForOCI()
	if pipelineID == "" {
		return false, "E2E_PIPELINE_ID and CI_PIPELINE_ID unset (need pipeline id for ddot-package OCI URL)"
	}

	_, err := s.Env().RemoteHost.Execute("command -v datadog-installer >/dev/null 2>&1")
	if err != nil {
		return false, "datadog-installer not on PATH"
	}

	ddotURL := "oci://installtesting.datad0g.com.internal.dda-testing.com/ddot-package:pipeline-" + pipelineID
	out, err := s.Env().RemoteHost.Execute("sudo datadog-installer install " + ddotURL)
	if err != nil {
		m := "datadog-installer install failed: " + err.Error()
		if trimmed := strings.TrimSpace(out); trimmed != "" {
			m += ": " + trimmed
		}
		return false, m
	}
	return true, ""
}

func (s *procmgrStandaloneDDOTLinuxSuite) requireStandaloneDDOT() {
	s.T().Helper()
	if !s.hasStandaloneDDOT {
		msg := "standalone datadog-agent-ddot not installed"
		if s.standaloneInstallMsg != "" {
			msg += " (" + s.standaloneInstallMsg + ")"
		}
		s.T().Skip(msg)
	}
	s.requireCLI()
}

func (s *procmgrStandaloneDDOTLinuxSuite) TestStandaloneDDOTManagedByProcmgrNotSystemdByDefault() {
	s.requireStandaloneDDOT()

	// Same contract as TestDDOTManagedByProcmgrNotSystemdByDefault: processes.d YAML,
	// global procmgr marker, systemd must not own DDOT, then dd-procmgr describe Running + stable
	// command line matches the standalone datadog-agent-ddot package (vs extension ext/ddot paths).
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
		ExpectedBinary: procmgrtest.DDOTOtelAgentFleetPackageBinary,
		DesiredState:   procmgrtest.ProcessStateRunning,
	})

	cmdLine, gerr := s.Env().RemoteHost.Execute(`sudo grep -E '^command:' "` + stableYAML + `"`)
	require.NoError(s.T(), gerr)
	require.Contains(s.T(), strings.TrimSpace(cmdLine), "datadog-packages/datadog-agent-ddot/",
		"processes.d command should use standalone package paths; got %q", strings.TrimSpace(cmdLine))
}

func (s *procmgrStandaloneDDOTLinuxSuite) ExecuteCommand(command string) (string, error) {
	return s.Env().RemoteHost.Execute(command)
}
