// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils provides shared common functions so different E2E tests suites can use them.
package utils

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"

	"github.com/DataDog/test-infra-definitions/components/os"
)

// LogsTestSuite is an interface for the log agent test suite.
type LogsTestSuite interface {
	T() *testing.T
	Env() *environments.Host
	IsDevMode() bool
}

// AppendLog appen log with 'content', which is then repeated 'reccurrence' times and verifies log contents.
func AppendLog(ls LogsTestSuite, content string, recurrence int) {
	// Determine the OS and set the appropriate log path and command.
	t := ls.T()
	t.Helper()

	var osStr string
	var logPath, cmd, checkCmd string

	logContent := strings.Repeat(content+"\n", recurrence)

	switch ls.Env().RemoteHost.OSFamily {
	case os.WindowsFamily:
		osStr = "windows"
		t.Log("Generating Windows log.")
		logPath = "C:\\logs\\hello-world.log"

		// Unless a log line is newline terminated, the log agent will not pick it up. This is a known behavior.
		logContent = strings.ReplaceAll(logContent, "\n", "\r\n")

		checkCmd = fmt.Sprintf("type %s", logPath)
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			// AppendFile instead of echo since echo introduce encoding into the file.
			bytes, err := ls.Env().RemoteHost.AppendFile(logPath, []byte(logContent))
			if assert.NoErrorf(c, err, "Error writing log: %v", err) {
				t.Logf("Writing %d bytes to %s", bytes, logPath)
			}
		}, 1*time.Minute, 5*time.Second)

	default: // Assuming Linux if not Windows.
		osStr = "linux"
		t.Log("Generating Linux log.")
		logPath = "/var/log/hello-world.log"
		cmd = fmt.Sprintf("echo '%s' | sudo tee -a %s", logContent, logPath)
		checkCmd = fmt.Sprintf("sudo cat %s", logPath)
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			_, err := ls.Env().RemoteHost.Execute(cmd)
			if assert.NoErrorf(c, err, "Having issue generating Linux log with error: %s", err) {
				t.Logf("Writing %s to %s", logContent, logPath)
			}
		}, 1*time.Minute, 5*time.Second)
	}

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		// Verify the log content locally
		output, err := ls.Env().RemoteHost.Execute(checkCmd)
		if err != nil {
			assert.FailNowf(c, "Log content %s not found, instead received:: %s", content, output)
		}
		if strings.Contains(output, content) {
			t.Logf("Finished generating %s log, log file's content is now: \n '%s' \n", osStr, output)
		}
	}, 2*time.Minute, 10*time.Second)
}

// CheckLogFilePresence verifies the presence or absence of a log file path
func CheckLogFilePresence(ls LogsTestSuite, logPath string) {
	t := ls.T()
	t.Helper()

	switch ls.Env().RemoteHost.OSFamily {
	case os.WindowsFamily:
		checkCmd := fmt.Sprintf("Get-Content %s", logPath)
		_, err := ls.Env().RemoteHost.Execute(checkCmd)
		if err != nil {
			assert.FailNow(t, "Log File not found")
		}
	default: // Assuming Linux if not Windows.
		checkCmd := fmt.Sprintf("sudo cat %s", logPath)
		_, err := ls.Env().RemoteHost.Execute(checkCmd)
		if err != nil {
			assert.FailNow(t, "Log File not found")
		}
	}
}

// CheckLogs verifies the presence or absence of logs in the intake based on the expectLogs flag.
func CheckLogs(ls LogsTestSuite, service, content string, expectLogs bool) {
	client := ls.Env().FakeIntake.Client()
	t := ls.T()
	t.Helper()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		names, err := client.GetLogServiceNames()
		if !assert.NoErrorf(c, err, "Error found: %s", err) {
			return
		}
		if assert.NotEmptyf(c, names, "No service %s found", names) {
			logs, err := client.FilterLogs(service)
			if !assert.NoErrorf(c, err, "Error found: %s", err) {
				return
			}
			if !assert.NotEmpty(c, logs, "No logs with service matching '%s' found, instead got '%s'", service, names) {
				return
			}
			logs, err = ls.Env().FakeIntake.Client().FilterLogs(service, fi.WithMessageMatching(content))
			intakeLogs := logsToString(logs)
			assert.NoErrorf(c, err, "Error found: %s", err)
			if expectLogs {
				if assert.NotEmpty(c, logs, "Expected at least 1 log with content: '%s', from service: %s but received %s logs.", content, names, intakeLogs) {
					t.Logf("Logs from service: '%s' with content: '%s' collected", names, content)
				}
			} else {
				if assert.Empty(c, logs, "No logs with content: '%s' is expected to be found from service: %s instead found: %s", content, names, intakeLogs) {
					t.Logf("No logs from service: '%s' with content: '%s' collected as expected", names, content)
				}
			}
		}
	}, 2*time.Minute, 10*time.Second)
}

