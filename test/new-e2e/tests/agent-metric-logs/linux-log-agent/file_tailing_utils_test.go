// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logagent

import (
	"fmt"
	"strings"
	"time"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	componentos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
)

// generateLog generates and verifies log contents.
func generateLog(s *LinuxVMFakeintakeSuite, content string) {
	// Determine the OS and set the appropriate log path and command.
	var logPath, cmd, checkCmd string
	t := s.T()

	// Get OS type using the OSType from vm_client.go from e2e/client package
	osType := s.Env().VM.GetOSType()
	var os string

	switch osType {
	case componentos.WindowsType:
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

	s.Env().VM.Execute(cmd)

	// Check if the log has been generated.
	s.EventuallyWithT(func(c *assert.CollectT) {
		output := s.Env().VM.Execute(checkCmd)
		if strings.Contains(output, content) {
			t.Logf("Finished generating %s log.", os)
		} else {
			assert.Fail(c, "Log not yet generated.")
		}
	}, 5*time.Minute, 2*time.Second)
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
			assert.True(c, len(logs) > 0, "Expected at least 1 log with content: '%s', but received %v logs.", content, len(logs))
		}
	}, 10*time.Minute, 10*time.Second)

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
		if err != nil {
			assert.NoErrorf(c, err, "Having issue flushing server and resetting aggregators, retrying...")
		}

		output, err := s.Env().VM.ExecuteWithError(checkCmd)
		if err != nil {
			assert.NoErrorf(c, err, "Having issue cleaning log %s files, retrying... %s", os, output)
		} else {
			t.Log("Successfully cleaned up.")
		}
	}, 5*time.Minute, 2*time.Second)
}
