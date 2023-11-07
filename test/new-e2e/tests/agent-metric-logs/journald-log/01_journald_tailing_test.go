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

	e2e.Run(t, &LinuxVMFakeintakeSuite{}, logsExampleStackDef())
}

func (s *LinuxVMFakeintakeSuite) BeforeTest(_, _ string) {
	// Flush server and reset aggregators before the test is ran
	if s.DevMode {
		s.cleanUp()
	}
	err := s.Env().Fakeintake.FlushServerAndResetAggregators()
	require.NoError(s.T(), err, "Unable to flush server and reset aggregators: %s", err)
}

func (s *LinuxVMFakeintakeSuite) TearDownSuite() {
	// Flush server and reset aggregators after the test is ran
	if s.DevMode {
		s.cleanUp()
	}
	err := s.Env().Fakeintake.FlushServerAndResetAggregators()
	require.NoError(s.T(), err, "Unable to flush server and reset aggregators: %s", err)
}

func (s *LinuxVMFakeintakeSuite) TestLinuxLogTailing() {
	// Run test cases
	s.Run("journaldLogCollection", func() {
		s.journaldLogCollection()
	})

}

func (s *LinuxVMFakeintakeSuite) journaldLogCollection() {
	t := s.T()
	fakeintake := s.Env().Fakeintake
	// Part 1: Ensure no logs are present in fakeintake
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := fakeintake.FilterLogs("hello")
		if !assert.NoError(c, err, "Unable to filter logs by the service 'hello'.") {
			return
		}
		assert.Emptyf(c, logs, "Logs were found when none were expected: %v", logs)
	}, 5*time.Minute, 10*time.Second)

	// Part 2: Add dd-agent user to systemd-journal group
	_, err := s.Env().VM.ExecuteWithError("sudo usermod -a -G systemd-journal dd-agent")
	require.NoError(t, err, "Unable to adjust permissions for dd-agent user.")

	// Restart agent
	s.Env().VM.Execute("sudo systemctl restart datadog-agent")

	// Generate log
	generateLog(s, "hello-world", "journald")

	// Part 3: Assert that logs are found in intake after generation
	checkLogs(s, "hello", "hello-world")
}
