// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logagent

import (
	_ "embed"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	e2e "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

// LinuxVMFakeintakeSuite defines a test suite for the log agent interacting with a virtual machine and fake intake.
type LinuxVMFakeintakeSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
	DevMode bool
}

//go:embed log-config/log-config.yaml
var logConfig []byte

// logsExampleStackDef returns the stack definition required for the log agent test suite.
func logsExampleStackDef() *e2e.StackDefinition[e2e.FakeIntakeEnv] {

	return e2e.FakeIntakeStackDef(
		e2e.WithAgentParams(
			agentparams.WithLogs(),
			agentparams.WithIntegration("custom_logs.d", string(logConfig))))

}

// TestE2EVMFakeintakeSuite runs the E2E test suite for the log agent with a VM and fake intake.
func TestE2EVMFakeintakeSuite(t *testing.T) {
	s := &LinuxVMFakeintakeSuite{}
	if _, devmode := os.LookupEnv("TESTS_E2E_DEVMODE"); devmode {
		s.DevMode = true
	}

	e2e.Run(t, s, logsExampleStackDef())
}

func (s *LinuxVMFakeintakeSuite) BeforeTest(_, _ string) {
	// Flush server and reset aggregators before the test is ran
	s.cleanUp()
}

func (s *LinuxVMFakeintakeSuite) TearDownSuite() {
	// Flush server and reset aggregators after the test is ran
	s.cleanUp()
}

func (s *LinuxVMFakeintakeSuite) TestLinuxLogTailing() {
	// Run test cases
	s.Run("LogCollection", func() {
		s.LogCollection()
	})

	s.Run("LogPermission", func() {
		s.LogPermission()
	})

	s.Run("LogRotation", func() {
		s.LogRotation()
	})
}

func (s *LinuxVMFakeintakeSuite) LogCollection() {
	t := s.T()
	fakeintake := s.Env().Fakeintake

	// Create a new log file
	s.Env().VM.Execute("sudo touch /var/log/hello-world.log")

	// Part 1: Ensure no logs are present in fakeintake
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := fakeintake.FilterLogs("hello")
		if !assert.NoError(c, err, "Unable to filter logs by the service 'hello'.") {
			return
		}
		assert.Empty(c, logs, "Logs were found when none were expected.")

		// If logs are found, print their content for debugging
		if len(logs) != 0 {
			cat, _ := s.Env().VM.ExecuteWithError("cat /var/log/hello-world.log")
			t.Logf("Logs detected when none were expected: %v", cat)
			assert.Empty(c, logs, "Logs were found when none were expected.")
		}
	}, 5*time.Minute, 10*time.Second)

	// Part 2: Adjust permissions of new log file
	_, err := s.Env().VM.ExecuteWithError("sudo chmod 777 /var/log/hello-world.log")
	assert.NoError(t, err, "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")

	// Generate log
	generateLog(s, "hello-world")

	// Part 3: Assert that logs are found in intake after generation
	checkLogs(s, "hello", "hello-world")
}

func (s *LinuxVMFakeintakeSuite) LogPermission() {

	// Part 4: Block permission and check the Agent status
	s.Env().VM.Execute("sudo chmod 000 /var/log/hello-world.log")
	s.EventuallyWithT(func(c *assert.CollectT) {
		// Check the Agent status
		statusOutput, err := s.Env().VM.ExecuteWithError("sudo datadog-agent status | grep -A 10 'custom_logs'")
		if !assert.NoError(c, err, "Issue running agent status: %s", err) {
			return
		}

		if strings.Contains(statusOutput, "Status: OK") {
			assert.Fail(c, "log file is unexpectedly accessible")
			return
		}

		assert.Contains(c, statusOutput, "denied", "Log file is correctly inaccessible")
	}, 3*time.Minute, 10*time.Second)

	// Part 5: Restore permissions
	s.Env().VM.Execute("sudo chmod 777 /var/log/hello-world.log")

	// Part 6: Restart the agent, generate new logs
	s.Env().VM.Execute("sudo service datadog-agent restart")

	// Wait for agent to be ready
	if s.Env().Agent.IsReady() {
		generateLog(s, "hello-world")
	}

	// Check the Agent status
	s.EventuallyWithT(func(c *assert.CollectT) {
		statusOutput, err := s.Env().VM.ExecuteWithError("sudo datadog-agent status | grep -A 10 'custom_logs'")
		assert.NoError(c, err, "Issue running agent status: %s", err)
		assert.Contains(c, statusOutput, "Status: OK", "Expecting log file to be accessible but it is inaccessible instead")
	}, 5*time.Minute, 2*time.Second)
}

func (s *LinuxVMFakeintakeSuite) LogRotation() {
	t := s.T()

	// Part 7: Rotate the log file and check if the agent is tailing the new log file.
	// Rotate the log file
	s.Env().VM.Execute("sudo mv /var/log/hello-world.log /var/log/hello-world.log.old && sudo touch /var/log/hello-world.log")

	// Verify the old log file's existence after rotation
	_, err := s.Env().VM.ExecuteWithError("ls /var/log/hello-world.log.old")
	assert.NoError(t, err, "Failed to find the old log file after rotation")

	// Grant new log file permission
	s.Env().VM.Execute("sudo chmod 777 /var/log/hello-world.log")

	// Check if agent is tailing new log file via agent status
	s.EventuallyWithT(func(c *assert.CollectT) {
		newStatusOutput, err := s.Env().VM.ExecuteWithError("sudo datadog-agent status | grep -A 10 'custom_logs'")
		assert.NoErrorf(t, err, "Issue running agent status. Is the agent running?\n %s", newStatusOutput)
		assert.Containsf(t, newStatusOutput, "Path: /var/log/hello-world.log", "The agent is not tailing the expected log file,instead: \n %s", newStatusOutput)
	}, 5*time.Minute, 10*time.Second)

	// Generate new log
	generateLog(s, "hello-world-new-content")

	// Verify Log's content is generated and submitted
	checkLogs(s, "hello", "hello-world-new-content")
}
