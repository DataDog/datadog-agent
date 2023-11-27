// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package journaldlog

import (
	_ "embed"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

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

//go:embed log-config/log-config2.yaml
var logConfig2 []byte

//go:embed log-config/log-config3.yaml
var logConfig3 []byte

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

	e2e.Run(t, &LinuxVMFakeintakeSuite{}, logsExampleStackDef(), params.WithDevMode())
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
	// Part 1: Add dd-agent user to systemd-journal group
	_, err := s.Env().VM.ExecuteWithError("sudo usermod -a -G systemd-journal dd-agent")
	require.NoError(s.T(), err, "Unable to adjust permissions for dd-agent user.")

	// Restart agent
	s.Env().VM.Execute("sudo systemctl restart datadog-agent")

	// Generate log
	generateLog(s, "hello-world", "journald")

	// Part 2: require.NoError t,are found in intake after generation
	checkLogs(s, "hello", "hello-world")
}

func (s *LinuxVMFakeintakeSuite) journaldIncludeServiceLogCollection() {
	s.UpdateEnv(e2e.FakeIntakeStackDef(
		e2e.WithAgentParams(
			agentparams.WithLogs(),
			agentparams.WithIntegration("custom_logs.d", string(logConfig2)))))

	vm := s.Env().VM
	t := s.T()

	python_script := `sudo bash -c 'cat > /usr/bin/random-logger.py << EOF
#!/usr/bin/env python3

import logging
import random
from time import sleep

logging.basicConfig(format="%(asctime)s | %(levelname)s | %(message)s", level=logging.DEBUG)

while True:
    logging.info("This is less important than debug log and is often used to provide context in the current task.")
    sleep(random.uniform(0, 5))
EOF'
`
	_, err := vm.ExecuteWithError(python_script)
	require.NoError(t, err, "Failed to generate journald log generation script ")
	_, err = vm.ExecuteWithError("sudo chmod 777 /usr/bin/random-logger.py")
	require.NoError(t, err, "Failed to change permission for journald script")

	logger_service := `sudo bash -c 'cat > /etc/systemd/system/random-logger.service << EOF
[Unit]
Description=Random logger

[Service]
ExecStart=/usr/bin/random-logger.py

[Install]
WantedBy=multi-user.target
EOF'
`

	_, err = vm.ExecuteWithError(logger_service)
	require.NoError(t, err, "Failed to create journald log generation service ")

	_, err = vm.ExecuteWithError("sudo chmod 777 /etc/systemd/system/random-logger.service")
	require.NoError(t, err, "Failed to change permission for service")

	_, err = vm.ExecuteWithError("sudo systemctl daemon-reload")
	require.NoError(t, err, "Failed to load journald service")

	_, err = vm.ExecuteWithError("sudo systemctl enable --now random-logger.service")
	require.NoError(t, err, "Failed to enable journaald service")

	_, err = vm.ExecuteWithError("sudo service datadog-agent restart")
	require.NoError(t, err, "Failed to restart the agent")

	checkLogs(s, "random-logger.py", "less important")

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

	// Check that the agent service log is not collected
	checkExcludeLog(s, "not-datadog", "running check")
}
