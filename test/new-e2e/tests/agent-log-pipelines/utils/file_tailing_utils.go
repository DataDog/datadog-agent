// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils provides shared common functions so different E2E tests suites can use them.
package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	agentclient "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

// LinuxLogsFolderPath is the folder where log files will be stored for Linux tests
const LinuxLogsFolderPath = "/var/log/e2e_test_logs"

// WindowsLogsFolderPath is the folder where log files will be stored for Windows tests
const WindowsLogsFolderPath = "C:\\logs\\e2e_test_logs"

type ddtags []string

const (
	// WaitFor Time to wait for file tailing utils
	WaitFor = 2 * time.Minute
	// Tick how many seconds each ticket should take
	Tick = 10 * time.Second
)

// LogsTestSuite is an interface for the log agent test suite.
type LogsTestSuite interface {
	T() *testing.T
	Env() *environments.Host
	IsDevMode() bool
}

// AppendLog append log with 'content', which is then repeated 'reccurrence' times and verifies log contents.
func AppendLog(ls LogsTestSuite, logFileName, content string, recurrence int) {
	// Determine the OS and set the appropriate log path and command.
	var checkCmd, logPath string
	t := ls.T()
	t.Helper()

	var osStr string

	// Unless a log line is newline terminated, the log agent will not pick it up,
	logContent := strings.Repeat(content+"\n", recurrence)

	switch ls.Env().RemoteHost.OSFamily {
	case os.WindowsFamily:
		osStr = "windows"
		t.Log("Generating Windows log.")
		//  Windows uses \r\n for newlines instead of \n.
		logContent = strings.ReplaceAll(logContent, "\n", "\r\n")
		logPath = fmt.Sprintf("%s\\%s", WindowsLogsFolderPath, logFileName)
		t.Logf("Log path: %s", logPath)

		checkCmd = "type " + logPath
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			// AppendFile instead of echo since echo introduce encoding into the file.
			bytes, err := ls.Env().RemoteHost.AppendFile(osStr, logPath, []byte(logContent))
			if assert.NoErrorf(c, err, "Error writing log: %v", err) {
				t.Logf("Writing %d bytes to %s", bytes, logPath)
			}
		}, 1*time.Minute, 5*time.Second)

	default: // Assuming Linux if not Windows.
		osStr = "linux"
		t.Log("Generating Linux log.")
		logPath = fmt.Sprintf("%s/%s", LinuxLogsFolderPath, logFileName)
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			bytes, err := ls.Env().RemoteHost.AppendFile(osStr, logPath, []byte(logContent))
			if assert.NoErrorf(c, err, "Error writing log: %v", err) {
				t.Logf("Writing %d bytes to %s", bytes, logPath)
			}
		}, 1*time.Minute, 5*time.Second)
		checkCmd = "sudo cat " + logPath
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
	}, WaitFor, Tick)
}

// TailerState represents the state of a log tailer
type TailerState string

const (
	// TailerStateOK indicates the tailer is running without errors
	TailerStateOK TailerState = "OK"
	// TailerStateError indicates the tailer is in an error state
	TailerStateError TailerState = "Error"
)

// AgentStatus represents the agent status JSON structure
type AgentStatus struct {
	LogsStats LogsStats `json:"logsStats"`
}

// LogsStats represents the logs statistics section
type LogsStats struct {
	Integrations []Integration `json:"integrations"`
}

// Integration represents a log integration
type Integration struct {
	Name    string   `json:"name"`
	Sources []Source `json:"sources"`
}

// Source represents a log source
type Source struct {
	Type          string                 `json:"type"`
	Configuration map[string]interface{} `json:"configuration"`
	Status        string                 `json:"status"`
	Inputs        []string               `json:"inputs"`
}

