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
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsvm "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/vm"
)

type linuxTestSuite struct {
	e2e.BaseSuite[environments.VM]
}

func TestLinuxTestSuite(t *testing.T) {
	e2e.Run(t, &linuxTestSuite{}, e2e.WithProvisioner(awsvm.Provisioner(awsvm.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr)))))
}

func (s *linuxTestSuite) SetupSuite() {
	// Start a process and keep it running
	s.Env().Host.Execute("sudo apt-get -y install stress")
	s.Env().Host.Execute("nohup stress -d 1 >myscript.log 2>&1 </dev/null &")
}

func (s *linuxTestSuite) TestProcessCheck() {
	t := s.T()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Host, []string{"process", "rtprocess"}, false)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertStressProcessCollected(t, payloads, false)
}

func (s *linuxTestSuite) TestProcessDiscoveryCheck() {
	s.UpdateEnv(awsvm.Provisioner(awsvm.WithAgentOptions(agentparams.WithAgentConfig(processDiscoveryCheckConfigStr))))
	t := s.T()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Host, []string{"process_discovery"}, false)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessDiscoveryPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcessDiscoveries()
		assert.NoError(c, err, "failed to get process discovery payloads from fakeintake")
		assert.NotEmpty(c, payloads, "no process discovery payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertStressProcessDiscoveryCollected(t, payloads)
}

func (s *linuxTestSuite) TestProcessCheckWithIO() {
	s.UpdateEnv(awsvm.Provisioner(awsvm.WithAgentOptions(agentparams.WithAgentConfig(processDiscoveryCheckConfigStr), agentparams.WithSystemProbeConfig(systemProbeConfigStr))))

	// Flush fake intake to remove payloads that won't have IO stats
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	t := s.T()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Host, []string{"process", "rtprocess"}, true)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertStressProcessCollected(t, payloads, true)
}

func (s *linuxTestSuite) TestManualProcessCheck() {
	check := s.Env().Host.MustExecute("sudo /opt/datadog-agent/embedded/bin/process-agent check process --json")

	assertManualProcessCheck(s.T(), check, false)
}

func (s *linuxTestSuite) TestManualProcessDiscoveryCheck() {
	check := s.Env().Host.MustExecute("sudo /opt/datadog-agent/embedded/bin/process-agent check process_discovery --json")

	assertManualProcessDiscoveryCheck(s.T(), check)
}

func (s *linuxTestSuite) TestManualProcessCheckWithIO() {
	s.UpdateEnv(awsvm.Provisioner(awsvm.WithAgentOptions(agentparams.WithAgentConfig(processDiscoveryCheckConfigStr), agentparams.WithSystemProbeConfig(systemProbeConfigStr))))

	check := s.Env().Host.MustExecute("sudo /opt/datadog-agent/embedded/bin/process-agent check process --json")

	assertManualProcessCheck(s.T(), check, true)
}
