// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package windowsfiletailing

import (
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metric-logs/log-agent/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	testos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
)

// WindowsFakeintakeSuite defines a test suite for the log agent interacting with a virtual machine and fake intake.
type WindowsFakeintakeSuite struct {
	e2e.BaseSuite[environments.Host]
}

//go:embed log-config/config.yaml
var logConfig string

var logPath = "C:\\logs\\hello-world.log"

// TestE2EVMFakeintakeSuite runs the E2E test suite for the log agent with a VM and fake intake.
func TestE2EVMFakeintakeSuite(t *testing.T) {
	s := &WindowsFakeintakeSuite{}
	// devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithEC2InstanceOptions(ec2.WithOS(testos.WindowsDefault)),
			awshost.WithAgentOptions(
				agentparams.WithVersion("7.50.3"),
				agentparams.WithAgentConfig("log_level: debug"),
				agentparams.WithLogs(),
				agentparams.WithIntegration("custom_logs.d", logConfig)))),
	}

	// if devMode, err := strconv.ParseBool(devModeEnv); err == nil && devMode {
	options = append(options, e2e.WithDevMode())
	// }
	e2e.Run(t, s, options...)
}

func (s *WindowsFakeintakeSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	// Flush server and reset aggregators before the test is ran
	utils.CleanUp(s)

	// Ensure no logs are present in fakeintake before testing starts
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().FilterLogs("hello")
		if !assert.NoError(c, err, "Unable to filter logs by the service 'hello'.") {
			return
		}
		// If logs are found, print their content for debugging
		if !assert.Empty(c, logs, "Logs were found when none were expected") {
			cat, _ := s.Env().RemoteHost.Execute("type C:\\logs\\hello-world.log")
			s.T().Logf("Logs detected when none were expected: %v", cat)
		}
	}, 2*time.Minute, 10*time.Second)
}

func (s *WindowsFakeintakeSuite) TearDownSuite() {
	// Flush server and reset aggregators after the test is ran
	utils.CleanUp(s)

	s.BaseSuite.TearDownSuite()
}

func (s *WindowsFakeintakeSuite) TestWindowsLogTailing() {
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
	// s.Run("LogCollectionBeforePermission", s.LogCollectionBeforePermission)
	// Given the agent configured to collect logs from a specific log file
	// When the log file is rotated
	// Then the agent successfully collects the log line
	// s.Run("LogRecreateRotation", s.LogRecreateRotation)
}
func (s *WindowsFakeintakeSuite) LogCollection() {
	t := s.T()
	// Create a new log directory
	_, err := s.Env().RemoteHost.Execute("New-Item -Path C:\\logs -ItemType Directory -Force")
	require.NoError(t, err, "Unable to create a new log directory.")

	// Create a new log file
	_, err = s.Env().RemoteHost.Execute("New-Item -Path C:\\logs\\hello-world.log -ItemType File -Force")
	require.NoError(t, err, "Unable to create a new log file.")

	// Adjust permissions of new log file before log generation
	_, err = s.Env().RemoteHost.Execute("icacls C:\\logs\\hello-world.log /grant ddagentuser:R")
	assert.NoError(t, err, "Unable to adjust permissions for the log file 'C:\\logs\\hello-world.log '.")

	t.Logf("Permissions granted for new log file.")

	// Generate log
	utils.AppendLog(s, "hello-world", 1)

	// Check intake for new logs
	utils.CheckLogs(s, "hello", "hello-world", true)

}

func (s *WindowsFakeintakeSuite) LogNoPermission() {
	t := s.T()
	utils.CheckLogFilePresence(s, logPath)

	// Revoke read permission from ddagentuser to the ls
	_, err := s.Env().RemoteHost.Execute("icacls C:\\logs\\hello-world.log /deny ddagentuser:R")
	assert.NoError(t, err, "Unable to adjust permissions for the log file 'C:\\logs\\hello-world.log'")
	t.Logf("Read permissions revoked")

	// Generate logs and check the intake for no new logs because of revoked permissions
	s.EventuallyWithT(func(c *assert.CollectT) {
		agentReady := s.Env().Agent.Client.IsReady()
		if assert.Truef(c, agentReady, "Agent is not ready after restart") {
			// Generate log
			utils.AppendLog(s, "access-denied", 1)
			// Check intake for new logs
			utils.CheckLogs(s, "hello", "access-denied", false)
		}
	}, 2*time.Minute, 5*time.Second)

}

func (s *WindowsFakeintakeSuite) LogCollectionAfterPermission() {
	t := s.T()
	utils.CheckLogFilePresence(s, logPath)

	// Generate logs
	utils.AppendLog(s, "hello-after-permission-world", 1)

	// Grant read permission
	_, err := s.Env().RemoteHost.Execute("icacls C:\\logs\\hello-world.log /grant ddagentuser:R")
	assert.NoError(t, err, "Unable to adjust permissions for the log file 'C:\\logs\\hello-world.log '.")
	t.Logf("Permissions granted for log file.")

	// Check intake for new logs
	utils.CheckLogs(s, "hello", "hello-after-permission-world", true)
}

func (s *WindowsFakeintakeSuite) LogCollectionBeforePermission() {
	t := s.T()
	utils.CheckLogFilePresence(s, logPath)

	// Reset log file permissions to default before testing
	_, err := s.Env().RemoteHost.Execute("icacls C:\\logs\\hello-world.log /reset")
	assert.NoErrorf(t, err, "Unable to adjust back to default permissions, err: %s.", err)
	t.Logf("Permissions reset to default.")

	// Grant read permission
	_, err = s.Env().RemoteHost.Execute("icacls C:\\logs\\hello-world.log /grant ddagentuser:R")
	assert.NoError(t, err, "Unable to adjust permissions for the log file 'C:\\logs\\hello-world.log '.")
	t.Logf("Permissions granted.")
	// Wait for the agent to tail the log file since there is a delay between permissions being granted and the agent tailing the log file
	time.Sleep(10000 * time.Millisecond)

	// Generate logs
	utils.AppendLog(s, "access-granted", 1)

	// Check intake for new logs
	utils.CheckLogs(s, "hello", "access-granted", true)
}

func (s *WindowsFakeintakeSuite) LogRecreateRotation() {
	t := s.T()
	utils.CheckLogFilePresence(s, logPath)

	// Rotate the log file and check if the agent is tailing the new log file.
	// Delete and Recreate file rotation
	s.Env().RemoteHost.Execute("Rename-Item -Path C:\\logs\\hello-world.log -NewName hello-world.log.old ")
	s.Env().RemoteHost.Execute("New-Item -Path C:\\logs\\hello-world.log -ItemType File ")

	// Verify the old log file's existence after rotation
	_, err := s.Env().RemoteHost.Execute("ls C:\\logs\\hello-world.log.old")
	assert.NoError(t, err, "Failed to find the old log file after rotation")

	// Adjust permissions of new log file before log generation
	_, err = s.Env().RemoteHost.Execute("icacls C:\\logs\\hello-world.log /grant ddagentuser:R")
	assert.NoError(t, err, "Unable to adjust permissions for the log file 'C:\\logs\\hello-world.log '.")
	t.Logf("Permissions granted for new log file.")

	// Generate new logs
	utils.AppendLog(s, "hello-world-new-content", 1)

	// Check intake for new logs
	utils.CheckLogs(s, "hello", "hello-world-new-content", true)

}
