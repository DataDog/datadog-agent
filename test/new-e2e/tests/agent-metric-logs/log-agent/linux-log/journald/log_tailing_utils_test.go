// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package journaldlog

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	componentos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateLog generates and verifies log contents.
func generateLog(s *LinuxVMFakeintakeSuite, content, contentType string) {
	// Determine the OS and set the appropriate log path and command.
	var logPath, cmd, checkCmd string
	t := s.T()

	// Get OS type using the OSType from vm_client.go from e2e/client package
	osType := s.Env().VM.GetOSType()
	var os string

	// Generate journald log
	if contentType == "journald" {
		if osType != componentos.LinuxFamily {
			os = "linux"
			t.Log("Generating Linux Journald log.")
			cmd = fmt.Sprintf("echo %s | systemd-cat", strings.Repeat(content, 5))
			checkCmd = fmt.Sprintf("journalctl --since '1 minute ago' | grep '%s'", content)
		} else {
			t.Logf("Journald logs are only supported on Linux. Skipping for OS: %v", osType)
			return
		}
	} else {
		// Generate log contents for log files
		switch osType {
		case componentos.WindowsFamily:
			os = "windows"
			t.Log("Generating Windows log.")
			logPath = "C:\\logs\\hello-world.log"
			cmd = fmt.Sprintf("echo %s > %s", strings.Repeat(content, 10), logPath)
			checkCmd = fmt.Sprintf("Get-Content %s", logPath)
		default: // Assuming Linux if not Windows.
			os = "linux"
			t.Log("Generating Linux log.")
			logPath = "/var/log/hello-world.log"
			cmd = fmt.Sprintf("echo %s > %s", strings.Repeat(content, 10), logPath)
			checkCmd = fmt.Sprintf("cat %s", logPath)
		}
	}

	_, err := s.Env().VM.ExecuteWithError(cmd)
	require.NoErrorf(t, err, "Error found: %s", err)

	// Check if the log has been generated.
	s.EventuallyWithT(func(c *assert.CollectT) {
		output := s.Env().VM.Execute(checkCmd)
		if strings.Contains(output, content) {
			t.Logf("Finished generating %s log.", os)
		} else {
			assert.Fail(c, "Log not yet generated.")
		}
	}, 2*time.Minute, 1*time.Second)
}

// checkLogs checks and verifies logs inside the intake.
func checkLogs(suite *LinuxVMFakeintakeSuite, service, content string) {
	client := suite.Env().Fakeintake

	suite.EventuallyWithT(func(c *assert.CollectT) {
		names, err := client.GetLogServiceNames()
		if !assert.NoErrorf(c, err, "Error found: %s", err) {
			return
		}

		if len(names) > 0 {
			logs, err := client.FilterLogs(service)
			if !assert.NoErrorf(c, err, "Error found: %s", err) {
				return
			}
			if !assert.NotEmpty(c, logs, "No logs with service matching '%s' found, instead got '%s'", service, names) {
				return
			}

			logs, err = client.FilterLogs(service, fi.WithMessageContaining(content))
			assert.NoErrorf(c, err, "Error found: %s", err)
			assert.NotEmpty(c, logs, "Expected at least 1 log with content: '%s', but received %v logs.", content, logs)
		}
	}, 2*time.Minute, 1*time.Second)

}

// checkExcludeLog checks and verifies excluded logs is not inside the intake.
func checkExcludeLog(suite *LinuxVMFakeintakeSuite, service, content string) {
	client := suite.Env().Fakeintake

	suite.EventuallyWithT(func(c *assert.CollectT) {
		names, err := client.GetLogServiceNames()
		if !assert.NoErrorf(c, err, "Error found: %s", err) {
			return
		}

		if len(names) > 0 {
			logs, err := client.FilterLogs(service)
			if !assert.NoErrorf(c, err, "Error found: %s", err) {
				return
			}
			if !assert.NotEmpty(c, logs, "No logs with service matching '%s' found, instead got '%s'", service, names) {
				return
			}

			logs, err = client.FilterLogs(service, fi.WithMessageContaining(content))
			logcContent := logsToString(logs)
			assert.NoErrorf(c, err, "Error found: %s", err)
			assert.NotContainsf(c, logs, content, "Expected no log with content: '%s', but received %s logs.", content, logcContent)
		}
	}, 2*time.Minute, 1*time.Second)
}

// cleanUp cleans up any existing log files (only useful when running dev mode/local runs).
func (s *LinuxVMFakeintakeSuite) cleanUp() {
	t := s.T()
	osType := s.Env().VM.GetOSType()
	var os string

	var checkCmd string

	switch osType {
	default: // default is linux
		os = "linux"
		s.Env().VM.Execute("sudo rm -f /var/log/hello-world.log")
		s.Env().VM.Execute("sudo rm -f /var/log/hello-world.log.old")
		checkCmd = "ls /var/log/hello-world.log /var/log/hello-world.log.old 2>/dev/null || echo 'Files do not exist'"
	case componentos.WindowsType:
		os = "windows"
		s.Env().VM.Execute("if (Test-Path C:\\logs\\hello-world.log) { Remove-Item -Path C:\\logs\\hello-world.log -Force }")
		s.Env().VM.Execute("if (Test-Path C:\\logs\\hello-world.log.old) { Remove-Item -Path C:\\logs\\hello-world.log.old -Force }")
		checkCmd = "if (Test-Path C:\\logs\\hello-world.log) { Get-ChildItem -Path C:\\logs\\hello-world.log } elseif (Test-Path C:\\logs\\hello-world.log.old) { Get-ChildItem -Path C:\\logs\\hello-world.log.old } else { Write-Output 'Files do not exist' }"
	}

	s.EventuallyWithT(func(c *assert.CollectT) {
		err := s.Env().Fakeintake.FlushServerAndResetAggregators()
		if assert.NoErrorf(c, err, "Having issue flushing server and resetting aggregators, retrying...") {
			t.Log("Successfully cleaned up pipeline")
			time.Sleep(10 * time.Second)
		}

		output, err := s.Env().VM.ExecuteWithError(checkCmd)
		if assert.NoErrorf(c, err, "Having issue cleaning log %s files, retrying... %s", os, output) {
			t.Log("Successfully cleaned up log files.")
		}
	}, 2*time.Minute, 1*time.Second)
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
