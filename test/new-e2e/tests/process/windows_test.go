// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
)

type windowsTestSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func TestWindowsTestSuite(t *testing.T) {
	e2e.Run(t, &windowsTestSuite{},
		e2e.FakeIntakeStackDef(
			e2e.WithAgentParams(agentparams.WithAgentConfig(processCheckConfigStr)),
			e2e.WithVMParams(ec2params.WithOS(ec2os.WindowsOS)),
		))
}

func (s *windowsTestSuite) SetupSuite() {
	s.Suite.SetupSuite()
	// Start an antivirus scan to use as process for testing
	s.Env().VM.Execute("Start-MpScan -ScanType FullScan -AsJob")
}

func (s *windowsTestSuite) TestProcessCheck() {
	t := s.T()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		command := "& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe\" status --json"
		assertRunningChecks(collect, s.Env().VM, []string{"process", "rtprocess"}, false, command)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().Fakeintake.GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestProcessDiscoveryCheck() {
	s.UpdateEnv(e2e.FakeIntakeStackDef(
		e2e.WithAgentParams(agentparams.WithAgentConfig(processDiscoveryCheckConfigStr)),
		e2e.WithVMParams(ec2params.WithOS(ec2os.WindowsOS)),
	))

	t := s.T()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		command := "& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe\" status --json"
		assertRunningChecks(collect, s.Env().VM, []string{"process_discovery"}, false, command)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessDiscoveryPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().Fakeintake.GetProcessDiscoveries()
		assert.NoError(c, err, "failed to get process discovery payloads from fakeintake")
		assert.NotEmpty(c, payloads, "no process discovery payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessDiscoveryCollected(t, payloads, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestProcessCheckIO() {
	s.UpdateEnv(e2e.FakeIntakeStackDef(e2e.WithAgentParams(
		agentparams.WithAgentConfig(processCheckConfigStr),
		agentparams.WithSystemProbeConfig(systemProbeConfigStr),
	), e2e.WithVMParams(ec2params.WithOS(ec2os.WindowsOS)),
	))

	// Flush fake intake to remove payloads that won't have IO stats
	s.Env().Fakeintake.FlushServerAndResetAggregators()

	t := s.T()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		command := "& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe\" status --json"
		assertRunningChecks(collect, s.Env().VM, []string{"process", "rtprocess"}, true, command)
	}, 1*time.Minute, 5*time.Second)

	//s.Env().VM.Execute("Start-MpScan -ScanType FullScan")

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().Fakeintake.GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, true, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestManualProcessCheck() {
	check := s.Env().VM.
		Execute("& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\process-agent.exe\" check process --json")

	assertManualProcessCheck(s.T(), check, false, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestManualProcessDiscoveryCheck() {
	// Skipping due to flakiness
	// Responses with more than 100 processes end up being chunked, which fails JSON unmarshalling
	s.T().Skip()

	check := s.Env().VM.
		Execute("& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\process-agent.exe\" check process_discovery --json")
	assertManualProcessDiscoveryCheck(s.T(), check, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestManualProcessCheckWithIO() {
	s.UpdateEnv(e2e.FakeIntakeStackDef(e2e.WithAgentParams(
		agentparams.WithAgentConfig(processCheckConfigStr),
		agentparams.WithSystemProbeConfig(systemProbeConfigStr),
	), e2e.WithVMParams(ec2params.WithOS(ec2os.WindowsOS)),
	))

	// Flush fake intake to remove payloads that won't have IO stats
	s.Env().Fakeintake.FlushServerAndResetAggregators()

	check := s.Env().VM.
		Execute("& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\process-agent.exe\" check process --json")

	assertManualProcessCheck(s.T(), check, true, "MsMpEng.exe")
}
