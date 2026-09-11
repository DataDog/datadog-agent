// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sdc

import (
	_ "embed"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

const (
	gaugeMetric = "e2e.sdc.gauge"
	rateMetric  = "e2e.sdc.rate"

	agentConfig = `
checks:
  sdc_compression_checks:
    - custom_sdc
`
	checkConfig = `
init_config:
instances:
  - min_collection_interval: 1
`
)

//go:embed fixtures/custom_sdc.py
var customCheck string

type sdcSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestSDC(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &sdcSuite{},
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(agentConfig),
					agentparams.WithIntegration("custom_sdc.d", checkConfig),
					agentparams.WithFile("/etc/datadog-agent/checks.d/custom_sdc.py", customCheck, true),
				),
			),
		)),
	)
}

func pointCount(series []*fakeintake.MetricSeries) int {
	count := 0
	for _, serie := range series {
		count += len(serie.Points)
	}
	return count
}

func assertSerieMetadata(c *assert.CollectT, series []*fakeintake.MetricSeries) {
	for _, serie := range series {
		hasHost := false
		for _, resource := range serie.Resources {
			if resource.Type == "host" && resource.Name != "" {
				hasHost = true
				break
			}
		}
		assert.True(c, hasHost, "metric serie should retain its host resource: %v", serie.Resources)
		assert.True(c, slices.Contains(serie.Tags, "compression:sdc"), "metric serie should retain check tags: %v", serie.Tags)
		assert.True(c, slices.Contains(serie.Tags, "fixture:custom-check"), "metric serie should retain all check tags: %v", serie.Tags)
	}
}

func (s *sdcSuite) TestCompressedGaugeReachesFakeintake() {
	s.EventuallyWithT(func(c *assert.CollectT) {
		gauges, err := s.Env().FakeIntake.Client().FilterMetrics(gaugeMetric)
		require.NoError(c, err)
		require.NotEmpty(c, gauges, "compressed gauge has not reached fakeintake")

		rates, err := s.Env().FakeIntake.Client().FilterMetrics(rateMetric)
		require.NoError(c, err)
		require.NotEmpty(c, rates, "uncompressed control rate has not reached fakeintake")

		gaugePoints := pointCount(gauges)
		ratePoints := pointCount(rates)
		require.GreaterOrEqual(c, ratePoints, 5, "not enough check runs have reached fakeintake to evaluate compression")
		assert.Less(c, gaugePoints, ratePoints, "flat gauges should contain fewer points than the uncompressed rate control")

		for _, gauge := range gauges {
			for _, point := range gauge.Points {
				assert.Equal(c, 42.0, point.Value)
			}
		}
		assertSerieMetadata(c, gauges)
		assertSerieMetadata(c, rates)
	}, 2*time.Minute, 10*time.Second)
}