// getTailerStatus extracts the tailer status for a given log file from the agent status JSON.
// Returns the status string and whether the tailer was found.
func getTailerStatus(statusJSON, logFileName string) (string, bool) {
	var status AgentStatus
	if err := json.Unmarshal([]byte(statusJSON), &status); err != nil {
		return "", false
	}

	for _, integration := range status.LogsStats.Integrations {
		for _, source := range integration.Sources {
			if source.Type != "file" {
				continue
			}
			if path, ok := source.Configuration["Path"].(string); ok {
				if strings.Contains(path, logFileName) {
					return source.Status, true
				}
			}
		}
	}

	return "", false
}

// AssertAgentTailerState asserts that a tailer for the given log file is in the expected state.
// expectedState can be TailerStateOK or TailerStateError.
func AssertAgentTailerState(ls LogsTestSuite, logFileName string, expectedState TailerState) {
	t := ls.T()
	t.Helper()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		statusObj, err := ls.Env().Agent.Client.StatusWithError(agentclient.WithArgs([]string{"--json"}))
		if !assert.NoErrorf(c, err, "Failed to get agent status: %v", err) {
			return
		}

		statusContent := statusObj.Content
		status, found := getTailerStatus(statusContent, logFileName)

		if !assert.Truef(c, found, "Tailer for log file %s not found in agent status", logFileName) {
			return
		}

		switch expectedState {
		case TailerStateOK:
			assert.Equalf(c, "OK", status, "Expected tailer for %s to be in OK state, but got: %s", logFileName, status)
		case TailerStateError:
			assert.Truef(c, strings.HasPrefix(status, "Error"), "Expected tailer for %s to be in Error state, but got: %s", logFileName, status)
		}
	}, 2*time.Minute, 5*time.Second)
}

// AssertAgentTailerStarted asserts that a tailer for the given log file exists in the agent status.
func AssertAgentTailerStarted(ls LogsTestSuite, logFileName string) {
	t := ls.T()
	t.Helper()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		statusObj, err := ls.Env().Agent.Client.StatusWithError(agentclient.WithArgs([]string{"--json"}))
		if !assert.NoErrorf(c, err, "Failed to get agent status: %v", err) {
			return
		}

		statusContent := statusObj.Content
		status, found := getTailerStatus(statusContent, logFileName)

		if assert.Truef(c, found, "Tailer for log file %s not found in agent status", logFileName) {
			t.Logf("Tailer for %s found with status: %s", logFileName, status)
		}
	}, 2*time.Minute, 5*time.Second)
}

// AssertAgentTailerOK asserts that a tailer for the given log file is in OK state.
func AssertAgentTailerOK(ls LogsTestSuite, logFileName string) {
	AssertAgentTailerState(ls, logFileName, TailerStateOK)
}

// AssertAgentTailerError asserts that a tailer for the given log file is in Error state.
func AssertAgentTailerError(ls LogsTestSuite, logFileName string) {
	AssertAgentTailerState(ls, logFileName, TailerStateError)
}

// CheckLogFilePresence verifies the presence or absence of a log file path
func CheckLogFilePresence(ls LogsTestSuite, logFileName string) {
	t := ls.T()
	t.Helper()

	switch ls.Env().RemoteHost.OSFamily {
	case os.WindowsFamily:
		checkCmd := fmt.Sprintf("Get-Content %s\\%s", WindowsLogsFolderPath, logFileName)
		_, err := ls.Env().RemoteHost.Execute(checkCmd)
		if err != nil {
			assert.FailNow(t, "Log File not found")
		}
	default: // Assuming Linux if not Windows.
		checkCmd := fmt.Sprintf("sudo cat %s/%s", LinuxLogsFolderPath, logFileName)
		_, err := ls.Env().RemoteHost.Execute(checkCmd)
		if err != nil {
			assert.FailNow(t, "Log File not found")
		}
	}
}

