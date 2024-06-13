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
	t.Parallel()
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", pulumi.StringPtr("false")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", pulumi.StringPtr("false")),

		dockeragentparams.WithExtraComposeManifest("fakeProcess", pulumi.String(fakeProcessCompose)),
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

	assertProcessCollected(t, payloads, false, "dd")
	assertContainersCollected(t, payloads, []string{"fake-process"})
}

func (s *dockerTestSuite) TestProcessDiscoveryCheck() {
	t := s.T()
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("false")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED", pulumi.StringPtr("false")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED", pulumi.StringPtr("true")),

		dockeragentparams.WithExtraComposeManifest("fakeProcess", pulumi.String(fakeProcessCompose)),
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

	assertProcessDiscoveryCollected(t, payloads, "dd")
}

func (s *dockerTestSuite) TestProcessCheckWithIO() {
	t := s.T()
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_SYSTEM_PROBE_PROCESS_ENABLED", pulumi.StringPtr("true")),
	}
	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithAgentOptions(agentOpts...)))

	// Flush fake intake to remove payloads that won't have IO stats
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Agent.Client, []string{"process", "rtprocess"}, true)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, true, "dd")
}

func (s *dockerTestSuite) TestProcessChecksWithNPM() {
	t := s.T()
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_SYSTEM_PROBE_NETWORK_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithExtraComposeManifest("fakeProcess", pulumi.String(fakeProcessCompose)),
	}
	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithAgentOptions(agentOpts...)))

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Agent.Client, []string{"process", "rtprocess", "connections"}, false)
	}, 1*time.Minute, 5*time.Second)

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "dd")
	assertContainersCollected(t, payloads, []string{"fake-process"})
}

func (s *dockerTestSuite) TestProcessChecksInCoreAgent() {
	t := s.T()
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED", pulumi.StringPtr("true")),
	}

	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithAgentOptions(agentOpts...)))

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		status := getAgentStatus(collect, s.Env().Agent.Client)

		// verify the standalone process-agent is not running
		assert.NotEmpty(t, status.ProcessAgentStatus.Error, "status: %+v", status)
		assert.Empty(t, status.ProcessAgentStatus.Expvars.Map.EnabledChecks)

		// Verify the process component is running in the core agent
		assert.ElementsMatch(t, status.ProcessComponentStatus.Expvars.Map.EnabledChecks, []string{"process", "rtprocess"})

	}, 1*time.Minute, 5*time.Second)

	// Flush fake intake to remove any payloads which may have
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "dd")

	// check that the process agent is not collected as it should not be running
	requireProcessNotCollected(t, payloads, "process-agent")
}

func (s *dockerTestSuite) TestProcessChecksInCoreAgentWithNPM() {
	t := s.T()
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_SYSTEM_PROBE_NETWORK_ENABLED", pulumi.StringPtr("true")),
	}
	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithAgentOptions(agentOpts...)))

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		assertRunningChecks(collect, s.Env().Agent.Client, []string{"connections"}, false)
	}, 1*time.Minute, 5*time.Second)

	assert.EventuallyWithT(t, func(collect *assert.CollectT) {
		status := getAgentStatus(collect, s.Env().Agent.Client)

		// verify the standalone process-agent is running with just the NPM check
		assert.Empty(t, status.ProcessAgentStatus.Error, "status: %+v", status)
		assert.ElementsMatch(t, []string{"connections"}, status.ProcessAgentStatus.Expvars.Map.EnabledChecks)

		// Verify the process component is running in the core agent
		assert.ElementsMatch(t, status.ProcessComponentStatus.Expvars.Map.EnabledChecks, []string{"process", "rtprocess"})

	}, 1*time.Minute, 5*time.Second)

	// Flush fake intake to remove any payloads which may have
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	var payloads []*aggregator.ProcessPayload
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		payloads, err = s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		// Wait for two payloads, as processes must be detected in two check runs to be returned
		assert.GreaterOrEqual(c, len(payloads), 2, "fewer than 2 payloads returned")
	}, 2*time.Minute, 10*time.Second)

	assertProcessCollected(t, payloads, false, "dd")
}

func (s *dockerTestSuite) TestManualProcessCheck() {
	check := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName,
		"process-agent", "check", "process", "--json")

	assertManualProcessCheck(s.T(), check, false, "dd", "fake-process")
}

func (s *dockerTestSuite) TestManualProcessDiscoveryCheck() {
	check := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName,
		"process-agent", "check", "process_discovery", "--json")

	assertManualProcessDiscoveryCheck(s.T(), check, "dd")
}

func (s *dockerTestSuite) TestManualProcessCheckWithIO() {
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_SYSTEM_PROBE_PROCESS_ENABLED", pulumi.StringPtr("true")),
	}
	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithAgentOptions(agentOpts...)))

	check := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName,
		"process-agent", "check", "process", "--json")

	assertManualProcessCheck(s.T(), check, true, "dd")
}
