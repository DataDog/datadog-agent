// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/internal/procmgrwait"
)

const (
	linuxDaemonBin = "/opt/datadog-agent/embedded/bin/dd-procmgrd"
	linuxCLIBin    = "/opt/datadog-agent/embedded/bin/dd-procmgr"
	linuxConfigDir = "/opt/datadog-agent/processes.d"

	// Extension layout after `datadog-agent otel install` (matches installer TestInstallDDOTSubcommand).
	ddotOtelAgentBinaryPath = "/opt/datadog-agent/ext/ddot/embedded/bin/otel-agent"

	linuxTestProcessConfig = `command: /bin/sleep
args:
  - "3600"
auto_start: true
restart: always
description: E2E test process
`

	linuxMissingBinaryConfig = `command: /nonexistent/binary
condition_path_exists: /nonexistent/binary
auto_start: true
restart: never
description: should not start
`
)

var linuxPlatform = platformConfig{
	daemonBin:         linuxDaemonBin,
	cliBin:            linuxCLIBin,
	configDir:         linuxConfigDir,
	sleepCommand:      "/bin/sleep",
	testProcessYAML:   linuxTestProcessConfig,
	missingBinaryYAML: linuxMissingBinaryConfig,
	checkBinCmd:       func(path string) string { return "test -f " + path },
	checkSvcRunning:   "systemctl is-active datadog-agent-procmgr",
	svcRunningOutput:  "active",
	// Run CLI as dd-agent so it can use the procmgrd socket without chmod (same as installer DDOT tests).
	cliCmd: func(args string) string {
		return fmt.Sprintf("sudo -u dd-agent -- %q %s", linuxCLIBin, args)
	},
}

type procmgrLinuxSuite struct {
	baseProcmgrSuite
	hasDDOT        bool
	ddotInstallMsg string // why install was skipped or failed (for requireDDOT Skip message)
}

func TestProcmgrSmokeLinuxSuite(t *testing.T) {
	t.Parallel()
	s := &procmgrLinuxSuite{}
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

func (s *procmgrLinuxSuite) SetupSuite() {
	s.baseProcmgrSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	s.hasDDOT = s.installDDOTExtension()
}

// ---------------------------------------------------------------------------
// Linux-only: DDOT tests
// ---------------------------------------------------------------------------

// datadogAgentCLI returns the on-host agent binary name used for `otel install`
// (matches installer unix DDOT tests and FIPS CI when E2E_FIPS is set).
func datadogAgentCLI() string {
	if os.Getenv("E2E_FIPS") != "" {
		return "datadog-fips-agent"
	}
	return "datadog-agent"
}

// e2ePipelineIDForOCI returns the pipeline id used in installtesting OCI tags
// (agent-package:pipeline-<id>). Prefer E2E_PIPELINE_ID; fall back to CI_PIPELINE_ID
// so jobs that do not inject E2E_PIPELINE_ID still work on GitLab runners.
func e2ePipelineIDForOCI() string {
	if id := strings.TrimSpace(os.Getenv("E2E_PIPELINE_ID")); id != "" {
		return id
	}
	return strings.TrimSpace(os.Getenv("CI_PIPELINE_ID"))
}

// installDDOTExtension installs the DDOT extension the same way as production:
// `datadog-agent otel install` pulling the pipeline agent-package OCI (see
// test/new-e2e/tests/installer/unix/package_ddot_test.go). The installer already
// restarts datadog-agent after extension install (pkg/fleet/installer/installer.go
// InstallExtensions); procmgrd follows via unit dependencies.
func (s *procmgrLinuxSuite) installDDOTExtension() bool {
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

func (s *procmgrLinuxSuite) TestDDOTProcessRunning() {
	s.requireDDOT()

	pid := s.waitForRunningProcess("datadog-agent-ddot", ddotOtelAgentBinaryPath, 60*time.Second)

	pidFileContent := strings.TrimSpace(
		s.Env().RemoteHost.MustExecute("cat /opt/datadog-agent/run/otel-agent.pid"))
	assert.Equal(s.T(), pid, pidFileContent, "PID file should match procmgrd-reported PID")

	unitState := strings.TrimSpace(
		s.Env().RemoteHost.MustExecute("systemctl is-active datadog-agent-ddot.service || true"))
	assert.NotEqual(s.T(), "active", unitState, "systemd unit should not be active; procmgrd manages DDOT")
}

// TestDDOTManagedByProcmgrNotSystemdByDefault checks that when processes.d
// contains the DDOT definition (default when procmgr gates are on), systemd
// skips the datadog-agent-ddot unit and dd-procmgr runs the collector instead.
func (s *procmgrLinuxSuite) TestDDOTManagedByProcmgrNotSystemdByDefault() {
	s.requireDDOT()

	_, err := s.Env().RemoteHost.Execute("test -f " + linuxConfigDir + "/datadog-agent-ddot.yaml")
	require.NoError(s.T(), err, "processes.d DDOT YAML should be present after extension install (fleet sync hook)")

	_, err = s.Env().RemoteHost.Execute("test -f /etc/datadog-agent/.procmgr-enabled")
	require.NoError(s.T(), err, "fleet post-install should create the global procmgr marker on systemd hosts")

	cond := strings.TrimSpace(s.Env().RemoteHost.MustExecute(
		"systemctl show datadog-agent-ddot.service -p ConditionResult --value"))
	assert.Equal(s.T(), "no", cond,
		"ConditionPathExists=!…/processes.d/datadog-agent-ddot.yaml should fail so systemd does not own DDOT")

	s.requireCLI()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("list"))
		assertTableRow(t, out, "datadog-agent-ddot", map[string]string{
			"STATE": "Running",
		})
	}, 60*time.Second, 2*time.Second)
}

