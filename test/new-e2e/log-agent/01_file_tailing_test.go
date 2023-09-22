// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logAgent

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
)

// vmFakeintakeSuite defines a test suite for the log agent interacting with a virtual machine and fake intake.
type vmFakeintakeSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

// logsExampleStackDef returns the stack definition required for the log agent test suite.
func logsExampleStackDef(vmParams []ec2params.Option, agentParams ...agentparams.Option) *e2e.StackDefinition[e2e.FakeIntakeEnv] {
	config :=
		`logs:
  - type: file
    path: '/var/log/hello-world.log'
    service: hello
    source: custom_log
`
	return e2e.FakeIntakeStackDef(nil, agentparams.WithLogs(), agentparams.WithIntegration("custom_logs.d", config))

}

// TestE2EVMFakeintakeSuite runs the E2E test suite for the log agent with a VM and fake intake.
func TestE2EVMFakeintakeSuite(t *testing.T) {
	e2e.Run(t, &vmFakeintakeSuite{}, logsExampleStackDef(nil), params.WithDevMode())
}

func (s *vmFakeintakeSuite) TestLinuxLogTailing() {
	// Clean up once test is finished running
	s.cleanUp()
	defer s.cleanUp()

	// Flush server and reset aggregators
	s.Env().Fakeintake.FlushServerAndResetAggregators()
	defer s.Env().Fakeintake.FlushServerAndResetAggregators()

	// Run test cases
	s.T().Run("LogCollection", func(t *testing.T) {
		s.LogCollection()
	})

	s.T().Run("LogPermission", func(t *testing.T) {
		s.LogPermission()
	})

	s.T().Run("LogRotation", func(t *testing.T) {
		s.LogRotation()
	})
}

func (s *vmFakeintakeSuite) LogCollection() {
	t := s.T()
	fakeintake := s.Env().Fakeintake

	// Create a new log file
	s.Env().VM.Execute("sudo touch /var/log/hello-world.log")

	// Part 1: Ensure no logs are present in fakeintake
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := fakeintake.FilterLogs("hello")
		require.NoError(t, err, "Unable to filter logs by the service 'hello'.")
		require.Empty(t, logs, "Logs were found when none were expected.")

		// If logs are found, print their content for debugging
		if len(logs) != 0 {
			cat, _ := s.Env().VM.ExecuteWithError("cat /var/log/hello-world.log")
			t.Logf("Logs detected when none were expected: %v", cat)
			require.Empty(t, logs, "Logs were found when none were expected.")
		}
	}, 5*time.Minute, 2*time.Second)

	// Part 2: Adjust permissions of new log file
	_, err := s.Env().VM.ExecuteWithError("sudo chmod 777 /var/log/hello-world.log")
	require.NoError(t, err, "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")

	// Generate log
	generateLog(s, t, "hello-world")

	// Part 3: Assert that logs are found in intake after generation
	checkLogs(s, "hello", "hello-world")
}

func (s *vmFakeintakeSuite) LogPermission() {
	t := s.T()

	// Part 4: Block permission and check the Agent status
	s.Env().VM.Execute("sudo chmod 000 /var/log/hello-world.log")
	s.EventuallyWithT(func(c *assert.CollectT) {
		// Check the Agent status
		statusOutput, err := s.Env().VM.ExecuteWithError("sudo datadog-agent status | grep -A 10 'custom_logs'")
		require.NoError(t, err, "Issue running agent status: %s", err)

		if strings.Contains(statusOutput, "Status: OK") {
			require.Fail(t, "log file is unexpectedly accessible")
		}

		require.Contains(t, statusOutput, "permission denied", "Log file is correctly inaccessible")
	}, 3*time.Minute, 10*time.Second)

	// Part 5: Restore permissions
	s.Env().VM.Execute("sudo chmod 777 /var/log/hello-world.log")

	// Part 6: Restart the agent, generate new logs
	s.Env().VM.Execute("sudo service datadog-agent restart")

	generateLog(s, s.T(), "hello-world")

	// Check the Agent status
	s.EventuallyWithT(func(c *assert.CollectT) {
		statusOutput, err := s.Env().VM.ExecuteWithError("sudo datadog-agent status | grep -A 10 'custom_logs'")
		require.NoError(t, err, "Issue running agent status: %s", err)
		require.Contains(t, statusOutput, "Status: OK", "Expecting log file to be accessible but it is inaccessible instead")
	}, 5*time.Minute, 2*time.Second)
}

func (s *vmFakeintakeSuite) LogRotation() {
	t := s.T()

	// Part 7: Rotate the log file and check if the agent is tailing the new log file.
	// Rotate the log file
	s.Env().VM.Execute("sudo mv /var/log/hello-world.log /var/log/hello-world.log.old && sudo touch /var/log/hello-world.log")

	// Verify the old log file's existence after rotation
	_, err := s.Env().VM.ExecuteWithError("ls /var/log/hello-world.log.old")
	require.NoError(t, err, "Failed to find the old log file after rotation")

	// Grant new log file permission
	s.Env().VM.Execute("sudo chmod 777 /var/log/hello-world.log")

	// Check if agent is tailing new log file via agent status
	s.EventuallyWithT(func(c *assert.CollectT) {
		newStatusOutput, err := s.Env().VM.ExecuteWithError("sudo datadog-agent status | grep -A 10 'custom_logs'")
		require.NoErrorf(t, err, "Issue running agent status. Is the agent running?\n %s", newStatusOutput)
		assert.Containsf(t, newStatusOutput, "Path: /var/log/hello-world.log", "The agent is not tailing the expected log file,instead: \n %s", newStatusOutput)
	}, 5*time.Minute, 10*time.Second)

	// Generate new log
	generateLog(s, t, "hello-world-new-content")

	// Verify Log's content is generated and submitted
	checkLogs(s, "hello", "hello-world-new-content")
}
