// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ccmmode

import (
	"fmt"
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

const infrastructureModeTag = "infra_mode:cloud_cost_only"

type ccmModeSuiteBase struct {
	e2e.BaseSuite[environments.Host]
}

type ccmModeDefaultTaggedSuite struct {
	ccmModeSuiteBase
}

type ccmModeConfiguredTaggedSuite struct {
	ccmModeSuiteBase
}

func yamlIndentedList(items []string) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "      - %s\n", item)
	}
	return b.String()
}

func ccmAgentConfig(taggedChecks []string) string {
	var cfg strings.Builder
	cfg.WriteString(`
infrastructure_mode: cloud_cost_only
`)
	if len(taggedChecks) > 0 {
		cfg.WriteString(`integration:
  cloud_cost_only:
    tagged:
`)
		cfg.WriteString(yamlIndentedList(taggedChecks))
	}
	return cfg.String()
}

func runCCMModeSuite[T e2e.Suite[environments.Host]](t *testing.T, stackName string, agentConfig string, suite T) {
	t.Helper()
	t.Parallel()

	agentOptions := []agentparams.Option{agentparams.WithAgentConfig(agentConfig)}

	e2e.Run(t, suite, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(agentOptions...),
			),
		),
	), e2e.WithStackName(stackName))
}

func (s *ccmModeSuiteBase) assertMetricHasInfrastructureModeTag(c *assert.CollectT, metricName, checkName string) {
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
		metricName,
		client.WithTags[*aggregator.MetricSeries]([]string{infrastructureModeTag}),
		client.WithMetricValueHigherThan(0),
	)
	assert.NoError(c, err)
	assert.NotEmpty(c, metrics, "%s from %s check should be forwarded with %s", metricName, checkName, infrastructureModeTag)
}

func (s *ccmModeSuiteBase) assertMetricLacksInfrastructureModeTag(c *assert.CollectT, metricName, checkName string) {
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(
		metricName,
		client.WithMetricValueHigherThan(0),
	)
	assert.NoError(c, err)
	assert.NotEmpty(c, metrics, "%s from %s check should be forwarded", metricName, checkName)
	for _, m := range metrics {
		assert.NotContains(c, m.GetTags(), infrastructureModeTag, "%s from %s check should not carry %s", metricName, checkName, infrastructureModeTag)
	}
}

// TestCCMModeLinuxDefaultTagged runs CCM e2e checks with the default empty
// integration.cloud_cost_only.tagged list (all checks receive infrastructure_mode).
func TestCCMModeLinuxDefaultTagged(t *testing.T) {
	runCCMModeSuite(t, "ccmmode-default-tagged", ccmAgentConfig(nil), &ccmModeDefaultTaggedSuite{})
}

// TestCCMModeLinuxConfiguredTagged runs CCM e2e checks with an explicit tagged list.
func TestCCMModeLinuxConfiguredTagged(t *testing.T) {
	runCCMModeSuite(t, "ccmmode-configured-tagged", ccmAgentConfig([]string{"cpu"}), &ccmModeConfiguredTaggedSuite{})
}

// TestDefaultTaggedAllChecksReceiveInfrastructureModeTag verifies the default empty tagged list
// tags metrics from multiple integrations, including checks outside a typical tagged: [cpu] config.
func (s *ccmModeDefaultTaggedSuite) TestDefaultTaggedAllChecksReceiveInfrastructureModeTag() {
	metricsByCheck := []struct {
		metric string
		check  string
	}{
		{"system.cpu.user", "cpu"},
		{"system.net.bytes_rcvd", "network"},
		{"system.mem.pct_usable", "memory"},
	}

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		for _, tc := range metricsByCheck {
			s.assertMetricHasInfrastructureModeTag(c, tc.metric, tc.check)
		}
	}, 3*time.Minute, 10*time.Second, "timed out waiting for default-tagged metrics")
}

// TestConfiguredTaggedAppliesSelectively verifies integration.cloud_cost_only.tagged limits
// which checks receive infrastructure_mode when the list is non-empty.
func (s *ccmModeConfiguredTaggedSuite) TestConfiguredTaggedAppliesSelectively() {
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		s.assertMetricHasInfrastructureModeTag(c, "system.cpu.user", "cpu")
		s.assertMetricLacksInfrastructureModeTag(c, "system.net.bytes_rcvd", "network")
	}, 3*time.Minute, 10*time.Second, "timed out waiting for configured selective tagging")
}
