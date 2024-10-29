// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/docker"

	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

type DockerFakeintakeSuite struct {
	e2e.BaseSuite[environments.DockerHost]
	transport transport
}

func dockerSuiteOpts(tr transport, opts ...awsdocker.ProvisionerOption) []e2e.SuiteOption {
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awsdocker.Provisioner(opts...)),
		e2e.WithStackName(fmt.Sprintf("apm-docker-suite-%s", tr)),
	}
	return options
}

var dockerUDSAgentOptions = []func(*dockeragentparams.Params) error{
	// Enable the UDS receiver in the trace-agent
	dockeragentparams.WithAgentServiceEnvVariable(
		"DD_APM_RECEIVER_SOCKET",
		pulumi.String("/var/run/datadog/apm.socket")),
	// Optional: UDS is more reliable for statsd metrics
	// Set DD_DOGSTATSD_SOCKET to enable the UDS statsd listener in the core-agent
	dockeragentparams.WithAgentServiceEnvVariable(
		"DD_DOGSTATSD_SOCKET",
		pulumi.String("/var/run/datadog/dsd.socket")),
	// Set STATSD_URL to instruct the statsd client in the trace-agent to send metrics through UDS
	dockeragentparams.WithAgentServiceEnvVariable(
		"STATSD_URL",
		pulumi.String("unix:///var/run/datadog/dsd.socket")),
}

func dockerAgentOptions(tr transport) []func(*dockeragentparams.Params) error {
	switch tr {
	case uds:
		return dockerUDSAgentOptions
	case tcp:
		return nil
	}
	return nil
}

// TestDockerFakeintakeSuiteUDS runs basic Trace Agent tests over the UDS transport
func TestDockerFakeintakeSuiteUDS(t *testing.T) {
	options := dockerSuiteOpts(uds, awsdocker.WithAgentOptions(
		dockerAgentOptions(uds)...,
	))
	e2e.Run(t, &DockerFakeintakeSuite{transport: uds}, options...)
}

// TestDockerFakeintakeSuiteTCP runs basic Trace Agent tests over the TCP transport
func TestDockerFakeintakeSuiteTCP(t *testing.T) {
	options := dockerSuiteOpts(tcp, awsdocker.WithAgentOptions(
		dockerAgentOptions(tcp)...,
	))
	e2e.Run(t, &DockerFakeintakeSuite{transport: tcp}, options...)
}

func (s *DockerFakeintakeSuite) TestTraceAgentMetrics() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)
	s.EventuallyWithTf(func(c *assert.CollectT) {
		testTraceAgentMetrics(s.T(), c, s.Env().FakeIntake)
	}, 2*time.Minute, 10*time.Second, "Failed finding datadog.trace_agent.* metrics")
}

func (s *DockerFakeintakeSuite) TestTraceAgentMetricTags() {
	service := fmt.Sprintf("tracegen-metric-tags-%s", s.transport)
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)
	s.EventuallyWithTf(func(c *assert.CollectT) {
		testTraceAgentMetricTags(s.T(), c, service, s.Env().FakeIntake)
	}, 3*time.Minute, 10*time.Second, "Failed finding datadog.trace_agent.* metrics with tags")
}

func (s *DockerFakeintakeSuite) TestTracesHaveContainerTag() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-container-tag-%s", s.transport)
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	defer runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})()
	s.EventuallyWithTf(func(c *assert.CollectT) {
		testTracesHaveContainerTag(s.T(), c, service, s.Env().FakeIntake)
	}, 2*time.Minute, 10*time.Second, "Failed finding traces with container tags")
}

func (s *DockerFakeintakeSuite) TestAutoVersionTraces() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-auto-version-traces-%s", s.transport)
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	defer runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})()
	s.EventuallyWithTf(func(c *assert.CollectT) {
		testAutoVersionTraces(s.T(), c, s.Env().FakeIntake)
	}, 2*time.Minute, 10*time.Second, "Failed finding version tags")
}

