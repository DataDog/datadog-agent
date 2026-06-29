// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package journaldlog

import (
	_ "embed"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-metric-logs/log-agent/utils"
)

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

// TestE2EVMFakeintakeSuite returns the stack definition required for the log agent test suite.
func TestE2EVMFakeintakeSuite(t *testing.T) {
	devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithAgentOptions(
				agentparams.WithLogs(),
				agentparams.WithIntegration("custom_logs.d", string(logIncludeConfig))))),
	}
	if devMode, err := strconv.ParseBool(devModeEnv); err == nil && devMode {
		options = append(options, e2e.WithDevMode())
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
	vm := s.Env().RemoteHost
	t := s.T()

	// Add dd-agent user to systemd-journal group
	_, err := vm.Execute("sudo usermod -a -G systemd-journal dd-agent")
	assert.NoErrorf(t, err, "Unable to adjust permissions for dd-agent user: %s", err)

	// Create journald log generation script
	pythonScriptContent := string(pythonScript)

	_, err = vm.Execute(pythonScriptContent)
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

	// Wait for the agent to be ready and the journald tailer to be active
	s.EventuallyWithT(func(c *assert.CollectT) {
		agentStatus := s.Env().Agent.Client.Status()
		if assert.Truef(c, strings.Contains(agentStatus.Content, "random-logger"), "Agent journald tailer not yet active") {
			// Check that the agent service log is collected
			utils.CheckLogsExpected(s, "random-logger", "less important")
		}
	}, 2*time.Minute, 10*time.Second)

	// Disable journald log generation service
	_, err = vm.Execute("sudo systemctl disable --now random-logger.service")
	assert.NoErrorf(t, err, "Failed to disable the logging service: %s", err)
}
