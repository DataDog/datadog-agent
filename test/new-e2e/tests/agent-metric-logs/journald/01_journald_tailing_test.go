// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package journaldlog

import (
	_ "embed"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e2e "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

// LinuxVMFakeintakeSuite defines a test suite for the log agent interacting with a virtual machine and fake intake.
type LinuxVMFakeintakeSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
	DevMode bool
}

//go:embed log-config/journald-log-config.yaml
var logConfig []byte

//go:embed log-config/journald-include-log-config.yaml
var logConfig2 []byte

//go:embed log-config/journald-exclude-log-config.yaml
var logConfig3 []byte

//go:embed log-config/python-log-script.sh
var pythonScript []byte

//go:embed log-config/random-logger-service.sh
var randomLogger []byte

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

	e2e.Run(t, &LinuxVMFakeintakeSuite{}, logsExampleStackDef())
}

func (s *LinuxVMFakeintakeSuite) BeforeTest(_, _ string) {
	// Flush server and reset aggregators before the test is ran
	s.cleanUp()
}

func (s *LinuxVMFakeintakeSuite) TearDownSuite() {
	// Flush server and reset aggregators after the test is ran
	s.cleanUp()
}

func (s *LinuxVMFakeintakeSuite) TestJournald() {
	// Run test cases
	s.Run("journaldLogCollection", s.journaldLogCollection)

	s.Run("journaldIncludeServiceLogCollection()", s.journaldIncludeServiceLogCollection)

	s.Run("journaldExcludeServiceCollection()", s.journaldExcludeServiceCollection)
}

func (s *LinuxVMFakeintakeSuite) journaldLogCollection() {

	fakeintake := s.Env().Fakeintake
	// Part 1: Ensure no logs are present in fakeintake
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := fakeintake.FilterLogs("hello")
		if !assert.NoError(c, err, "Unable to filter logs by the service 'hello'.") {
			return
		}
		assert.Emptyf(c, logs, "Logs were found when none were expected: %v", logs)
	}, 2*time.Minute, 1*time.Second)

	// Part 2: Add dd-agent user to systemd-journal group
	_, err := s.Env().VM.ExecuteWithError("sudo usermod -a -G systemd-journal dd-agent")
	require.NoError(s.T(), err, "Unable to adjust permissions for dd-agent user.")

	// Restart agent
	s.Env().VM.Execute("sudo systemctl restart datadog-agent")

	// Generate log
	generateLog(s, "hello-world", "journald")

	// Part 2: check that the generated log is collected
	checkLogs(s, "hello", "hello-world")
}

func (s *LinuxVMFakeintakeSuite) journaldIncludeServiceLogCollection() {
	s.UpdateEnv(e2e.FakeIntakeStackDef(
		e2e.WithAgentParams(
			agentparams.WithLogs(),
			agentparams.WithIntegration("custom_logs.d", string(logConfig2)))))

	vm := s.Env().VM
	t := s.T()

	// Create journald log generation script
	python_script := string(pythonScript)
	_, err := vm.ExecuteWithError(python_script)
	require.NoError(t, err, "Failed to generate journald log generation script ")

	// Create journald log generation service
	logger_service := string(randomLogger)
	_, err = vm.ExecuteWithError(logger_service)
	require.NoError(t, err, "Failed to create journald log generation service ")

	// Enable journald log generation service
	_, err = vm.ExecuteWithError("sudo systemctl daemon-reload")
	require.NoError(t, err, "Failed to load journald service")

	// Start journald log generation service
	_, err = vm.ExecuteWithError("sudo systemctl enable --now random-logger.service")
	require.NoError(t, err, "Failed to enable journaald service")

	// Restart agent
	_, err = vm.ExecuteWithError("sudo service datadog-agent restart")
	require.NoError(t, err, "Failed to restart the agent")

	// Check that the agent service log is collected
	checkLogs(s, "random-logger.py", "less important")

	// Disable journald log generation service
	_, err = vm.ExecuteWithError("sudo systemctl disable --now random-logger.service")
	require.NoError(t, err, "Failed to disable the logging service")
}

func (s *LinuxVMFakeintakeSuite) journaldExcludeServiceCollection() {
	s.UpdateEnv(e2e.FakeIntakeStackDef(
		e2e.WithAgentParams(
			agentparams.WithLogs(),
			agentparams.WithIntegration("custom_logs.d", string(logConfig3)))))

	// Restart agent
	s.Env().VM.Execute("sudo systemctl restart datadog-agent")

	// Check that the datadog-agent.service log is not collected
	checkExcludeLog(s, "not-datadog", "running check")
}
