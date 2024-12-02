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

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

type haAgentTestSuite03 struct {
	e2e.BaseSuite[environments.Host]
}

// TestHaAgentSuite runs the netflow e2e suite
func TestHaAgentSuite(t *testing.T) {
	// language=yaml
	agentConfig := `
ha_agent:
    enabled: true
    group: test-group01
log_level: debug
`
	e2e.Run(t, &haAgentTestSuite03{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
	))
}

func (s *haAgentTestSuite03) TestHaAgentGroupTag_PresentOnDatadogAgentRunningMetric() {
	fakeClient := s.Env().FakeIntake.Client()
	s.EventuallyWithT(func(c *assert.CollectT) {
		s.T().Log("try assert datadog.agent.running metric")
		metrics, err := fakeClient.FilterMetrics("datadog.agent.running")
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
		for _, metric := range metrics {
			s.T().Logf("    datadog.agent.running metric in fake intake: %+v", metric)
		}

		tags := []string{"agent_group:test-group01"}
		metrics, err = fakeClient.FilterMetrics("datadog.agent.running", fakeintakeclient.WithTags[*aggregator.MetricSeries](tags))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 5*time.Minute, 3*time.Second)
}

func (s *haAgentTestSuite03) TestHaAgentAddedToRCListeners() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		output, err := s.Env().RemoteHost.Execute("cat /var/log/datadog/agent.log")
		if !assert.NoError(c, err) {
			return
		}
		assert.Contains(c, output, "Add onHaAgentUpdate RCListener")
	}, 3*time.Minute, 1*time.Second)
}
