// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/docker"
)

type dockerTestSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestDockerTestSuite(t *testing.T) {
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", pulumi.StringPtr("false")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", pulumi.StringPtr("false")),

		dockeragentparams.WithExtraComposeManifest("stress", pulumi.String(stressCompose)),
	}

	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awsdocker.Provisioner(
			awsdocker.WithAgentOptions(agentOpts...),
		)),
	}

	devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")
	if devMode, err := strconv.ParseBool(devModeEnv); err == nil && devMode {
		options = append(options, e2e.WithDevMode())
	}

	e2e.Run(t, &dockerTestSuite{}, options...)
}

func (s *dockerTestSuite) TestDockerProcessCheck() {
	t := s.T()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Agent.Client, []string{"process", "rtprocess"}, false)
	}, 2*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "stress")

	// TODO: assert containers collected
}

func (s *dockerTestSuite) TestProcessDiscoveryCheck() {
	t := s.T()
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("false")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", pulumi.StringPtr("false")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", pulumi.StringPtr("true")),

		dockeragentparams.WithExtraComposeManifest("stress", pulumi.String(stressCompose)),
	}

	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithAgentOptions(agentOpts...)))

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Agent.Client, []string{"process_discovery"}, false)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessDiscoveryPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcessDiscoveries()
		assert.NoError(c, err, "failed to get process discovery payloads from fakeintake")
		assert.NotEmpty(c, payloads, "no process discovery payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessDiscoveryCollected(t, payloads, "stress")
}

//
//func (s *dockerTestSuite) TestProcessCheckWithIO() {
//	t := s.T()
//	s.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr), agentparams.WithSystemProbeConfig(systemProbeConfigStr))))
//
//	// Flush fake intake to remove payloads that won't have IO stats
//	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
//
//	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
//		assertRunningChecks(collect, s.Env().RemoteHost, []string{"process", "rtprocess"}, true, "sudo datadog-agent status --json")
//	}, 1*time.Minute, 5*time.Second)
//
//	var payloads []*aggregator.ProcessPayload
//	assert.EventuallyWithT(t, func(c *assert.CollectT) {
//		var err error
//		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
//		assert.NoError(c, err, "failed to get process payloads from fakeintake")
//
//		// Wait for two payloads, as processes must be detected in two check runs to be returned
//		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
//	}, 2*time.Minute, 10*time.Second)
//
//	assertProcessCollected(t, payloads, true, "stress")
//}
//
//func (s *dockerTestSuite) TestProcessChecksInCoreAgent() {
//	t := s.T()
//	s.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(agentparams.WithAgentConfig(processCheckInCoreAgentConfigStr))))
//
//	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
//		assertRunningChecks(collect, s.Env().RemoteHost, []string{}, false, "sudo datadog-agent status --json")
//	}, 1*time.Minute, 5*time.Second)
//
//	// Verify that the process agent is not running
//	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
//		status := s.Env().RemoteHost.MustExecute("/opt/datadog-agent/embedded/bin/process-agent status")
//		assert.Contains(t, status, "The Process Agent is not running")
//	}, 1*time.Minute, 5*time.Second)
//
//	// Flush fake intake to remove any payloads which may have
//	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
//
//	var payloads []*aggregator.ProcessPayload
//	assert.EventuallyWithT(t, func(c *assert.CollectT) {
//		var err error
//		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
//		assert.NoError(c, err, "failed to get process payloads from fakeintake")
//
//		// Wait for two payloads, as processes must be detected in two check runs to be returned
//		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
//	}, 2*time.Minute, 10*time.Second)
//
//	assertProcessCollected(t, payloads, false, "stress")
//
//	// check that the process agent is not collected as it should not be running
//	requireProcessNotCollected(t, payloads, "process-agent")
//}
//
//func (s *dockerTestSuite) TestProcessChecksInCoreAgentWithNPM() {
//	t := s.T()
//	s.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(agentparams.WithAgentConfig(processCheckInCoreAgentConfigStr), agentparams.WithSystemProbeConfig(systemProbeNPMConfigStr))))
//
//	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
//		assertRunningChecks(collect, s.Env().RemoteHost, []string{"connections"}, false, "sudo datadog-agent status --json")
//	}, 1*time.Minute, 5*time.Second)
//
//	// Flush fake intake to remove any payloads which may have
//	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
//
//	var payloads []*aggregator.ProcessPayload
//	assert.EventuallyWithT(t, func(c *assert.CollectT) {
//		var err error
//		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
//		assert.NoError(c, err, "failed to get process payloads from fakeintake")
//
//		// Wait for two payloads, as processes must be detected in two check runs to be returned
//		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
//	}, 2*time.Minute, 10*time.Second)
//
//	assertProcessCollected(t, payloads, false, "stress")
//}
//
//func (s *dockerTestSuite) TestProcessChecksWithNPM() {
//	t := s.T()
//	s.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr), agentparams.WithSystemProbeConfig(systemProbeNPMConfigStr))))
//
//	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
//		assertRunningChecks(collect, s.Env().RemoteHost, []string{"process", "rtprocess", "connections"}, false, "sudo datadog-agent status --json")
//	}, 1*time.Minute, 5*time.Second)
//
//	// Flush fake intake to remove any payloads which may have
//	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
//
//	var payloads []*aggregator.ProcessPayload
//	assert.EventuallyWithT(t, func(c *assert.CollectT) {
//		var err error
//		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
//		assert.NoError(c, err, "failed to get process payloads from fakeintake")
//
//		// Wait for two payloads, as processes must be detected in two check runs to be returned
//		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
//	}, 2*time.Minute, 10*time.Second)
//
//	assertProcessCollected(t, payloads, false, "stress")
//}
//
//func (s *dockerTestSuite) TestManualProcessCheck() {
//	check := s.Env().RemoteHost.MustExecute("sudo /opt/datadog-agent/embedded/bin/process-agent check process --json")
//
//	assertManualProcessCheck(s.T(), check, false, "stress")
//}
//
//func (s *dockerTestSuite) TestManualProcessDiscoveryCheck() {
//	check := s.Env().RemoteHost.MustExecute("sudo /opt/datadog-agent/embedded/bin/process-agent check process_discovery --json")
//
//	assertManualProcessDiscoveryCheck(s.T(), check, "stress")
//}
//
//func (s *dockerTestSuite) TestManualProcessCheckWithIO() {
//	s.UpdateEnv(awshost.Provisioner(awshost.WithAgentOptions(agentparams.WithAgentConfig(processCheckConfigStr), agentparams.WithSystemProbeConfig(systemProbeConfigStr))))
//
//	check := s.Env().RemoteHost.MustExecute("sudo /opt/datadog-agent/embedded/bin/process-agent check process --json")
//
//	assertManualProcessCheck(s.T(), check, true, "stress")
//}
