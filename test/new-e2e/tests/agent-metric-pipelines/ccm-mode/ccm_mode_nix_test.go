// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ccmmode

import (
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
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

const infraModeTag = "infra_mode:cloud_cost_only"

type ccmModeSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestCCMModeLinux runs cloud-cost-only infra tagging e2e checks on a Linux EC2 host with fakeintake.
func TestCCMModeLinux(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ccmModeSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(`
infrastructure_mode: cloud_cost_only
integration:
  cloud_cost_only:
    tagged:
      - cpu
`),
				),
			),
		),
	))
}

// TestTaggedCoreCheckMetricsIncludeCCMModeTag verifies metrics from the cpu core check
// carry the infra_mode tag when that integration is listed under integration.cloud_cost_only.tagged.
func (s *ccmModeSuite) TestTaggedCoreCheckMetricsIncludeCCMModeTag() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			"system.cpu.user",
			client.WithTags[*aggregator.MetricSeries]([]string{infraModeTag}),
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "expected system.cpu.user with %s on fakeintake", infraModeTag)
	}, 3*time.Minute, 10*time.Second, "timed out waiting for tagged cpu metrics")
}

// TestNonAllowlistedMetricsDropped verifies metrics outside integration.cloud_cost_only.metrics
// are not forwarded when infrastructure_mode is cloud_cost_only.
func (s *ccmModeSuite) TestNonAllowlistedMetricsDropped() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			"system.disk.free",
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.Empty(c, metrics, "system.disk.free should be dropped by the cloud_cost_only metric allowlist")
	}, 3*time.Minute, 10*time.Second, "timed out waiting for allowlist to drop disk metrics")
}
