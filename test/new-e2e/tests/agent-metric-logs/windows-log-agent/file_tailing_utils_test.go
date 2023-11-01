// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package windowslogagent

import (
	"fmt"
	"strings"
	"time"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	commonos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateLog generates and verifies log contents.
func generateLog(s *windowsVMFakeintakeSuite, content string) {
	// Determine the OS and set the appropriate log path and command.
	var logPath, cmd, checkCmd string
	t := s.T()

	osType := s.Env().VM.GetOSType()
	var os string

	switch osType {
	case commonos.WindowsType:
		os = "Windows"
		t.Log("Generating Windows log.")
		logPath = "C:\\logs\\hello-world.log"
		cmd = fmt.Sprintf("echo %s > %s", strings.Repeat(content, 10), logPath)
		checkCmd = fmt.Sprintf("Get-Content %s", logPath)
	default: // Assuming Linux if not Windows.
		os = "Linux"
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
			require.Fail(t, "Log not yet generated.")
		}
	}, 5*time.Minute, 2*time.Second)
}

// checkLogs checks and verifies logs inside the intake.
func checkLogs(suite *windowsVMFakeintakeSuite, service, content string) {
	client := suite.Env().Fakeintake
	t := suite.T()

	suite.EventuallyWithT(func(c *assert.CollectT) {
		names, err := client.GetLogServiceNames()
		assert.NoErrorf(t, err, "Error found: %s", err)

		if len(names) > 0 {
			logs, err := client.FilterLogs(service)
			assert.NoErrorf(t, err, "Error found: %s", err)
			assert.NotEmpty(t, logs, "No logs with service matching '%s' found, instead got '%s'", service, names)

			logs, err = client.FilterLogs(service, fi.WithMessageContaining(content))
			assert.NoErrorf(t, err, "Error found: %s", err)
			assert.True(t, len(logs) > 0, "Expected at least 1 log with content: '%s', but received %v logs.", content, len(logs))
		}
	}, 10*time.Minute, 10*time.Second)

}

// cleanUp cleans up any existing log files (only useful when running dev mode/local runs).
func (s *windowsVMFakeintakeSuite) cleanUp() {
	t := s.T()

	var checkCmd string

	osType := s.Env().VM.GetOSType()
	var os string

	switch osType {
	default:
		s.Env().VM.Execute("sudo rm -f /var/log/hello-world.log")
		s.Env().VM.Execute("sudo rm -f /var/log/hello-world.log.old")
		checkCmd = "ls /var/log/hello-world.log /var/log/hello-world.log.old 2>/dev/null || echo 'Files do not exist'"
		os = "Linux"
	case commonos.WindowsType:
		s.Env().VM.Execute("if (Test-Path C:\\logs\\hello-world.log) { Remove-Item -Path C:\\logs\\hello-world.log -Force }")
		s.Env().VM.Execute("if (Test-Path C:\\logs\\hello-world.log.old) { Remove-Item -Path C:\\logs\\hello-world.log.old -Force }")
		checkCmd = "if (Test-Path C:\\logs\\hello-world.log) { Get-ChildItem -Path C:\\logs\\hello-world.log } elseif (Test-Path C:\\logs\\hello-world.log.old) { Get-ChildItem -Path C:\\logs\\hello-world.log.old } else { Write-Output 'Files do not exist' }"
		os = "Windows"
	}

	s.EventuallyWithT(func(c *assert.CollectT) {
		output, err := s.Env().VM.ExecuteWithError(checkCmd)
		if err != nil {
			require.NoErrorf(t, err, "Having issue cleaning %s log files, retrying... %s", os, output)
		} else {
			t.Logf("Successfully %s cleaned up.", os)
		}
	}, 5*time.Minute, 2*time.Second)
}

// func (s *windowsVMFakeintakeSuite) getOSType() (string, error) {
// 	// Get Linux OS.
// 	output, err := s.Env().VM.ExecuteWithError("cat /etc/os-release")
// 	if err == nil && strings.Contains(output, "ID=ubuntu") {
// 		return "linux", nil
// 	}

// 	// Get Windows OS.
// 	output, err = s.Env().VM.ExecuteWithError("wmic os get Caption")
// 	if err == nil && strings.Contains(output, "Windows") {
// 		return "windows", nil
// 	}

// 	return "", errors.New("unable to determine OS type")
// }
