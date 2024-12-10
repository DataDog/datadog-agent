// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netpath contains e2e tests for Network Path Integration feature
package netpath

import (
	_ "embed"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
)

type networkPathIntegrationTestSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestNetworkPathIntegrationSuite runs the Network Path Integration e2e suite
func TestNetworkPathIntegrationSuite(t *testing.T) {
	// language=yaml
	networkPathIntegration := `
instances:
- hostname: api.datadoghq.eu
  protocol: TCP
  port: 443
- hostname: 8.8.8.8
  protocol: UDP
`

	e2e.Run(t, &networkPathIntegrationTestSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithIntegration("network_path.d", networkPathIntegration),
		)),
	))
}

func (s *networkPathIntegrationTestSuite) TestHaAgentRunningMetrics() {
	//fakeClient := s.Env().FakeIntake.Client()

	s.EventuallyWithT(func(c *assert.CollectT) {
		s.T().Log("try assert datadog.agent.running metric")
		//metrics, err := fakeClient.FilterMetrics("datadog.agent.running")
		//require.NoError(c, err)
		//assert.NotEmpty(c, metrics)
		//for _, metric := range metrics {
		//	s.T().Logf("    datadog.agent.running metric tags: %+v", metric.Tags)
		//}
		//
		//tags := []string{"agent_group:test-group01"}
		//metrics, err = fakeClient.FilterMetrics("datadog.agent.running", fakeintakeclient.WithTags[*aggregator.MetricSeries](tags))
		//require.NoError(c, err)
		//assert.NotEmpty(c, metrics)
	}, 5*time.Minute, 3*time.Second)
}