// CleanUp cleans up any existing log files (only useful when running dev mode/local runs).
func CleanUp(ls LogsTestSuite) {
	t := ls.T()
	t.Helper()
	var checkCmd string

	if ls.IsDevMode() {
		switch ls.Env().RemoteHost.OSFamily {
		default: // default is linux
			ls.Env().RemoteHost.MustExecute("sudo rm -f /var/log/hello-world.log")
			ls.Env().RemoteHost.MustExecute("sudo rm -f /var/log/hello-world.log.old")
			checkCmd = "ls /var/log/hello-world.log /var/log/hello-world.log.old 2>/dev/null || echo 'Files do not exist'"
		case os.WindowsFamily:
			if ls.IsDevMode() {
				// Removing registry.json in DevMode because when the VM is reused, the agent would try to resume the file offset but the tests would truncate the log files.
				t.Logf("Turning off agent")
				_, err := ls.Env().RemoteHost.Execute("& \"$env:ProgramFiles\\Datadog\\Datadog Agent\\bin\\agent.exe\" stopservice")
				require.NoError(t, err, "Unable to stop the agent")

				t.Logf("Removing registry.json")
				err = ls.Env().RemoteHost.RemoveAll("C:\\ProgramData\\Datadog\\run")
				require.NoError(t, err, "Unable to remove agent registry ")

				t.Logf("Turning on agent")
				_, err = ls.Env().RemoteHost.Execute("& \"$env:ProgramFiles\\Datadog\\Datadog Agent\\bin\\agent.exe\" start-service")
				require.NoError(t, err, "Unable to start the agent")
			}
			_, err := ls.Env().RemoteHost.Execute("if (Test-Path C:\\logs) { Remove-Item -Path C:\\logs -Recurse -Force }")
			require.NoError(t, err, "Unable to remove windows log file")

			checkCmd = "if (Test-Path C:\\logs) { Get-ChildItem -Path C:\\logs\\hello-world.log } else { Write-Output 'No File exist to be removed' }"
		}

		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			output, err := ls.Env().RemoteHost.Execute(checkCmd)
			if assert.NoErrorf(c, err, "Having issue cleaning up log files, retrying... %s", output) {
				t.Log("Successfully cleaned up log files.")
			}
		}, 1*time.Minute, 10*time.Second)
	}

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		err := ls.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		if assert.NoErrorf(c, err, "Having issue flushing server and resetting aggregators, retrying...") {
			t.Log("Successfully flushed server and reset aggregators.")
		}
	}, 1*time.Minute, 10*time.Second)
}

// prettyPrintLog pretty prints a log entry.
func prettyPrintLog(log *aggregator.Log) string {
	// Unmarshal and re-marshal the message field for pretty printing
	var messageObj map[string]interface{}
	if err := json.Unmarshal([]byte(log.Message), &messageObj); err == nil {
		prettyMessage, _ := json.MarshalIndent(messageObj, "", "  ")
		log.Message = string(prettyMessage)
	}
	// Marshal the entire log entry
	logStr, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		// Handle the error appropriately
		return fmt.Sprintf("Error marshaling log: %v", err)
	}
	return string(logStr)
}

// logsToString converts a slice of logs to a string.
func logsToString(logs []*aggregator.Log) string {
	var logsStrings []string
	for _, log := range logs {
		logsStrings = append(logsStrings, prettyPrintLog(log))
	}
	return fmt.Sprintf("[%s]", strings.Join(logsStrings, ",\n"))
}
