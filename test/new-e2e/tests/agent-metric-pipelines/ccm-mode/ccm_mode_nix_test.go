// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ccmmode

import (
	"slices"
	"strings"
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

const ccmTag = "ccm_mode:lightweight"

type ccmModeSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestCCMModeLinux runs CCM tagging e2e checks on a Linux EC2 host with fakeintake.
func TestCCMModeLinux(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &ccmModeSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(`
ccm_mode: lightweight
integration:
  ccm_lightweight:
    tagged:
      - cpu
`),
				),
			),
		),
	))
}

// TestTaggedCoreCheckMetricsIncludeCCMModeTag verifies metrics from the cpu core check
// carry the ccm_mode tag when that integration is listed under integration.ccm_lightweight.tagged.
func (s *ccmModeSuite) TestTaggedCoreCheckMetricsIncludeCCMModeTag() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			"system.cpu.user",
			client.WithTags[*aggregator.MetricSeries]([]string{ccmTag}),
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "expected system.cpu.user with %s on fakeintake", ccmTag)
	}, 3*time.Minute, 10*time.Second, "timed out waiting for tagged cpu metrics")
}

// TestUntaggedCoreCheckMetricsExcludeCCMModeTag verifies metrics from the disk core check
// do not include ccm_mode when only the cpu integration is listed as tagged.
func (s *ccmModeSuite) TestUntaggedCoreCheckMetricsExcludeCCMModeTag() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
			"system.disk.free",
			client.WithMetricValueHigherThan(0),
		)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics, "expected system.disk.free metrics for negative tagging assertion")
		for _, m := range metrics {
			assert.False(c, slices.ContainsFunc(m.Tags, func(tag string) bool {
				return strings.HasPrefix(tag, "ccm_mode:")
			}), "disk metrics should not include ccm_mode tag; tags=%v", m.Tags)
		}
	}, 3*time.Minute, 10*time.Second, "timed out waiting for disk metrics without ccm_mode tag")
}
