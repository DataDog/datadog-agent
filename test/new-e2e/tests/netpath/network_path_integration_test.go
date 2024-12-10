// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netpath contains e2e tests for Network Path Integration feature
package netpath

import (
	_ "embed"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	sysProbeConfig := `
traceroute:
  enabled: true
`

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
			agentparams.WithSystemProbeConfig(sysProbeConfig),
			agentparams.WithIntegration("network_path.d", networkPathIntegration),
		)),
	))
}

func (s *networkPathIntegrationTestSuite) TestMetrics() {
	fakeClient := s.Env().FakeIntake.Client()

	s.EventuallyWithT(func(c *assert.CollectT) {
		s.T().Log("try assert datadog.network_path.path.monitored metric")
		metrics, err := fakeClient.FilterMetrics("datadog.network_path.path.monitored")
		require.NoError(c, err)
		assert.NotEmpty(c, metrics)
		for _, metric := range metrics {
			s.T().Logf("    datadog.network_path.path.monitored metric tags: %+v", metric.Tags)
		}

		destinationsTagsToAssert := [][]string{
			{"destination_hostname:api.datadoghq.eu", "protocol:TCP", "destination_port:443"},
			{"destination_hostname:8.8.8.8", "protocol:UDP"},
		}
		for _, tags := range destinationsTagsToAssert {
			// assert destination is monitored
			metrics, err = fakeClient.FilterMetrics("datadog.network_path.path.monitored", fakeintakeclient.WithTags[*aggregator.MetricSeries](tags))
			assert.NoError(c, err)
			assert.NotEmpty(c, metrics, fmt.Sprintf("metric with tags `%v` not found", tags))

			// assert hops
			metrics, err = fakeClient.FilterMetrics("datadog.network_path.path.hops",
				fakeintakeclient.WithTags[*aggregator.MetricSeries](tags),
				fakeintakeclient.WithMetricValueHigherThan(0),
			)
			assert.NoError(c, err)
			assert.NotEmpty(c, metrics, fmt.Sprintf("metric with tags `%v` not found", tags))

		}
	}, 5*time.Minute, 3*time.Second)
}
