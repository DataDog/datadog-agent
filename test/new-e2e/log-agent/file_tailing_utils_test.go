package logAgent

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateLog generates and verify log contents
func generateLog(s *vmFakeintakeSuite, t *testing.T, content string) {

	// Determine the OS and set the appropriate log path and command
	var logPath string
	var cmd string
	var checkCmd string

	osType, err := s.getOSType()
	if err != nil {
		require.NoErrorf(t, err, "%s", err)
	}
	if osType == "windows" {
		t.Log("Generating windows Log")
		logPath = "C:\\logs\\hello-world.log"
		cmd = fmt.Sprintf("echo %s > %s", strings.Repeat(content, 10), logPath)
		checkCmd = fmt.Sprintf("Get-Content %s", logPath)

	} else {
		t.Log("Generating linux Log")
		logPath = "/var/log/hello-world.log"
		cmd = fmt.Sprintf("echo %s > %s", strings.Repeat(content, 10), logPath)
		checkCmd = fmt.Sprintf("cat %s", logPath)
	}

	s.Env().VM.Execute(cmd)

	// This part check to see if log has been generated
	s.EventuallyWithT(func(c *assert.CollectT) {
		output := s.Env().VM.Execute(checkCmd)
		if strings.Contains(output, content) {
			t.Logf("Finished generating %s log", osType)
		}
		require.NoErrorf(t, err, "log not yet generated")
	}, 5*time.Minute, 2*time.Second)
}

// cleanUp cleans up any existing log files
func (s *vmFakeintakeSuite) cleanUp() {
	t := s.T()
	osType, err := s.getOSType()
	if err != nil {
		s.T().Logf("Failed to determine OS type: %v", err)
		return
	}

	var checkCmd string

	switch osType {
	case "linux":
		s.Env().VM.Execute("sudo rm -f /var/log/hello-world.log")
		s.Env().VM.Execute("sudo rm -f /var/log/hello-world.log.old")
		checkCmd = "ls /var/log/hello-world.log /var/log/hello-world.log.old 2>/dev/null || echo 'Files do not exist'"
	case "windows":
		s.Env().VM.Execute("if (Test-Path C:\\logs\\hello-world.log) { Remove-Item -Path C:\\logs\\hello-world.log -Force }")
		s.Env().VM.Execute("if (Test-Path C:\\logs\\hello-world.log.old) { Remove-Item -Path C:\\logs\\hello-world.log.old -Force }")
		checkCmd = "if (Test-Path C:\\logs\\hello-world.log) { Get-ChildItem -Path C:\\logs\\hello-world.log } elseif (Test-Path C:\\logs\\hello-world.log.old) { Get-ChildItem -Path C:\\logs\\hello-world.log.old } else { Write-Output 'Files do not exist' }"
	default:
		s.T().Logf("Unsupported OS type: %s", osType)
		return
	}

	s.EventuallyWithT(func(c *assert.CollectT) {
		output, err := s.Env().VM.ExecuteWithError(checkCmd)
		if err != nil {
			require.NoErrorf(t, err, "Having issue cleaning log files, retrying... %s", output)
		} else {
			s.T().Log("Sucessfully cleaning up")
		}
	}, 5*time.Minute, 2*time.Second)
}

// checkLogs checks and verifies logs inside the intake
func checkLogs(fakeintake *vmFakeintakeSuite, service, content string) {
	client := fakeintake.Env().Fakeintake
	t := fakeintake.T()

	fakeintake.EventuallyWithT(func(c *assert.CollectT) {
		names, err := client.GetLogServiceNames()
		if err != nil {
			require.NoErrorf(t, err, "found error %s", err)
		}
		if len(names) == 0 {
			require.NoErrorf(t, err, "no logs found in intake service")
		}
		logs, err := client.FilterLogs(service)
		if err != nil {
			require.NoErrorf(t, err, "found error %s", err)
		}
		if len(logs) < 1 {
			require.NoErrorf(t, err, "no logs with service matching '%s' found, instead got '%s'", service, names)
		}
		logs, err = client.FilterLogs(service, fi.WithMessageContaining(content))
		if err != nil {
			require.NoErrorf(t, err, "found error %s", err)
		}
		if len(logs) != 1 {
			require.NoErrorf(t, err, "received %v logs with '%s', expecting 1", len(logs), content)
		}
	}, 5*time.Minute, 2*time.Second)
}

func (s *vmFakeintakeSuite) getOSType() (string, error) {

	// Get Linux OS
	output, err := s.Env().VM.ExecuteWithError("cat /etc/os-release")
	if err == nil && strings.Contains(output, "ID=ubuntu") {
		return "linux", nil
	}

	// Get Windows OS
	output, err = s.Env().VM.ExecuteWithError("wmic os get Caption")
	if err == nil && strings.Contains(output, "Windows") {
		return "windows", nil
	}

	return "", errors.New("unable to determine OS type")
}
