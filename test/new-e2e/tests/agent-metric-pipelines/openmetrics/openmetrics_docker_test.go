// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package openmetrics

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dockerOpenMetricsSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestDockerOpenMetricsCoreLoaderSuite(t *testing.T) {
	t.Parallel()

	compose := strings.ReplaceAll(openMetricsCompose, "{APPS_VERSION}", apps.Version)
	e2e.Run(t, &dockerOpenMetricsSuite{}, e2e.WithProvisioner(
		awsdocker.Provisioner(
			awsdocker.WithRunOptions(
				scendocker.WithAgentOptions(
					dockeragentparams.WithAgentServiceEnvVariable("DD_OPENMETRICS_USE_CORE_LOADER", pulumi.StringPtr("true")),
					dockeragentparams.WithExtraComposeManifest("openmetrics", pulumi.String(compose)),
				),
			),
		),
	))
}

func (s *dockerOpenMetricsSuite) TestAutodiscoveryInstancesUseCoreLoaderWithAgentFlag() {
	t := s.T()
	require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assertMetric(c, s, "openmetrics_e2e_one.prom_gauge")
		assertMetric(c, s, "openmetrics_e2e_two.prom_gauge")
	}, 5*time.Minute, 10*time.Second)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		telemetry := s.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "agent-full-telemetry"}))
		assertLoadedCounterAtLeastTwo(c, telemetry)
		assert.NotContains(c, telemetry, `openmetrics_core__configure_total{outcome="fallback"`)
	}, time.Minute, 5*time.Second)
}

func assertMetric(c *assert.CollectT, s *dockerOpenMetricsSuite, metricName string) {
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(metricName)
	assert.NoError(c, err)
	assert.NotEmpty(c, metrics, "no %s metrics found", metricName)
}

func assertLoadedCounterAtLeastTwo(c *assert.CollectT, telemetry string) {
	pattern := regexp.MustCompile(`openmetrics_core__configure_total\{outcome="loaded",reason="none"\}\s+(?:[2-9]|[1-9][0-9]+)(?:\.[0-9]+)?`)
	assert.Regexp(c, pattern, telemetry)
}

const openMetricsCompose = `
version: "3.9"
services:
  openmetrics-one:
    image: ghcr.io/datadog/apps-prometheus:{APPS_VERSION}
    container_name: openmetrics-one-e2e
    labels:
      com.datadoghq.ad.checks: |
        {
          "openmetrics": {
            "init_config": {},
            "instances": [
              {
                "openmetrics_endpoint": "http://%%host%%:8080/metrics",
                "namespace": "openmetrics_e2e_one",
                "metrics": ["prom_gauge"]
              }
            ]
          }
        }

  openmetrics-two:
    image: ghcr.io/datadog/apps-prometheus:{APPS_VERSION}
    container_name: openmetrics-two-e2e
    labels:
      com.datadoghq.ad.checks: |
        {
          "openmetrics": {
            "init_config": {},
            "instances": [
              {
                "openmetrics_endpoint": "http://%%host%%:8080/metrics",
                "namespace": "openmetrics_e2e_two",
                "metrics": ["prom_gauge"]
              }
            ]
          }
        }
`
