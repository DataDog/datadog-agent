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
	_, s.DevMode = os.LookupEnv("E2E_DEVMODE")

	e2e.Run(t, s, logsExampleStackDef())
}

func (s *LinuxVMFakeintakeSuite) BeforeTest(_, _ string) {
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

func (s *LinuxVMFakeintakeSuite) TearDownSuite() {
	// Flush server and reset aggregators after the test is ran
	s.cleanUp()
}

func (s *LinuxVMFakeintakeSuite) TestLinuxLogTailing() {
	// Run test cases

	// Test log file collection with permissions granted before and after log generation
	s.Run("LogCollection", s.LogCollection)

	// Test log file collection with permission revoked and granted before log generation
	s.Run("LogPermission", s.LogPermission)

	// Test log file collection with log file delete and recreate rotation
	s.Run("LogRecreateRotation", s.LogRecreateRotation)
}

func (s *LinuxVMFakeintakeSuite) LogCollection() {
	t := s.T()
	// fakeintake := s.Env().Fakeintake

	// Create a new log file with permissionn inaccessible to the agent
	s.Env().VM.Execute("sudo touch /var/log/hello-world.log")

	// Adjust permissions of new log file before log generation
	output, err := s.Env().VM.ExecuteWithError("sudo chmod +r /var/log/hello-world.log && echo true")

	assert.NoError(t, err, "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")

	assert.Equal(t, "true", strings.TrimSpace(output), "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")

	t.Logf("Permissions granted for new log file.")
	// Generate log
	generateLog(s, "hello-before-permission-world", 1)

	// Check intake for new logs
	checkLogs(s, "hello", "hello-before-permission-world", true)

	//  Adjust permissions of new log file after log generation

	// Restore permissions to default and generate log
	output, err = s.Env().VM.ExecuteWithError("sudo chmod 644 /var/log/hello-world.log && echo true")

	assert.NoErrorf(t, err, "Unable to adjust back to default permissions, err: %s.", err)

	assert.Equal(t, "true", strings.TrimSpace(output), "Unable to adjust back to default permissions for the log file '/var/log/hello-world.log'.")

	t.Logf("Permissions reset to default.")
	generateLog(s, "hello-after-permission-world", 1)

	// Grant log file permission and check intake for new logs
	output, err = s.Env().VM.ExecuteWithError("sudo chmod +r /var/log/hello-world.log && echo true")

	assert.NoError(t, err, "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")

	assert.Equal(t, "true", strings.TrimSpace(output), "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")

	t.Logf("Permissions granted for log file.")

	checkLogs(s, "hello", "hello-after-permission-world", true)
}

func (s *LinuxVMFakeintakeSuite) LogPermission() {
	t := s.T()
	logPath := "/var/log/hello-world.log"
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

	generateLog(s, "access-denied", 1)

	// Check intake to see if new logs are not present
	checkLogs(s, "hello", "access-denied", false)

	// Restore permissions
	output, err = s.Env().VM.ExecuteWithError("sudo chmod +r /var/log/hello-world.log && echo true")

	assert.NoError(t, err, "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")

	assert.Equal(t, "true", strings.TrimSpace(output), "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")

	// Generate new logs and check the intake for new logs
	t.Logf("Permissions restored.")

	// Wait for the agent to tail the log file since there is a delay between permissions being granted and the agent tailing the log file
	time.Sleep(1000 * time.Millisecond)

	generateLog(s, "access-granted", 1)

	// Check intake to see if new logs are present
	checkLogs(s, "hello", "access-granted", true)

}

func (s *LinuxVMFakeintakeSuite) LogRecreateRotation() {
	t := s.T()
	logPath := "/var/log/hello-world.log"
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
	generateLog(s, "hello-world-new-content", 1)

	// Check intake for new logs
	checkLogs(s, "hello", "hello-world-new-content", true)

}