func (s *DockerFakeintakeSuite) TestAutoVersionStats() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-auto-version-stats-%s", s.transport)
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	defer runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})()
	s.EventuallyWithTf(func(c *assert.CollectT) {
		testAutoVersionStats(s.T(), c, s.Env().FakeIntake)
	}, 2*time.Minute, 10*time.Second, "Failed finding version tags")
}

func (s *DockerFakeintakeSuite) TestIsTraceRootTag() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-auto-is-trace-root-tag-%s", s.transport)
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	defer runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})()
	s.EventuallyWithTf(func(c *assert.CollectT) {
		testIsTraceRootTag(s.T(), c, s.Env().FakeIntake)
	}, 2*time.Minute, 10*time.Second, "Failed finding is_trace_root tag")
}

func (s *DockerFakeintakeSuite) TestStatsForService() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-stats-%s", s.transport)
	addSpanTags := "peer.hostname:foo,span.kind:client"
	expectPeerTag := "peer.hostname:foo"
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	defer runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport, addSpanTags: addSpanTags})()
	s.EventuallyWithTf(func(c *assert.CollectT) {
		testStatsForService(s.T(), c, service, expectPeerTag, s.Env().FakeIntake)
	}, 2*time.Minute, 10*time.Second, "Failed finding stats")
}

func (s *DockerFakeintakeSuite) TestBasicTrace() {
	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-basic-trace-%s", s.transport)

	// Run Trace Generator
	s.T().Log("Starting Trace Generator.")
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.T().Log("Waiting for traces.")
	s.EventuallyWithTf(func(c *assert.CollectT) {
		testBasicTraces(c, service, s.Env().FakeIntake, s.Env().Agent.Client)
	}, 2*time.Minute, 10*time.Second, "Failed to find traces with basic properties")
}

func (s *DockerFakeintakeSuite) TestTPS() {
	agentTPS := 2.

	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithAgentOptions(
		append(dockerAgentOptions(s.transport),
			dockeragentparams.WithAgentServiceEnvVariable(
				"DD_APM_TARGET_TPS",
				pulumi.Float64(agentTPS)),
		)...)))

	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-tps-trace-%s", s.transport)

	// Run Trace Generator
	s.T().Log("Starting Trace Generator.")
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.T().Log("Waiting for traces.")
	s.EventuallyWithTf(func(c *assert.CollectT) {
		testTPS(c, s.Env().FakeIntake, agentTPS)
	}, 2*time.Minute, 10*time.Second, "Failed to test TargetTPS")
}

func (s *DockerFakeintakeSuite) TestProbabilitySampler() {
	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithAgentOptions(
		append(dockerAgentOptions(s.transport),
			dockeragentparams.WithAgentServiceEnvVariable(
				"DD_APM_PROBABILISTIC_SAMPLER_ENABLED",
				pulumi.Bool(true)),
			dockeragentparams.WithAgentServiceEnvVariable(
				"DD_APM_PROBABILISTIC_SAMPLER_SAMPLING_PERCENTAGE",
				pulumi.Int(50)),
			dockeragentparams.WithAgentServiceEnvVariable(
				"DD_APM_PROBABILISTIC_SAMPLER_SAMPLING_PERCENTAGE",
				pulumi.Int(22)),
		)...)))

	err := s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	s.Require().NoError(err)

	service := fmt.Sprintf("tracegen-probability-sampler-%s", s.transport)

	// Run Trace Generator
	s.T().Log("Starting Trace Generator.")
	defer waitTracegenShutdown(&s.Suite, s.Env().FakeIntake)
	shutdown := runTracegenDocker(s.Env().RemoteHost, service, tracegenCfg{transport: s.transport})
	defer shutdown()

	s.T().Log("Waiting for traces.")
	s.EventuallyWithTf(func(c *assert.CollectT) {
		tracesSampledByProbabilitySampler(s.T(), c, s.Env().FakeIntake)
	}, 2*time.Minute, 10*time.Second, "Failed to find traces sampled by the probability sampler")
}
