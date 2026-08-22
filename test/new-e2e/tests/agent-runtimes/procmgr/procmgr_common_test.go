// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package procmgr contains end-to-end tests for dd-procmgr and dd-procmgrd.
package procmgr

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

type platformConfig struct {
	daemonBin    string
	cliBin       string
	configDir    string
	sleepCommand string // COMMAND column in `list` output

	testProcessYAML   string
	missingBinaryYAML string

	checkFileExists  func(path string) string
	checkSvcRunning  string
	svcRunningOutput string

	cliCmd     func(args string) string
	killPIDCmd func(pid uint32) string // SIGKILL / Stop-Process, not `dd-procmgr stop`
}

type baseProcmgrSuite struct {
	e2e.BaseSuite[environments.Host]
	platform platformConfig
}

func (s *baseProcmgrSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	_, err := s.Env().RemoteHost.Execute(s.platform.checkFileExists(s.platform.daemonBin))
	if err != nil {
		s.T().Skip("procmgr daemon not included in this agent package; skipping process manager tests")
	}
}

func (s *baseProcmgrSuite) TestBinariesExist() {
	s.Env().RemoteHost.MustExecute(s.platform.checkFileExists(s.platform.daemonBin))
	s.Env().RemoteHost.MustExecute(s.platform.checkFileExists(s.platform.cliBin))
}

func (s *baseProcmgrSuite) TestServiceRunning() {
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.platform.checkSvcRunning)
		assert.Equal(ct, s.platform.svcRunningOutput, strings.TrimSpace(out))
	}, 30*time.Second, 2*time.Second)
}

func (s *baseProcmgrSuite) TestCLIStatus() {
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.cliStatus())
		assertHasField(ct, out, "Version")
		assertField(ct, out, "Ready", "true")
		assertHasField(ct, out, "Uptime")
		assertHasField(ct, out, "Total Processes")
		assertHasField(ct, out, "Running")
		assertHasField(ct, out, "Stopped")
		assertHasField(ct, out, "Created")
		assertHasField(ct, out, "Failed")
		assertHasField(ct, out, "Exited")
	}, 30*time.Second, 2*time.Second)
}

func (s *baseProcmgrSuite) TestCLIListShowsConfiguredProcess() {
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.cliList())
		assertTableRow(ct, out, "test-sleep", map[string]string{
			"STATE":   "Running",
			"COMMAND": s.platform.sleepCommand,
		})
	}, 30*time.Second, 2*time.Second)
}

func (s *baseProcmgrSuite) TestCLIDescribe() {
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.cliDescribe("test-sleep"))
		assertField(ct, out, "Name", "test-sleep")
		assertField(ct, out, "State", "Running")
		assertField(ct, out, "Command", s.platform.sleepCommand)
	}, 30*time.Second, 2*time.Second)
}

// Regression: leftover stop_requested after handle_stop treated the next crash as intentional.
func (s *baseProcmgrSuite) TestCLIStopStartThenKillRestarts() {
	const procName = "test-sleep"

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.cliList())
		assertTableRow(ct, out, procName, map[string]string{"STATE": "Running"})
	}, 30*time.Second, 2*time.Second)

	s.Env().RemoteHost.MustExecute(s.cliStop(procName))
	s.Env().RemoteHost.MustExecute(s.cliStart(procName))

	var pidBeforeKill uint64
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.cliDescribe(procName))
		assertField(ct, out, "State", "Running")
		pidStr := fieldValue(out, "PID")
		require.NotEmpty(ct, pidStr)
		require.NotEqual(ct, "-", pidStr)
		var err error
		pidBeforeKill, err = strconv.ParseUint(pidStr, 10, 32)
		require.NoError(ct, err)
	}, 30*time.Second, 2*time.Second)

	s.Env().RemoteHost.MustExecute(s.platform.killPIDCmd(uint32(pidBeforeKill)))

	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.cliList())
		assertTableRow(ct, out, procName, map[string]string{"STATE": "Running"})
		desc := s.Env().RemoteHost.MustExecuteOn(ct, s.cliDescribe(procName))
		pidAfter := fieldValue(desc, "PID")
		require.NotEmpty(ct, pidAfter)
		require.NotEqual(ct, "-", pidAfter)
		assert.NotEqual(ct, strconv.FormatUint(pidBeforeKill, 10), pidAfter, "PID should change after crash restart")
		restarts := fieldValue(desc, "Restarts")
		require.NotEmpty(ct, restarts)
		restartCount, err := strconv.ParseUint(restarts, 10, 32)
		require.NoError(ct, err)
		assert.GreaterOrEqual(ct, restartCount, uint64(1), "restart count should reflect crash restart")
	}, 30*time.Second, 2*time.Second)
}

func (s *baseProcmgrSuite) TestConditionPathExistsSkipsMissingBinary() {
	require.EventuallyWithT(s.T(), func(ct *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecuteOn(ct, s.cliList())
		assertTableRow(ct, out, "missing-binary", map[string]string{
			"STATE": "Created",
			"PID":   "-",
		})
	}, 30*time.Second, 2*time.Second)
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
