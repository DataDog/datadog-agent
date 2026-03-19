// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const (
	procmgrdBin   = "/opt/datadog-agent/embedded/bin/dd-procmgrd"
	procmgrCLI    = "/opt/datadog-agent/embedded/bin/dd-procmgr"
	procmgrSocket = "/var/run/datadog-procmgrd/dd-procmgrd.sock"

	ddotPkgBinaryPath = "/opt/datadog-agent/embedded/bin/otel-agent"
	ddotExtBinaryPath = "/opt/datadog-agent/ext/ddot/embedded/bin/otel-agent"

	testProcessConfig = `command: /bin/sleep
args:
  - "3600"
auto_start: true
restart: always
description: E2E test process
`

	missingBinaryConfig = `command: /nonexistent/binary
condition_path_exists: /nonexistent/binary
auto_start: true
restart: never
description: should not start
`

	ddotConfigRepoPath = "pkg/fleet/installer/packages/embedded/tmpl/gen/debrpm/datadog-agent-ddot.yaml"
)

func repoRoot() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot determine test file path")
	}
	// thisFile is test/new-e2e/tests/agent-runtimes/procmgr/procmgr_nix_test.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..")
}

func readDDOTProcessConfig(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(), ddotConfigRepoPath))
	require.NoError(t, err, "failed to read DDOT process config from %s", ddotConfigRepoPath)
	return string(data)
}

type procmgrLinuxSuite struct {
	e2e.BaseSuite[environments.Host]
	hasCLI  bool
	hasDDOT bool
}

func TestProcmgrSmokeLinuxSuite(t *testing.T) {
	t.Parallel()
	ddotProcessConfig := readDDOTProcessConfig(t)
	e2e.Run(t, &procmgrLinuxSuite{}, e2e.WithProvisioner(
		awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithFile("/etc/datadog-agent/processes.d/test-sleep.yaml", testProcessConfig, true),
					agentparams.WithFile("/etc/datadog-agent/processes.d/datadog-agent-ddot.yaml", ddotProcessConfig, true),
					agentparams.WithFile("/etc/datadog-agent/processes.d/missing-binary.yaml", missingBinaryConfig, true),
				),
			),
		),
	))
}

func (s *procmgrLinuxSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	_, err := s.Env().RemoteHost.Execute("test -f " + procmgrdBin)
	if err != nil {
		s.T().Skip("dd-procmgrd not included in this agent package; skipping process manager tests")
	}

	_, err = s.Env().RemoteHost.Execute("test -f " + procmgrCLI)
	s.hasCLI = err == nil

	s.hasDDOT = s.installRealDDOT()

	if s.hasCLI {
		require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
			_, err := s.Env().RemoteHost.Execute("sudo chmod 0777 " + procmgrSocket)
			assert.NoError(t, err, "socket not yet available")
		}, 30*time.Second, 2*time.Second)
	}
}

func (s *procmgrLinuxSuite) installRealDDOT() bool {
	// Refresh repos first — this must succeed; a transient failure here
	// should not silently skip DDOT tests.
	s.Env().RemoteHost.MustExecute(
		"(sudo apt-get update -qq) || (sudo yum makecache -q)")

	// Check whether the package exists in repos. Only skip when genuinely absent.
	_, err := s.Env().RemoteHost.Execute(
		"(apt-cache show datadog-agent-ddot >/dev/null 2>&1) || " +
			"(yum info datadog-agent-ddot >/dev/null 2>&1)")
	if err != nil {
		s.T().Logf("datadog-agent-ddot package not found in repos; DDOT tests will be skipped")
		return false
	}

	// Package is available — install must succeed or the suite fails.
	s.Env().RemoteHost.MustExecute(
		"(sudo apt-get install -y datadog-agent-ddot) || " +
			"(sudo yum install -y datadog-agent-ddot)")

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

func (s *procmgrLinuxSuite) TestBinariesExist() {
	s.Env().RemoteHost.MustExecute("test -f " + procmgrdBin)

	if !s.hasCLI {
		s.T().Skip("dd-procmgr CLI not included in this agent package")
	}
	s.Env().RemoteHost.MustExecute("test -f " + procmgrCLI)
}

func (s *procmgrLinuxSuite) TestServiceRunning() {
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute("systemctl is-active datadog-agent-procmgrd")
		assert.Equal(t, "active", strings.TrimSpace(out))
	}, 30*time.Second, 2*time.Second)
}

func (s *procmgrLinuxSuite) TestCLIStatus() {
	s.requireCLI()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(procmgrCLI + " status")
		assertHasField(t, out, "Version")
		assertHasField(t, out, "Uptime")
	}, 30*time.Second, 2*time.Second)
}

func (s *procmgrLinuxSuite) TestCLIListShowsConfiguredProcess() {
	s.requireCLI()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(procmgrCLI + " list")
		assertTableRow(t, out, "test-sleep", map[string]string{
			"STATE":   "Running",
			"COMMAND": "/bin/sleep",
		})
	}, 30*time.Second, 2*time.Second)
}

func (s *procmgrLinuxSuite) TestCLIDescribe() {
	s.requireCLI()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(procmgrCLI + " describe test-sleep")
		assertField(t, out, "Name", "test-sleep")
		assertField(t, out, "State", "Running")
		assertField(t, out, "Command", "/bin/sleep")
	}, 30*time.Second, 2*time.Second)
}

