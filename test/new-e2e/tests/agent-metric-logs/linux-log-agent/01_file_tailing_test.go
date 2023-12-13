// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logagent

import (
	_ "embed"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

// LinuxFakeintakeSuite defines a test suite for the log agent interacting with a virtual machine and fake intake.
type LinuxFakeintakeSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
	DevMode bool
}

//go:embed log-config/log-config.yaml
var logConfig string

var logPath = "/var/log/hello-world.log"

// logsExampleStackDef returns the stack definition required for the log agent test suite.
func logsExampleStackDef() *e2e.StackDefinition[e2e.FakeIntakeEnv] {
	return e2e.FakeIntakeStackDef(
		e2e.WithAgentParams(
			agentparams.WithLogs(),
			agentparams.WithIntegration("custom_logs.d", logConfig)))

}

// TestE2EVMFakeintakeSuite runs the E2E test suite for the log agent with a VM and fake intake.
func TestE2EVMFakeintakeSuite(t *testing.T) {
	s := &LinuxFakeintakeSuite{}
	devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")
	options := []params.Option{}
	if devMode, err := strconv.ParseBool(devModeEnv); err == nil && devMode {
		options = append(options, params.WithDevMode())
	}
	e2e.Run(t, s, logsExampleStackDef(), options...)
}

func (s *LinuxFakeintakeSuite) AfterTest(suiteName, testName string) {
	s.Suite.AfterTest(suiteName, testName)
	// Flush server and reset aggregators before the test is ran
	s.cleanUp()

	// Ensure no logs are present in fakeintake before testing starts
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().Fakeintake.FilterLogs("hello")
		if !assert.NoError(c, err, "Unable to filter logs by the service 'hello'.") {
			return
		}
		// If logs are found, print their content for debugging
		if !assert.Empty(c, logs, "Logs were found when none were expected.") {
			cat, _ := s.Env().VM.ExecuteWithError("cat /var/log/hello-world.log && cat /var/log/hello-world-2.log")
			s.T().Logf("Logs detected when none were expected: %v", cat)
		}
	}, 2*time.Minute, 10*time.Second)
}

func (s *LinuxFakeintakeSuite) TearDownSuite() {
	// Flush server and reset aggregators after the test is ran
	s.cleanUp()
	s.Suite.TearDownSuite()
}

func (s *LinuxFakeintakeSuite) TestLinuxLogTailing() {
	// Run test cases:

	// Given the agent configured to collect logs from a log file.
	// When new log line is generated inside the log file.
	// Then the agent collects the log line and forward it to the intake.
	s.Run("LogCollection", s.LogCollection)

	// Given the agent configured to collect logs from a log file that has no read permissions,
	// When new log line is generated inside the log file.
	// Then the agent fail to collects the log line.
	s.Run("LogCollectionNoPermission", s.LogNoPermission)

	// Given the agent configured to collect logs from a log file with reading permissions,
	// When new log line is generated inside the log file.
	// Then the agent collects the log line and forward it to the intake.
	s.Run("LogCollectionAfterPermission", s.LogCollectionAfterPermission)

	// Given the agent configured to collect logs from a log file without reading permissions and new log line actively generating,
	// When read permission is granted
	// Then the agent collects the log line and forward it to the intake.
	s.Run("LogCollectionBeforePermission", s.LogCollectionBeforePermission)

	// Given the agent configured to collect logs from a specific log file
	// When the log file is rotated
	// Then the agent successfully collects the log line
	s.Run("LogRecreateRotation", s.LogRecreateRotation)
}

func (s *LinuxFakeintakeSuite) LogCollection() {
	t := s.T()

	// Create a new log file with permissionn inaccessible to the agent
	s.Env().VM.Execute("sudo touch /var/log/hello-world.log")

	// Adjust permissions of new log file before log generation
	output, err := s.Env().VM.ExecuteWithError("sudo chmod +r /var/log/hello-world.log && echo true")

	assert.NoError(t, err, "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")

	assert.Equal(t, "true", strings.TrimSpace(output), "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")

	// t.Logf("Permissions granted for new log file.")
	// Generate log
	appendLog(s, "hello-world", 1)

	// Check intake for new logs
	checkLogs(s, "hello", "hello-world", true)

}

