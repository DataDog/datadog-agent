// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"fmt"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

// platformConfig holds all platform-specific paths, commands, and config
// snippets so that the shared test methods in baseProcmgrSuite work on both
// Linux and Windows without branching.
type platformConfig struct {
	daemonBin    string // path to dd-procmgrd binary
	cliBin       string // path to dd-procmgr CLI binary
	configDir    string // processes.d directory for agent file provisioning
	sleepCommand string // expected COMMAND column value in "list" output

	testProcessYAML   string // YAML config that starts a long-running sleep process
	missingBinaryYAML string // YAML config whose condition_path_exists prevents start

	// checkBinCmd returns a shell command that succeeds (exit 0) when the
	// given binary path exists on the remote host.
	checkBinCmd func(path string) string

	// checkSvcRunning is a shell command whose trimmed stdout indicates the
	// service is running (compared against svcRunningOutput).
	checkSvcRunning  string
	svcRunningOutput string

	// cliCmd returns the full shell command to invoke the procmgr CLI with
	// the given arguments (handles quoting differences between bash and
	// PowerShell).
	cliCmd func(args string) string
}

type baseProcmgrSuite struct {
	e2e.BaseSuite[environments.Host]
	platform platformConfig
	hasCLI   bool
}

func (s *baseProcmgrSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	_, err := s.Env().RemoteHost.Execute(s.platform.checkBinCmd(s.platform.daemonBin))
	if err != nil {
		s.T().Skip("procmgr daemon not included in this agent package; skipping process manager tests")
	}

	_, err = s.Env().RemoteHost.Execute(s.platform.checkBinCmd(s.platform.cliBin))
	s.hasCLI = err == nil
}

func (s *baseProcmgrSuite) requireCLI() {
	s.T().Helper()
	if !s.hasCLI {
		s.T().Skip("dd-procmgr CLI not included in this agent package")
	}
}

// ---------------------------------------------------------------------------
// Shared tests — run on both Linux and Windows
// ---------------------------------------------------------------------------

func (s *baseProcmgrSuite) TestBinariesExist() {
	s.Env().RemoteHost.MustExecute(s.platform.checkBinCmd(s.platform.daemonBin))

	if !s.hasCLI {
		s.T().Skip("dd-procmgr CLI not included in this agent package")
	}
	s.Env().RemoteHost.MustExecute(s.platform.checkBinCmd(s.platform.cliBin))
}

func (s *baseProcmgrSuite) TestServiceRunning() {
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute(s.platform.checkSvcRunning)
		assert.NoError(t, err)
		assert.Equal(t, s.platform.svcRunningOutput, strings.TrimSpace(out))
	}, 30*time.Second, 2*time.Second)
}

func (s *baseProcmgrSuite) TestCLIStatus() {
	s.requireCLI()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("status"))
		assertHasField(t, out, "Version")
		assertHasField(t, out, "Uptime")
	}, 30*time.Second, 2*time.Second)
}

func (s *baseProcmgrSuite) TestCLIListShowsConfiguredProcess() {
	s.requireCLI()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("list"))
		assertTableRow(t, out, "test-sleep", map[string]string{
			"STATE":   "Running",
			"COMMAND": s.platform.sleepCommand,
		})
	}, 30*time.Second, 2*time.Second)
}

func (s *baseProcmgrSuite) TestCLIDescribe() {
	s.requireCLI()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("describe test-sleep"))
		assertField(t, out, "Name", "test-sleep")
		assertField(t, out, "State", "Running")
		assertField(t, out, "Command", s.platform.sleepCommand)
	}, 30*time.Second, 2*time.Second)
}

func (s *baseProcmgrSuite) TestConditionPathExistsSkipsMissingBinary() {
	s.requireCLI()
	require.EventuallyWithT(s.T(), func(t *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute(s.platform.cliCmd("list"))
		assertTableRow(t, out, "missing-binary", map[string]string{
			"STATE": "Created",
			"PID":   "-",
		})
	}, 30*time.Second, 2*time.Second)
}

// ---------------------------------------------------------------------------
// CLI output parsing helpers
// ---------------------------------------------------------------------------

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
