// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package journaldlog

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-log-pipelines/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

const eventuallyWithTickDuration = 5 * time.Second

// LinuxJournaldFakeintakeSuite defines a test suite for the log agent interacting with a virtual machine and fake intake.
type LinuxJournaldFakeintakeSuite struct {
	e2e.BaseSuite[environments.Host]
}

//go:embed log-config/include.yaml
var logIncludeConfig []byte

//go:embed log-config/python-script.sh
var pythonScript []byte

//go:embed log-config/logger-service.sh
var randomLogger []byte

// TestVMJournaldTailingSuite returns the stack definition required for the log agent test suite.
func TestVMJournaldTailingSuite(t *testing.T) {
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithLogs(),
					agentparams.WithIntegration("custom_logs.d", string(logIncludeConfig))))),
		),
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
	s.Run("journaldIncludeServiceLogCollection()", s.journaldIncludeServiceLogCollection)
}

func (s *LinuxJournaldFakeintakeSuite) journaldIncludeServiceLogCollection() {
	s.UpdateEnv(awshost.Provisioner(awshost.WithRunOptions(
		ec2.WithAgentOptions(
			agentparams.WithLogs(),
			agentparams.WithIntegration("custom_logs.d", string(logIncludeConfig)))),
	))

	vm := s.Env().RemoteHost
	t := s.T()

	// Add dd-agent user to systemd-journal group
	_, err := s.Env().RemoteHost.Execute("sudo usermod -a -G systemd-journal dd-agent")
	require.NoErrorf(t, err, "Unable to adjust permissions for dd-agent user: %s", err)

	// Create journald log generation script
	pythonScript := string(pythonScript)

	_, err = vm.Execute(pythonScript)
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
		agentStatus := s.Env().Agent.Client.Status()
		agentReady := strings.Contains(agentStatus.Content, "random-logger")
		assert.Truef(c, agentReady, "Agent is not ready after restart")
	}, utils.WaitFor, eventuallyWithTickDuration)

	// Check that the agent service log is collected
	utils.CheckLogsExpected(s.T(), s.Env().FakeIntake, "random-logger", "less important", []string{})

	// Disable journald log generation service
	_, err = vm.Execute("sudo systemctl disable --now random-logger.service")
	assert.NoErrorf(t, err, "Failed to disable the logging service: %s", err)
}
