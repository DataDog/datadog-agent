// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package haagent contains e2e tests for HA Agent feature
package haagent

import (
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

type haAgentTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestHaAgentSuite runs the HA Agent e2e suite
func TestHaAgentSuite(t *testing.T) {
	// language=yaml
	agentConfig := `
ha_agent:
    enabled: true
config_id: test-config01
log_level: debug
`
	e2e.Run(t, &haAgentTestSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig)))),
	))
}

func (s *haAgentTestSuite) TestHaAgentRunningMetrics() {
	fakeClient := s.Env().FakeIntake.Client()

	s.EventuallyWithT(func(c *assert.CollectT) {
		s.T().Log("try assert datadog.agent.ha_agent.running metric")
		metrics, err := fakeClient.FilterMetrics("datadog.agent.ha_agent.running")
		require.NoError(c, err)
		assert.NotEmpty(c, metrics)
		for _, metric := range metrics {
			s.T().Logf("    datadog.agent.ha_agent.running metric tags: %+v", metric.Tags)
		}

		tags := []string{"ha_agent_state:unknown", "config_id:test-config01"}
		metrics, err = fakeClient.FilterMetrics("datadog.agent.ha_agent.running", fakeintakeclient.WithTags[*aggregator.MetricSeries](tags))
		require.NoError(c, err)
		assert.NotEmpty(c, metrics)

		tags = []string{"config_id:test-config01"}
		metrics, err = fakeClient.FilterMetrics("datadog.agent.running", fakeintakeclient.WithTags[*aggregator.MetricSeries](tags))
		require.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 5*time.Minute, 3*time.Second)
}

func (s *haAgentTestSuite) TestHaAgentAddedToRCListeners() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		s.T().Log("try assert HA Agent added to RCListeners in agent.log")
		output, err := s.Env().RemoteHost.ReadFilePrivileged("/var/log/datadog/agent.log")
		require.NoError(c, err)

		assert.Contains(c, string(output), "Add HA Agent RCListener")
	}, 5*time.Minute, 3*time.Second)
}

// TODO: Add test for Agent behaviour when receiving RC HA_AGENT messages
//       - Agent receiving message to become leader
//       - Agent receiving message to become follower
