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

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const (
	linuxDaemonBin = "/opt/datadog-agent/embedded/bin/dd-procmgrd"
	linuxCLIBin    = "/opt/datadog-agent/embedded/bin/dd-procmgr"
	linuxSocket    = "/var/run/datadog-procmgrd/dd-procmgrd.sock"
	linuxConfigDir = "/opt/datadog-agent/processes.d"

	ddotPkgBinaryPath = "/opt/datadog-agent/embedded/bin/otel-agent"
	ddotExtBinaryPath = "/opt/datadog-agent/ext/ddot/embedded/bin/otel-agent"

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
	checkSvcRunning:   "systemctl is-active datadog-agent-procmgrd",
	svcRunningOutput:  "active",
	cliCmd:            func(args string) string { return linuxCLIBin + " " + args },
}

type procmgrLinuxSuite struct {
	baseProcmgrSuite
	hasDDOT bool
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
					agentparams.WithFile(linuxConfigDir+"/datadog-agent-ddot.yaml", embedded.DDOTProcessConfig, true),
					agentparams.WithFile(linuxConfigDir+"/missing-binary.yaml", linuxMissingBinaryConfig, true),
				),
			),
		),
	))
}

func (s *procmgrLinuxSuite) SetupSuite() {
	s.baseProcmgrSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	s.hasDDOT = s.installRealDDOT()

	if s.hasCLI {
		require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
			_, err := s.Env().RemoteHost.Execute("sudo chmod 0777 " + linuxSocket)
			assert.NoError(t, err, "socket not yet available")
		}, 30*time.Second, 2*time.Second)
	}
}

// ---------------------------------------------------------------------------
// Linux-only: DDOT tests
// ---------------------------------------------------------------------------

func ddotPackageName() string {
	if os.Getenv("E2E_FIPS") != "" {
		return "datadog-fips-agent-ddot"
	}
	return "datadog-agent-ddot"
}

func (s *procmgrLinuxSuite) installRealDDOT() bool {
	pkg := ddotPackageName()

	s.Env().RemoteHost.MustExecute(
		"(sudo apt-get update -qq) || (sudo yum makecache -q)")

	_, err := s.Env().RemoteHost.Execute(
		"(apt-cache show " + pkg + " >/dev/null 2>&1) || " +
			"(yum info " + pkg + " >/dev/null 2>&1)")
	if err != nil {
		s.T().Logf("%s package not found in repos; DDOT tests will be skipped", pkg)
		return false
	}

	s.Env().RemoteHost.MustExecute(
		"(sudo apt-get install -y " + pkg + ") || " +
			"(sudo yum install -y " + pkg + ")")

	s.Env().RemoteHost.Execute("sudo systemctl stop datadog-agent-ddot.service || true")
	s.Env().RemoteHost.Execute("sudo systemctl reset-failed datadog-agent-ddot.service || true")
	s.Env().RemoteHost.Execute("sudo systemctl disable datadog-agent-ddot.service || true")

	s.Env().RemoteHost.MustExecute("sudo mkdir -p /opt/datadog-agent/ext/ddot/embedded/bin")
	s.Env().RemoteHost.MustExecute(fmt.Sprintf("sudo cp %s %s", ddotPkgBinaryPath, ddotExtBinaryPath))

	s.Env().RemoteHost.MustExecute(`sudo sh -c "printf 'otelcollector:\n  enabled: true\n' >> /etc/datadog-agent/datadog.yaml"`)
	s.Env().RemoteHost.MustExecute(`sudo sh -c "sed -e 's/\${env:DD_API_KEY}/aaaaaaaaaaaaaaaa/' -e 's/\${env:DD_SITE}/datadoghq.com/' /etc/datadog-agent/otel-config.yaml.example > /etc/datadog-agent/otel-config.yaml"`)
	s.Env().RemoteHost.MustExecute("sudo chown dd-agent:dd-agent /etc/datadog-agent/otel-config.yaml && sudo chmod 640 /etc/datadog-agent/otel-config.yaml")

	s.Env().RemoteHost.MustExecute("sudo systemctl restart datadog-agent.service")
	s.Env().RemoteHost.MustExecute("sudo systemctl restart datadog-agent-procmgrd")

	return true
}

func (s *procmgrLinuxSuite) TestDDOTProcessRunning() {
	s.requireDDOT()

	pid := s.waitForRunningProcess("datadog-agent-ddot", ddotExtBinaryPath, 60*time.Second)

	pidFileContent := strings.TrimSpace(
		s.Env().RemoteHost.MustExecute("cat /opt/datadog-agent/run/otel-agent.pid"))
	assert.Equal(s.T(), pid, pidFileContent, "PID file should match procmgrd-reported PID")

	unitState := strings.TrimSpace(
		s.Env().RemoteHost.MustExecute("systemctl is-active datadog-agent-ddot.service || true"))
	assert.NotEqual(s.T(), "active", unitState, "systemd unit should not be active; procmgrd manages DDOT")
}

func (s *procmgrLinuxSuite) TestDDOTRestartAfterKill() {
	s.requireDDOT()

	originalPID := s.waitForRunningProcess("datadog-agent-ddot", ddotExtBinaryPath, 60*time.Second)

	baselineRestarts := s.getRestartCount("datadog-agent-ddot")

	s.Env().RemoteHost.MustExecute("sudo kill -9 " + originalPID)

	newPID := s.waitForRunningProcess("datadog-agent-ddot", ddotExtBinaryPath, 30*time.Second)

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
		assertField(t, out, "Command", ddotExtBinaryPath)
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
	var pid string
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("describe " + name))
		assertField(t, out, "State", "Running")
		p := fieldValue(out, "PID")
		if !assert.NotEmpty(t, p, "PID should be present for a Running process") ||
			!assert.NotEqual(t, "-", p, "PID should not be '-' for a Running process") {
			return
		}
		s.assertProcessBinary(t, p, expectedBinary)
		pid = p
	}, timeout, 2*time.Second)
	return pid
}

func (s *procmgrLinuxSuite) getRestartCount(name string) int {
	s.T().Helper()
	out := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("describe " + name))
	count, err := strconv.Atoi(fieldValue(out, "Restarts"))
	require.NoError(s.T(), err, "Restarts field for %s should be a number", name)
	return count
}

func (s *procmgrLinuxSuite) assertProcessBinary(t assert.TestingT, pid, expectedBinary string) {
	out, err := s.Env().RemoteHost.Execute("sudo readlink -f /proc/" + pid + "/exe")
	if !assert.NoError(t, err, "readlink /proc/%s/exe failed (process may have exited)", pid) {
		return
	}
	assert.Equal(t, expectedBinary, strings.TrimSpace(out),
		"process %s should be running %s", pid, expectedBinary)
}

func (s *procmgrLinuxSuite) requireDDOT() {
	s.T().Helper()
	if !s.hasDDOT {
		s.T().Skipf("%s package not available", ddotPackageName())
	}
	s.requireCLI()
}
