// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package journaldlog

import (
	_ "embed"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metrics-logs/log-agent/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

const eventuallyWithTickDuration = 5 * time.Second

// LinuxJournaldFakeintakeSuite defines a test suite for the log agent interacting with a virtual machine and fake intake.
type LinuxJournaldFakeintakeSuite struct {
	e2e.BaseSuite[environments.Host]
}

//go:embed log-config/journald.yaml
var logBasicConfig []byte

//go:embed log-config/include.yaml
var logIncludeConfig []byte

//go:embed log-config/exclude.yaml
var logExcludeConfig []byte

//go:embed log-config/python-script.sh
var pythonScript []byte

//go:embed log-config/logger-service.sh
var randomLogger []byte

// TestE2EVMFakeintakeSuite returns the stack definition required for the log agent test suite.
func TestE2EVMFakeintakeSuite(t *testing.T) {
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithAgentOptions(
				agentparams.WithLogs(),
				agentparams.WithIntegration("custom_logs.d", string(logBasicConfig))))),
	}

	e2e.Run(t, &LinuxJournaldFakeintakeSuite{}, options...)
}

func (s *LinuxJournaldFakeintakeSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	// Flush server and reset aggregators before the test is ran
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
}

func (s *LinuxJournaldFakeintakeSuite) TearDownSuite() {
	// Flush server and reset aggregators after the test is ran
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.BaseSuite.TearDownSuite()
}

func (s *LinuxJournaldFakeintakeSuite) TestJournald() {
	// Run test cases
	s.Run("journaldLogCollection", s.journaldLogCollection)

	s.Run("journaldIncludeServiceLogCollection()", s.journaldIncludeServiceLogCollection)

	s.Run("journaldExcludeServiceCollection()", s.journaldExcludeServiceCollection)
}

func (s *LinuxJournaldFakeintakeSuite) journaldLogCollection() {

	t := s.T()
	// Add dd-agent user to systemd-journal group
	_, err := s.Env().RemoteHost.Execute("sudo usermod -a -G systemd-journal dd-agent")
	require.NoErrorf(t, err, "Unable to adjust permissions for dd-agent user: %s", err)

	// Restart agent and make sure it's ready before adding logs
	_, err = s.Env().RemoteHost.Execute("sudo systemctl restart datadog-agent")
	assert.NoErrorf(t, err, "Failed to restart the agent: %s", err)
	s.EventuallyWithT(func(t *assert.CollectT) {
		agentReady := s.Env().Agent.Client.IsReady()
		assert.True(t, agentReady)
	}, utils.WaitFor, eventuallyWithTickDuration, "Agent was not ready")

	// Generate log
	appendJournaldLog(s, "hello-world", 1)

	// Check that the generated log is collected
	utils.CheckLogsExpected(s.T(), s.Env().FakeIntake, "hello", "hello-world", []string{})
}

func (s *LinuxJournaldFakeintakeSuite) journaldIncludeServiceLogCollection() {
	s.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(
		agentparams.WithLogs(),
		agentparams.WithIntegration("custom_logs.d", string(logIncludeConfig)))))

	vm := s.Env().RemoteHost
	t := s.T()

	// Create journald log generation script
	pythonScript := string(pythonScript)

	_, err := vm.Execute(pythonScript)
	if assert.NoErrorf(t, err, "Failed to create python script that generate journald log: %s", err) {
		t.Log("Successfully created python script for journald log generation")
	}

	// Create journald log generation service
	loggerService := string(randomLogger)
	_, err = vm.Execute(loggerService)
	if assert.NoErrorf(t, err, "Failed to create journald log service: %s ", err) {
		t.Log("Successfully created journald log service")
	}

	// Enable journald log generation service
	_, err = vm.Execute("sudo systemctl daemon-reload")
	if assert.NoErrorf(t, err, "Failed to load journald service: %s", err) {
		t.Log("Successfully loaded journald service")
	}

	// Start journald log generation service
	_, err = vm.Execute("sudo systemctl enable --now random-logger.service")
	if assert.NoErrorf(t, err, "Failed to enable journald service: %s", err) {
		t.Log("Successfully enabled journald service")
	}

	// Restart agent
	_, err = vm.Execute("sudo service datadog-agent restart")
	assert.NoErrorf(t, err, "Failed to restart the agent: %s", err)

	s.EventuallyWithT(func(c *assert.CollectT) {
		agentReady := s.Env().Agent.Client.IsReady()
		assert.Truef(c, agentReady, "Agent is not ready after restart")
	}, utils.WaitFor, eventuallyWithTickDuration)

	// Check that the agent service log is collected
	utils.CheckLogsExpected(s.T(), s.Env().FakeIntake, "random-logger", "less important", []string{})

	// Disable journald log generation service
	_, err = vm.Execute("sudo systemctl disable --now random-logger.service")
	assert.NoErrorf(t, err, "Failed to disable the logging service: %s", err)
}

func (s *LinuxJournaldFakeintakeSuite) journaldExcludeServiceCollection() {
	s.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(
		agentparams.WithLogs(),
		agentparams.WithIntegration("custom_logs.d", string(logExcludeConfig)))))

	// Restart agent
	s.Env().RemoteHost.Execute("sudo systemctl restart datadog-agent")

	s.EventuallyWithT(func(c *assert.CollectT) {
		agentReady := s.Env().Agent.Client.IsReady()
		assert.Truef(c, agentReady, "Agent is not ready after restart")
	}, utils.WaitFor, eventuallyWithTickDuration)

	// Check that the datadog-agent.service log is not collected, specifically logs from the check runners
	utils.CheckLogsNotExpected(s.T(), s.Env().FakeIntake, "no-datadog", "running check")
}

// appendJournaldLog appends a log to journald.
func appendJournaldLog(s *LinuxJournaldFakeintakeSuite, content string, recurrence int) {
	t := s.T()
	t.Helper()

	logContent := strings.Repeat(content+"\n", recurrence)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		cmd := fmt.Sprintf("sudo printf '%s' | systemd-cat", logContent)
		_, err := s.Env().RemoteHost.Execute(cmd)
		if assert.NoErrorf(c, err, "Error writing log: %v", err) {
			t.Log("Writing logs to journald")
		}

		checkCmd := fmt.Sprintf("sudo journalctl --since '1 minute ago' | grep '%s'", content)
		output, err := s.Env().RemoteHost.Execute(checkCmd)
		assert.NoErrorf(c, err, "Error found checking for journald logs: %s", err)

		if assert.Contains(c, output, content, "Journald log not properly generated.") {
			t.Log("Finished generating journald log.")
		}
	}, utils.WaitFor, eventuallyWithTickDuration)
}
