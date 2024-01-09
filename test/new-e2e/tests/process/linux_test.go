// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
)

type linuxTestSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func TestLinuxTestSuite(t *testing.T) {
	e2e.Run(t, &linuxTestSuite{},
		e2e.FakeIntakeStackDef(
			e2e.WithAgentParams(agentparams.WithAgentConfig(processCheckConfigStr)),
		))
}

func (s *linuxTestSuite) SetupSuite() {
	s.Suite.SetupSuite()

	// Start a process and keep it running
	s.Env().VM.Execute("sudo apt-get -y install stress")
	s.Env().VM.Execute("nohup stress -d 1 >myscript.log 2>&1 </dev/null &")
}

func (s *linuxTestSuite) TestProcessCheck() {
	t := s.T()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().VM, []string{"process", "rtprocess"}, false, "sudo datadog-agent status --json")
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().Fakeintake.GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "stress")
}

func (s *linuxTestSuite) TestProcessDiscoveryCheck() {
	s.UpdateEnv(e2e.FakeIntakeStackDef(
		e2e.WithAgentParams(agentparams.WithAgentConfig(processDiscoveryCheckConfigStr))))

	t := s.T()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().VM, []string{"process_discovery"}, false, "sudo datadog-agent status --json")
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessDiscoveryPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().Fakeintake.GetProcessDiscoveries()
		assert.NoError(c, err, "failed to get process discovery payloads from fakeintake")
		assert.NotEmpty(c, payloads, "no process discovery payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessDiscoveryCollected(t, payloads, "stress")
}

func (s *linuxTestSuite) TestProcessCheckWithIO() {
	s.UpdateEnv(e2e.FakeIntakeStackDef(e2e.WithAgentParams(
		agentparams.WithAgentConfig(processCheckConfigStr),
		agentparams.WithSystemProbeConfig(systemProbeConfigStr),
	)))

	// Flush fake intake to remove payloads that won't have IO stats
	s.Env().Fakeintake.FlushServerAndResetAggregators()

	t := s.T()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().VM, []string{"process", "rtprocess"}, true, "sudo datadog-agent status --json")
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().Fakeintake.GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, true, "stress")
}

func (s *linuxTestSuite) TestManualProcessCheck() {
	check := s.Env().VM.
		Execute("sudo /opt/datadog-agent/embedded/bin/process-agent check process --json")

	assertManualProcessCheck(s.T(), check, false, "stress")
}

func (s *linuxTestSuite) TestManualProcessDiscoveryCheck() {
	check := s.Env().VM.
		Execute("sudo /opt/datadog-agent/embedded/bin/process-agent check process_discovery --json")

	assertManualProcessDiscoveryCheck(s.T(), check, "stress")
}

func (s *linuxTestSuite) TestManualProcessCheckWithIO() {
	s.UpdateEnv(e2e.FakeIntakeStackDef(e2e.WithAgentParams(
		agentparams.WithAgentConfig(processCheckConfigStr),
		agentparams.WithSystemProbeConfig(systemProbeConfigStr),
	)))

	check := s.Env().VM.
		Execute("sudo /opt/datadog-agent/embedded/bin/process-agent check process --json")

	assertManualProcessCheck(s.T(), check, true, "stress")
}
