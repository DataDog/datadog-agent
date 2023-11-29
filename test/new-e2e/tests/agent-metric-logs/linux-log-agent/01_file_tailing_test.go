// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logagent

import (
	_ "embed"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	e2e "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
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
	_, s.DevMode = os.LookupEnv("TESTS_E2E_DEVMODE")

	e2e.Run(t, s, logsExampleStackDef(), params.WithDevMode())
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
	s.Run("LogCollection", s.LogCollection)

	s.Run("LogPermission", s.LogPermission)

	s.Run("LogRotation", s.LogRotation)
}

func (s *LinuxVMFakeintakeSuite) LogCollection() {
	t := s.T()
	fakeintake := s.Env().Fakeintake

	// Create a new log file with permissionn inaccessable to the agent
	s.Env().VM.Execute("sudo touch /var/log/hello-world.log")

	// Part 1: Ensure no logs are present in fakeintake
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := fakeintake.FilterLogs("hello")
		if !assert.NoError(c, err, "Unable to filter logs by the service 'hello'.") {
			return
		}
		// If logs are found, print their content for debugging
		if !assert.Empty(c, logs, "Logs were found when none were expected.") {
			cat, _ := s.Env().VM.ExecuteWithError("cat /var/log/hello-world.log")
			t.Logf("Logs detected when none were expected: %v", cat)
		}
	}, 5*time.Minute, 10*time.Second)

	// Part 2: Adjust permissions of new log file
	_, err := s.Env().VM.ExecuteWithError("sudo chmod 777 /var/log/hello-world.log")
	assert.NoError(t, err, "Unable to adjust permissions for the log file '/var/log/hello-world.log'.")

	// Generate log
	generateLog(s, "hello-world")

	// Part 3: Check intake for new logs
	checkLogs(s, "hello", "hello-world")
}

func (s *LinuxVMFakeintakeSuite) LogPermission() {

	// Part 4: Allow on only write permission to the log file so the agent cannot tail it
	s.Env().VM.Execute("sudo chmod 666 /var/log/hello-world.log")

	generateLog(s, "access-denied")

	// Check intake to see if new logs are not present
	checkLogs(s, "hello", "access-denied")

	// Part 5: Restore permissions
	s.Env().VM.Execute("sudo chmod 777 /var/log/hello-world.log")

	// Part 6: Generate new logs and check the intake for new logs
	generateLog(s, "access-granted")
	checkLogs(s, "hello", "access-granted")
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

	// Generate new logs
	generateLog(s, "hello-world-new-content")

	// Check intake for new logs
	checkLogs(s, "hello", "hello-world-new-content")
}
