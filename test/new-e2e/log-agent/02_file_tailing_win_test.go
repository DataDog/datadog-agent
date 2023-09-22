// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logAgent

import (
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
)

func (s *vmFakeintakeSuite) TestWindowsLogTailing() {
	windowsConfig :=
		`logs:
  - type: file
    path: 'C:\\logs\\hello-world.log'
    service: hello
    source: custom_log
`
	s.UpdateEnv(e2e.FakeIntakeStackDef([]ec2params.Option{ec2params.WithOS(ec2os.WindowsOS)}, agentparams.WithLogs(), agentparams.WithIntegration("custom_logs.d", windowsConfig)))

	// Clean up any existing log files
	s.cleanUp()
	defer s.cleanUp()

	// Flush server and reset aggregators
	s.Env().Fakeintake.FlushServerAndResetAggregators()
	defer s.Env().Fakeintake.FlushServerAndResetAggregators()

	// // Run test cases
	// s.T().Run("WindowsLogCollection", func(t *testing.T) {
	// 	s.WindowsLogCollection()
	// })

	// s.T().Run("WindowsLogPermission", func(t *testing.T) {
	// 	s.WindowsLogPermission()
	// })

}

func (s *vmFakeintakeSuite) WindowsLogCollection() {
	t := s.T()
	fakeintake := s.Env().Fakeintake

	// Create a new directory
	_, err := s.Env().VM.ExecuteWithError("New-Item -Path C:\\logs -ItemType Directory -Force")
	require.NoError(t, err, "Unable to create the directory 'C:\\logs'.")

	// Create a new log file
	_, err = s.Env().VM.ExecuteWithError("New-Item -Path C:\\logs\\hello-world.log -ItemType file")
	require.NoError(t, err, "Unable to create the log file 'C:\\logs\\hello-world.log'.")

	// Part 1: Ensure no logs are present in fakeintake
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := fakeintake.FilterLogs("hello")
		require.NoError(t, err, "Unable to filter logs by the service 'hello'.")
		require.Empty(t, logs, "Logs were found when none were expected.")

		// If logs are found, print their content for debugging
		if len(logs) != 0 {
			cat, _ := s.Env().VM.ExecuteWithError("Get-Content C:\\logs\\hello-world.log")
			t.Logf("Unexpected logs detected: %v", cat)
			require.Empty(t, logs, "Logs were found when none were expected.")
		}

	}, 5*time.Minute, 2*time.Second)

	// Part 2: Adjust permissions of new log file
	_, err = s.Env().VM.ExecuteWithError("icacls C:\\logs\\hello-world.log /grant *S-1-1-0:F")
	require.NoError(t, err, "Unable to change permissions for the log file 'C:\\logs\\hello-world.log'.")

	// Generate logs
	generateLog(s, t, "hello-world")

	// Part 3: Assert that logs are found in intake after generation
	checkLogs(s, "hello", "hello-world")
}

func (s *vmFakeintakeSuite) WindowsLogPermission() {
	t := s.T()

	// Part 4: Block permission and check the Agent status
	s.Env().VM.Execute("icacls C:\\logs\\hello-world.log /deny *S-1-1-0:F")

	statusCmd := `& "$env:ProgramFiles\Datadog\Datadog Agent\bin\agent.exe" status | ` +
		`Select-String 'custom_logs' -Context 0,10 | ` +
		`ForEach-Object { $_.Line; $_.Context.PostContext }`

	s.EventuallyWithT(func(c *assert.CollectT) {
		// Check the Agent status
		statusOutput, err := s.Env().VM.ExecuteWithError(statusCmd)
		require.NoError(t, err, "Issue running agent status: %s", err)

		if strings.Contains(statusOutput, "Status: OK") {
			require.Fail(t, "log file is unexpectedly accessible")
		}

		require.Contains(t, statusOutput, "permission denied", "Log file is correctly inaccessible")
	}, 3*time.Minute, 10*time.Second)

	// Part 5: Restore permissions
	s.Env().VM.Execute("icacls C:\\logs\\hello-world.log /grant *S-1-1-0:F")

	// Part 6: Restart the agent, generate new logs
	s.Env().VM.Execute("& \"$env:ProgramFiles\\Datadog\\Datadog Agent\\bin\\agent.exe\" restart-service")

	generateLog(s, s.T(), "hello-world")

	// Check the Agent status
	s.EventuallyWithT(func(c *assert.CollectT) {
		statusOutput, err := s.Env().VM.ExecuteWithError(statusCmd)
		require.NoError(t, err, "Issue running agent status: %s", err)
		require.Contains(t, statusOutput, "Status: OK", "Expecting log file to be accessible but it is inaccessible instead")
	}, 5*time.Minute, 2*time.Second)
}

func (s *vmFakeintakeSuite) TestWindowsLogRotation() {
	t := s.T()

	// Part 7: Rotate the log file and check if the agent is tailing the new log file.
	// Rotate the log file
	s.Env().VM.Execute(`Rename-Item -Path "C:\logs\hello-world.log" -NewName "hello-world.log.old"`)
	s.Env().VM.Execute(`New-Item -Path "C:\logs\hello-world.log" -ItemType file`)

	// Verify the old log file's existence after rotation
	_, err := s.Env().VM.ExecuteWithError(`Test-Path "C:\logs\hello-world.log.old"`)
	if err != nil {
		require.NoError(t, err, "Can not find old log file after log rotation")
	}
	// Grant new log file permission
	s.Env().VM.Execute(`icacls C:\\logs\\hello-world.log /grant *S-1-1-0:F`)

	statusCmd := `& \"$env:ProgramFiles\Datadog\Datadog Agent\bin\agent.exe\" status | ` +
		`Select-String 'custom_logs' -Context 0,10 | ` +
		`ForEach-Object { $_.Line; $_.Context.PostContext }`

	// Check if agent is tailing new log file via agent status
	s.EventuallyWithT(func(c *assert.CollectT) {
		newStatusOutput, err := s.Env().VM.ExecuteWithError(statusCmd)
		require.NoErrorf(t, err, "Issue running agent status. Is the agent running?\n %s", newStatusOutput)
		assert.Containsf(t, newStatusOutput, "Path: C:\\logs\\hello-world.log", "The agent is not tailing the expected log file,instead: \n %s", newStatusOutput)
	}, 5*time.Minute, 10*time.Second)

	// Generate new log
	generateLog(s, t, "hello-world-new-content")

	// Verify Log's content is generated and submitted
	checkLogs(s, "hello", "hello-world-new-content")
}
