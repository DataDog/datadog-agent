// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlpingest

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

const (
	otlpMetricsV2Endpoint = "/api/v2/series"
	otlpMetricsV3Endpoint = "/api/intake/metrics/v3/series"
)

type otlpIngestDockerTestSuite struct {
	e2e.BaseSuite[environments.DockerHost]

	v3Enabled bool
}

//go:embed compose/otlp_ingest_compose.yaml
var otlpIngestCompose string

func testOTLPIngestDocker(t *testing.T, v3Enabled bool) {
	t.Parallel()

	agentOptions := []dockeragentparams.Option{
		dockeragentparams.WithLogs(),
		dockeragentparams.WithAgentServiceEnvVariable("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT", pulumi.StringPtr("0.0.0.0:4317")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT", pulumi.StringPtr("0.0.0.0:4318")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_LOGS_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_OTLP_CONFIG_LOGS_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL", pulumi.StringPtr("false")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_OTLP_CONFIG_METRICS_RESOURCE_ATTRIBUTES_AS_TAGS", pulumi.StringPtr("true")),
		dockeragentparams.WithExtraComposeManifest("calendar-rest-go", pulumi.String(strings.ReplaceAll(otlpIngestCompose, "{APPS_VERSION}", apps.Version))),
	}
	if v3Enabled {
		// The test fakeintake URL is not a Datadog intake URL, so explicitly opt in
		// to V3 instead of relying on the default datadog_only mode.
		agentOptions = append(agentOptions,
			dockeragentparams.WithAgentServiceEnvVariable("DD_USE_V3_API_SERIES_ENABLED", pulumi.StringPtr("true")))
	} else {
		// Force V2 explicitly to exercise the V2 wire format.
		agentOptions = append(agentOptions, dockeragentparams.WithV3MetricsDisabled())
	}

	stackName := "otlpingestdocker"
	if v3Enabled {
		stackName += "-v3"
	}

	e2e.Run(t,
		&otlpIngestDockerTestSuite{v3Enabled: v3Enabled},
		e2e.WithProvisioner(
			awsdocker.Provisioner(
				awsdocker.WithRunOptions(
					ec2docker.WithAgentOptions(agentOptions...),
				),
			),
		),
		e2e.WithStackName(stackName),
	)
}

// TestOTLPIngestDocker runs the OTLP ingest Docker e2e test with the V2 metrics intake API.
func TestOTLPIngestDocker(t *testing.T) {
	testOTLPIngestDocker(t, false)
}

// TestOTLPIngestDockerV3 runs the OTLP ingest Docker e2e test with the V3 metrics intake API.
// Metric assertions are identical to the V2 variant; the test additionally verifies that
// payloads were routed to /api/intake/metrics/v3/series and not /api/v2/series.
func TestOTLPIngestDockerV3(t *testing.T) {
	testOTLPIngestDocker(t, true)
}

func (s *otlpIngestDockerTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarAppDocker(s)
}

func (s *otlpIngestDockerTestSuite) TestOTLPTraces() {
	utils.TestTracesDocker(s)
}

func (s *otlpIngestDockerTestSuite) TestOTLPMetrics() {
	utils.TestMetricsDocker(s)

	// Verify routing: each mode must use exactly its intended series endpoint.
	routeStats, err := s.Env().FakeIntake.Client().RouteStats()
	require.NoError(s.T(), err)
	if s.v3Enabled {
		assert.Greater(s.T(), routeStats[otlpMetricsV3Endpoint], 0,
			"expected payloads on %s when V3 is enabled", otlpMetricsV3Endpoint)
	} else {
		assert.Greater(s.T(), routeStats[otlpMetricsV2Endpoint], 0,
			"expected payloads on %s when V3 is not enabled", otlpMetricsV2Endpoint)
		assert.Zero(s.T(), routeStats[otlpMetricsV3Endpoint],
			"expected no payloads on %s when V3 is not enabled", otlpMetricsV3Endpoint)
	}
}

func (s *otlpIngestDockerTestSuite) TestOTLPLogs() {
	utils.TestLogsDocker(s)
}