func (s *LinuxFakeintakeSuite) LogNoPermission() {
	t := s.T()
	checkLogFilePresence(s, logPath)

	// Allow on only write permission to the log file so the agent cannot tail it
	output, err := s.Env().VM.ExecuteWithError("sudo chmod -r /var/log/hello-world.log && echo true")
	assert.NoError(t, err, "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")
	assert.Equal(t, "true", strings.TrimSpace(output), "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")
	t.Logf("Read permissions revoked")

	// In Linux, file permissions are checked at the time of file opening, not during subsequent read or write operations
	// => If the agent has already successfully opened a file for reading, it can continue to read from that file even if the read permissions are later removed
	// => Restart the agent to force it to reopen the file
	s.Env().VM.Execute("sudo service datadog-agent restart")

	// Generate logs and check the intake for no new logs because of revoked permissions
	s.EventuallyWithT(func(c *assert.CollectT) {
		agentReady := s.Env().Agent.IsReady()
		if assert.Truef(c, agentReady, "Agent is not ready after restart") {
			// Generate log
			appendLog(s, "access-denied", 1)
			// Check intake for new logs
			checkLogs(s, "hello", "access-denied", false)
		}
	}, 2*time.Minute, 1*time.Second)

}

func (s *LinuxFakeintakeSuite) LogCollectionAfterPermission() {
	t := s.T()
	checkLogFilePresence(s, logPath)

	// Generate logs
	appendLog(s, "hello-after-permission-world", 1)

	// Grant read permission
	output, err := s.Env().VM.ExecuteWithError("sudo chmod +r /var/log/hello-world.log && echo true")
	assert.NoError(t, err, "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")
	assert.Equal(t, "true", strings.TrimSpace(output), "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")
	t.Logf("Permissions granted for log file.")

	// Check intake for new logs
	checkLogs(s, "hello", "hello-after-permission-world", true)
}

func (s *LinuxFakeintakeSuite) LogCollectionBeforePermission() {
	t := s.T()
	checkLogFilePresence(s, logPath)

	// Reset log file permissions to default before testing
	output, err := s.Env().VM.ExecuteWithError("sudo chmod 644 /var/log/hello-world.log && echo true")
	assert.NoErrorf(t, err, "Unable to adjust back to default permissions, err: %s.", err)
	assert.Equal(t, "true", strings.TrimSpace(output), "Unable to adjust back to default permissions for the log file '/var/log/hello-world.log'.")
	t.Logf("Permissions reset to default.")

	// Grant read permission
	output, err = s.Env().VM.ExecuteWithError("sudo chmod +r /var/log/hello-world.log && echo true")
	assert.NoError(t, err, "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")
	assert.Equal(t, "true", strings.TrimSpace(output), "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")
	t.Logf("Permissions granted.")

	// Wait for the agent to tail the log file since there is a delay between permissions being granted and the agent tailing the log file
	time.Sleep(1000 * time.Millisecond)

	// Generate logs
	appendLog(s, "access-granted", 1)

	// Check intake for new logs
	checkLogs(s, "hello", "access-granted", true)
}

func (s *LinuxFakeintakeSuite) LogRecreateRotation() {
	t := s.T()
	checkLogFilePresence(s, logPath)

	// Rotate the log file and check if the agent is tailing the new log file.
	// Delete and Recreate file rotation
	output, err := s.Env().VM.ExecuteWithError("umask 022 && echo true")
	assert.NoError(t, err, "Failed to set umask")

	s.Env().VM.Execute("sudo mv /var/log/hello-world.log /var/log/hello-world.log.old && sudo touch /var/log/hello-world.log")

	// Verify the old log file's existence after rotation
	_, err = s.Env().VM.ExecuteWithError("ls /var/log/hello-world.log.old")
	assert.NoError(t, err, "Failed to find the old log file after rotation")

	assert.Equal(t, "true", strings.TrimSpace(output), "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")

	t.Logf("Permissions granted for new log file.")

	// Generate new logs
	appendLog(s, "hello-world-new-content", 1)

	// Check intake for new logs
	checkLogs(s, "hello", "hello-world-new-content", true)

}