func (s *procmgrLinuxSuite) TestConditionPathExistsSkipsMissingBinary() {
	s.requireCLI()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(procmgrCLI + " list")
		assertTableRow(t, out, "missing-binary", map[string]string{
			"STATE": "Created",
			"PID":   "-",
		})
	}, 30*time.Second, 2*time.Second)
}

func (s *procmgrLinuxSuite) TestDDOTProcessRunning() {
	s.requireDDOT()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(procmgrCLI + " list")
		assertTableRow(t, out, "datadog-agent-ddot", map[string]string{
			"STATE":   "Running",
			"COMMAND": ddotExtBinaryPath,
		})
	}, 60*time.Second, 2*time.Second)

	out := s.Env().RemoteHost.MustExecute(procmgrCLI + " describe datadog-agent-ddot")
	pid := fieldValue(out, "PID")
	require.NotEmpty(s.T(), pid, "PID should be reported for a Running process")
	require.NotEqual(s.T(), "-", pid, "PID should not be '-' for a Running process")
	s.Env().RemoteHost.MustExecute("test -d /proc/" + pid)

	pidFileContent := strings.TrimSpace(
		s.Env().RemoteHost.MustExecute("cat /opt/datadog-agent/run/otel-agent.pid"))
	assert.Equal(s.T(), pid, pidFileContent, "PID file should match procmgrd-reported PID")

	unitState := strings.TrimSpace(
		s.Env().RemoteHost.MustExecute("systemctl is-active datadog-agent-ddot.service || true"))
	assert.NotEqual(s.T(), "active", unitState, "systemd unit should not be active; procmgrd manages DDOT")
}

func (s *procmgrLinuxSuite) TestDDOTProcessDescribe() {
	s.requireDDOT()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(procmgrCLI + " describe datadog-agent-ddot")
		assertField(t, out, "Name", "datadog-agent-ddot")
		assertField(t, out, "State", "Running")
		assertField(t, out, "Command", ddotExtBinaryPath)
		assertField(t, out, "Restart Policy", "on-failure")
		assertHasField(t, out, "PID")
		assertHasField(t, out, "UUID")
	}, 60*time.Second, 2*time.Second)
}

func fieldValue(output, label string) string {
	needle := label + ":"
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, needle) {
			return strings.TrimSpace(trimmed[len(needle):])
		}
	}
	return ""
}

func assertField(t assert.TestingT, output, label, expected string) {
	needle := label + ":"
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, needle) {
			actual := strings.TrimSpace(trimmed[len(needle):])
			assert.Equal(t, expected, actual, "field %q", label)
			return
		}
	}
	assert.Fail(t, fmt.Sprintf("field %q not found in output:\n%s", label, output))
}

func assertHasField(t assert.TestingT, output, label string) {
	needle := label + ":"
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), needle) {
			return
		}
	}
	assert.Fail(t, fmt.Sprintf("field %q not found in output:\n%s", label, output))
}

func assertTableRow(t assert.TestingT, output, rowName string, expected map[string]string) {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if !assert.GreaterOrEqual(t, len(lines), 2, "list output should have header + at least one row") {
		return
	}
	header := lines[0]
	columns := parseTableColumns(header)

	for _, line := range lines[1:] {
		name := extractColumn(line, 0, columns)
		if name != rowName {
			continue
		}
		for col, want := range expected {
			idx := -1
			for i, c := range columns {
				if c.name == col {
					idx = i
					break
				}
			}
			if !assert.NotEqual(t, -1, idx, "column %q not in header: %s", col, header) {
				continue
			}
			got := extractColumn(line, idx, columns)
			assert.Equal(t, want, got, "row %q column %q", rowName, col)
		}
		return
	}
	assert.Fail(t, fmt.Sprintf("row %q not found in table output:\n%s", rowName, output))
}

type tableColumn struct {
	name  string
	start int
}

func parseTableColumns(header string) []tableColumn {
	var cols []tableColumn
	i := 0
	for i < len(header) {
		for i < len(header) && header[i] == ' ' {
			i++
		}
		if i >= len(header) {
			break
		}
		start := i
		for i < len(header) {
			if header[i] == ' ' {
				j := i
				for j < len(header) && header[j] == ' ' {
					j++
				}
				if j >= len(header) || (j-i >= 2) {
					break
				}
				i = j
			} else {
				i++
			}
		}
		cols = append(cols, tableColumn{name: header[start:i], start: start})
	}
	return cols
}

func extractColumn(line string, idx int, columns []tableColumn) string {
	if idx >= len(columns) {
		return ""
	}
	start := columns[idx].start
	end := len(line)
	if idx+1 < len(columns) {
		end = columns[idx+1].start
	}
	if start >= len(line) {
		return ""
	}
	if end > len(line) {
		end = len(line)
	}
	return strings.TrimSpace(line[start:end])
}

func (s *procmgrLinuxSuite) requireCLI() {
	s.T().Helper()
	if !s.hasCLI {
		s.T().Skip("dd-procmgr CLI not included in this agent package")
	}
}

func (s *procmgrLinuxSuite) requireDDOT() {
	s.T().Helper()
	if !s.hasDDOT {
		s.T().Skip("datadog-agent-ddot package not available")
	}
	s.requireCLI()
}
