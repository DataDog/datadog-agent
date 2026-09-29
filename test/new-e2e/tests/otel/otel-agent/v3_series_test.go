// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

const (
	seriesV2Endpoint = "/api/v2/series"
	seriesV3Endpoint = "/api/intake/metrics/v3/series"
)

type v3SeriesTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/minimal.yml
var v3SeriesConfig string

var v3SeriesParams = utils.IAParams{
	InfraAttributes: true,
	EKS:             false,
	Cardinality:     types.LowCardinality,
}

// TestOTelAgentV3Series provisions a DDOT deployment with the v3 series metrics
// intake enabled on the otel-agent, then asserts OTLP series are routed to the
// v3 endpoint (rather than v2) and shipped zstd-compressed.
//
// This is a sibling entry point of the other otel-agent suites: it reuses the
// shared pipeline assertions (utils.*) but provisions a distinct agent config
// (DD_USE_V3_API_SERIES_ENABLED=true). Its own suite type keeps it on a separate
// Pulumi stack from the default (v2) suites running in parallel.
func TestOTelAgentV3Series(t *testing.T) {
	// The test fakeintake URL is not a Datadog intake URL, so the default
	// use_v3_api.series.enabled=datadog_only would keep the otel-agent on the v2
	// series intake. Explicitly opt in to v3 to exercise the v3 wire format —
	// this also relies on the env var being honored, which is only true because
	// DDOT no longer forces use_v3_api.series.enabled at SourceAgentRuntime.
	values := `
datadog:
  otelCollector:
    useStandaloneImage: false
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
agents:
  containers:
    otelAgent:
      env:
        - name: DD_USE_V3_API_SERIES_ENABLED
          value: 'true'
        - name: DD_APM_FEATURES
          value: 'disable_operation_and_resource_name_logic_v2'
`
	t.Parallel()
	e2e.Run(t, &v3SeriesTestSuite{},
		e2e.WithProvisioner(provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				scenkindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(values),
					kubernetesagentparams.WithOTelAgent(),
					kubernetesagentparams.WithOTelConfig(v3SeriesConfig),
				),
			),
		)),
	)
}

func (s *v3SeriesTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

// TestOTLPMetricsV3 verifies OTLP metrics reach the intake as usual AND that the
// otel-agent routed series to the v3 endpoint. Only v3 is asserted (not v2==0):
// the co-located core Agent legitimately ships its own series to v2 under the
// default datadog_only mode (fakeintake is not a Datadog URL), so v2 payloads are
// expected. Series on the v3 endpoint can only come from the v3-enabled
// otel-agent, so v3 > 0 uniquely proves the DDOT series moved to v3.
func (s *v3SeriesTestSuite) TestOTLPMetricsV3() {
	utils.TestMetrics(s, v3SeriesParams)

	routeStats, err := s.Env().FakeIntake.Client().RouteStats()
	require.NoError(s.T(), err)
	assert.Greater(s.T(), routeStats[seriesV3Endpoint], 0,
		"expected series payloads on %s when v3 is enabled for the otel-agent", seriesV3Endpoint)
}

// TestV3SeriesCompression verifies the v3 series payloads are zstd-compressed on
// the wire — the v3 intake only accepts zstd, so this guards the compressor
// pairing that makes v3 viable for DDOT.
func (s *v3SeriesTestSuite) TestV3SeriesCompression() {
	utils.TestCompression(s, []utils.SignalCompression{
		{Name: "metrics (v3 series)", Endpoint: seriesV3Endpoint, Encoding: "zstd", Required: true},
	})
}