func (s *procmgrLinuxSuite) TestDDOTRestartAfterKill() {
	s.requireDDOT()

	originalPID := s.waitForRunningProcess("datadog-agent-ddot", ddotOtelAgentBinaryPath, 60*time.Second)

	baselineRestarts := s.getRestartCount("datadog-agent-ddot")

	s.Env().RemoteHost.MustExecute("sudo kill -9 " + originalPID)

	newPID := s.waitForRunningProcess("datadog-agent-ddot", ddotOtelAgentBinaryPath, 30*time.Second)

	require.NotEqual(s.T(), originalPID, newPID,
		"PID should differ after restart (was %s)", originalPID)
	assert.Equal(s.T(), baselineRestarts+1, s.getRestartCount("datadog-agent-ddot"),
		"Restarts should have increased by 1 (baseline %d)", baselineRestarts)
}

func (s *procmgrLinuxSuite) TestDDOTProcessDescribe() {
	s.requireDDOT()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("describe datadog-agent-ddot"))
		assertField(t, out, "Name", "datadog-agent-ddot")
		assertField(t, out, "State", "Running")
		assertField(t, out, "Command", ddotOtelAgentBinaryPath)
		assertField(t, out, "Restart Policy", "on-failure")
		assertHasField(t, out, "PID")
		assertHasField(t, out, "UUID")
	}, 60*time.Second, 2*time.Second)
}

// ---------------------------------------------------------------------------
// Linux-only helpers
// ---------------------------------------------------------------------------

func (s *procmgrLinuxSuite) waitForRunningProcess(name, expectedBinary string, timeout time.Duration) string {
	s.T().Helper()
	describeCmd := s.platform.cliCmd("describe " + name)
	return procmgrwait.WaitForRunningProcess(
		s.T(),
		func(command string) (string, error) { return s.Env().RemoteHost.Execute(command) },
		describeCmd,
		name,
		expectedBinary,
		timeout,
	)
}

func (s *procmgrLinuxSuite) getRestartCount(name string) int {
	s.T().Helper()
	out := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("describe " + name))
	count, err := strconv.Atoi(fieldValue(out, "Restarts"))
	require.NoError(s.T(), err, "Restarts field for %s should be a number", name)
	return count
}

func (s *procmgrLinuxSuite) requireDDOT() {
	s.T().Helper()
	if !s.hasDDOT {
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
