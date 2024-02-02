// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

type windowsTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestWindowsTestSuite(t *testing.T) {
	e2e.Run(t, &windowsTestSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
				awshost.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr)),
			),
		),
	)
}

func (s *windowsTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// Start an antivirus scan to use as process for testing
	s.Env().RemoteHost.MustExecute("Start-MpScan -ScanType FullScan -AsJob")
}

func (s *windowsTestSuite) TestProcessCheck() {
	t := s.T()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		command := "& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe\" status --json"
		assertRunningChecks(collect, s.Env().RemoteHost, []string{"process", "rtprocess"}, false, command)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestProcessDiscoveryCheck() {
	t := s.T()
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		awshost.WithAgentOptions(agentparams.WithAgentConfig(processDiscoveryCheckConfigStr)),
	))

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		command := "& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe\" status --json"
		assertRunningChecks(collect, s.Env().RemoteHost, []string{"process_discovery"}, false, command)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessDiscoveryPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcessDiscoveries()
		assert.NoError(c, err, "failed to get process discovery payloads from fakeintake")
		assert.NotEmpty(c, payloads, "no process discovery payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessDiscoveryCollected(t, payloads, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestProcessCheckIO() {
	t := s.T()
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		awshost.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr), agentparams.WithSystemProbeConfig(systemProbeConfigStr)),
	))

	// Flush fake intake to remove payloads that won't have IO stats
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		command := "& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe\" status --json"
		assertRunningChecks(collect, s.Env().RemoteHost, []string{"process", "rtprocess"}, true, command)
	}, 1*time.Minute, 5*time.Second)

	// s.Env().VM.Execute("Start-MpScan -ScanType FullScan")

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, true, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestManualProcessCheck() {
	check := s.Env().RemoteHost.
		MustExecute("& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\process-agent.exe\" check process --json")

	assertManualProcessCheck(s.T(), check, false, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestManualProcessDiscoveryCheck() {
	// Skipping due to flakiness
	// Responses with more than 100 processes end up being chunked, which fails JSON unmarshalling
	s.T().Skip("PROCS-3644: consistent failures on process tests")

	check := s.Env().RemoteHost.
		MustExecute("& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\process-agent.exe\" check process_discovery --json")
	assertManualProcessDiscoveryCheck(s.T(), check, "MsMpEng.exe")
}

func (s *windowsTestSuite) TestManualProcessCheckWithIO() {
	s.T().Skip("PROCS-3644: consistent failures on process tests")
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
		awshost.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr), agentparams.WithSystemProbeConfig(systemProbeConfigStr)),
	))

	// Flush fake intake to remove payloads that won't have IO stats
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	check := s.Env().RemoteHost.
		MustExecute("& \"C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent\\process-agent.exe\" check process --json")

	assertManualProcessCheck(s.T(), check, true, "MsMpEng.exe")
}