// FetchAndFilterLogs fetches logs from the fake intake server and filters them by service and content.
func FetchAndFilterLogs(fakeIntake *components.FakeIntake, service, content string) ([]*aggregator.Log, error) {
	client := fakeIntake.Client()

	names, err := client.GetLogServiceNames()
	if err != nil {
		return nil, err
	}

	if len(names) == 0 {
		return nil, errors.New("the fake intake has no logs for any services")
	}

	var contains bool
	for _, v := range names {
		if v == service {
			contains = true
		}
	}
	if !contains {
		return nil,
			fmt.Errorf("the fake intake has no logs for service '%s'. Only found logs for the following services %v",
				service, names)
	}

	logs, err := client.FilterLogs(service, fi.WithMessageMatching(content))
	if err != nil {
		return nil, err
	}
	return logs, nil
}

// CheckLogsExpected verifies the presence of expected logs, and verifies that there are no duplicate tags.
func CheckLogsExpected(t *testing.T, fakeIntake *components.FakeIntake, service, content string, expectedTags ddtags) {
	t.Helper()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		logs, err := FetchAndFilterLogs(fakeIntake, service, content)

		if assert.NoErrorf(c, err, "Error fetching logs: %s", err) {
			intakeLog := logsToString(logs)
			if assert.NotEmpty(c, logs, "Expected logs with content: '%s' not found. Instead, found: %s", content, intakeLog) {
				t.Logf("Logs from service: '%s' with content: '%s' collected", service, content)
				log := logs[0]
				// Use a map to check for duplicate tags
				seenTags := make(map[string]struct{})
				for _, tag := range log.Tags {
					if _, exists := seenTags[tag]; exists {
						t.Errorf("Duplicate tag found: %s", tag)
					}
					seenTags[tag] = struct{}{} // Mark the tag as seen
				}
				for _, expectedTag := range expectedTags {
					assert.Contains(c, log.Tags, expectedTag)
				}
			}
		}
	}, WaitFor, Tick)
}

// CheckLogsNotExpected verifies the absence of unexpected logs.
func CheckLogsNotExpected(t *testing.T, fakeIntake *components.FakeIntake, service, content string) {
	t.Helper()
	t.Logf("Checking for logs from service: '%s' with content: '%s' are not collected", service, content)
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		logs, err := FetchAndFilterLogs(fakeIntake, service, content)
		if assert.NoErrorf(c, err, "Error fetching logs: %s", err) {
			intakeLog := logsToString(logs)
			if assert.Empty(c, logs, "Unexpected logs with content: '%s' found. Instead, found: %s", content, intakeLog) {
				t.Logf("No logs from service: '%s' with content: '%s' collected as expected", service, content)
			}
		}
	}, WaitFor, Tick)
}

// CleanUp cleans up any existing log files (only useful when running dev mode/local runs).
func CleanUp(ls LogsTestSuite) {
	t := ls.T()
	t.Helper()
	var checkCmd string

	if ls.IsDevMode() {
		switch ls.Env().RemoteHost.OSFamily {
		default: // default is linux
			ls.Env().RemoteHost.MustExecute("sudo rm -rf " + LinuxLogsFolderPath)
			checkCmd = fmt.Sprintf("ls %s 2>/dev/null || echo 'Files do not exist'", LinuxLogsFolderPath)
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
			_, err := ls.Env().RemoteHost.Execute(fmt.Sprintf("if (Test-Path %s) { Remove-Item -Path %s -Recurse -Force }", WindowsLogsFolderPath, WindowsLogsFolderPath))
			require.NoError(t, err, "Unable to remove windows log file")

			checkCmd = fmt.Sprintf("if (Test-Path %s) { Get-ChildItem -Path %s } else { Write-Output 'No File exist to be removed' }", WindowsLogsFolderPath, WindowsLogsFolderPath)
		}

		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			output, err := ls.Env().RemoteHost.Execute(checkCmd)
			if assert.NoErrorf(c, err, "Having issue cleaning up log files, retrying... %s", output) {
				t.Log("Successfully cleaned up log files.")
			}
		}, 1*time.Minute, Tick)
	}

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		err := ls.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		if assert.NoErrorf(c, err, "Having issue flushing server and resetting aggregators, retrying...") {
			t.Log("Successfully flushed server and reset aggregators.")
		}
	}, 1*time.Minute, Tick)
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
