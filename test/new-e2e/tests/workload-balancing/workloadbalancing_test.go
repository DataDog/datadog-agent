// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadbalancing contains e2e tests for the Agent Workload Balancing feature
package workloadbalancing

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const workloadBalancingHostname = "test-e2e-workload-balancing"
const workloadBalancingGroupID = "test-group01"

type workloadBalancingTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestWorkloadBalancingSuite runs the Agent Workload Balancing e2e suite
func TestWorkloadBalancingSuite(t *testing.T) {
	// language=yaml
	agentConfig := fmt.Sprintf(`
hostname: %s
agent_workload_balancing:
    enabled: true
log_level: debug
`, workloadBalancingHostname)

	e2e.Run(t, &workloadBalancingTestSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig)))),
	))
}

// setActiveAgent pushes an NDM_AGENT_WORKLOAD_BALANCING Remote Config payload to
// fakeintake, electing activeHostname as the active agent for the group. This
// replaces the balancing decision the real Datadog backend would otherwise make.
func (s *workloadBalancingTestSuite) setActiveAgent(activeHostname string) {
	fakeClient := s.Env().FakeIntake.Client()

	payload := fmt.Sprintf(`{"group_id":%q,"active_agent":%q}`, workloadBalancingGroupID, activeHostname)
	err := fakeClient.RCAddConfig("", state.ProductNDMAgentWorkloadBalancing, workloadBalancingGroupID, "leader", []byte(payload))
	require.NoError(s.T(), err)
}

func (s *workloadBalancingTestSuite) TestWorkloadBalancingRunningMetrics() {
	s.setActiveAgent(workloadBalancingHostname)

	fakeClient := s.Env().FakeIntake.Client()

	s.EventuallyWithT(func(c *assert.CollectT) {
		s.T().Log("try assert datadog.agent.workload_balancing.running metric")
		metrics, err := fakeClient.FilterMetrics("datadog.agent.workload_balancing.running")
		require.NoError(c, err)
		assert.NotEmpty(c, metrics)
		for _, metric := range metrics {
			s.T().Logf("    datadog.agent.workload_balancing.running metric tags: %+v", metric.Tags)
		}

		tags := []string{"workload_balancing_group:" + workloadBalancingGroupID, "workload_balancing_state:active"}
		metrics, err = fakeClient.FilterMetrics("datadog.agent.workload_balancing.running", fakeintakeclient.WithTags[*aggregator.MetricSeries](tags))
		require.NoError(c, err)
		assert.NotEmpty(c, metrics)

		metrics, err = fakeClient.FilterMetrics("datadog.agent.running")
		require.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 5*time.Minute, 3*time.Second)
}

func (s *workloadBalancingTestSuite) TestWorkloadBalancingAddedToRCListeners() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		s.T().Log("try assert workload balancing added to RCListeners in agent.log")
		output, err := s.Env().RemoteHost.ReadFilePrivileged("/var/log/datadog/agent.log")
		require.NoError(c, err)

		assert.Contains(c, string(output), "Add workload balancing RCListener")
	}, 5*time.Minute, 3*time.Second)
}

// TODO: Add a multi-host failover test analogous to haagent_failover_test.go, driving
//       an active-agent handoff for a group between two hosts via Remote Config.
