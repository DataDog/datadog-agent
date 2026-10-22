// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelagent

import (
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

const (
	sketchesV3Endpoint = "/api/intake/metrics/v3/sketches"
	// Float64Histogram the calendar app records on every request. The app exports histograms
	// with delta temporality and base2 exponential aggregation, see
	// apps/rest-services/golang/calendar in DataDog/opentelemetry-examples.
	calendarHistogram = "calendar-rest-go.task.duration"
)

type nativeHistogramsTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/minimal.yml
var nativeHistogramsConfig string

var nativeHistogramsParams = utils.IAParams{
	InfraAttributes: true,
	EKS:             false,
	Cardinality:     types.LowCardinality,
}

// TestOTelAgentNativeHistograms deploys DDOT next to the core Agent through the Helm chart
// with the NativeOTelHistograms feature gate, and checks that OTel histograms reach the v3
// sketches intake as is.
//
// The gate needs the v3 sketches intake for every metrics destination, or DDOT refuses to
// start, so the deployment also exercises the destination DDOT ends up with once the core
// Agent configuration is merged in. Its own suite type keeps it on a separate Pulumi stack.
func TestOTelAgentNativeHistograms(t *testing.T) {
	// The framework sets DD_DD_URL to the fakeintake URL in datadog.env, which comes first in
	// the otel-agent container env, so Kubernetes expands $(DD_DD_URL) to it.
	values := `
datadog:
  otelCollector:
    useStandaloneImage: false
    featureGates: datadog.serializerexporter.NativeOTelHistograms
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
agents:
  containers:
    otelAgent:
      env:
        - name: DD_SERIALIZER_EXPERIMENTAL_USE_V3_API_SKETCHES_ENDPOINTS
          value: '$(DD_DD_URL)'
`
	t.Parallel()
	e2e.Run(t, &nativeHistogramsTestSuite{},
		e2e.WithProvisioner(provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				scenkindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(values),
					kubernetesagentparams.WithOTelAgent(),
					kubernetesagentparams.WithOTelConfig(nativeHistogramsConfig),
				),
			),
		)),
	)
}

func (s *nativeHistogramsTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.TestCalendarApp(s, false, utils.CalendarService)
}

// TestHistogramSentAsIs checks that the calendar app histogram, a delta exponential
// histogram, reaches the intake as is rather than as a DDSketch.
func (s *nativeHistogramsTestSuite) TestHistogramSentAsIs() {
	tags := []string{"service:" + utils.CalendarService}
	s.EventuallyWithT(func(c *assert.CollectT) {
		histograms, err := s.Env().FakeIntake.Client().FilterHistograms(calendarHistogram, fakeintake.WithTags[*aggregator.Histogram](tags))
		require.NoError(c, err)
		require.NotEmpty(c, histograms, "no %s histogram received yet", calendarHistogram)
		for _, h := range histograms {
			assert.Empty(c, h.Explicit, "%s is an exponential histogram", calendarHistogram)
			require.NotEmpty(c, h.Exponential)
			hasHost := false
			for _, r := range h.Resources {
				hasHost = hasHost || (r.Type == "host" && r.Name != "")
			}
			assert.True(c, hasHost, "%s has no host resource", calendarHistogram)
			for _, p := range h.Exponential {
				total := p.ZeroCount
				for _, n := range p.Positive {
					total += n
				}
				for _, n := range p.Negative {
					total += n
				}
				assert.Equal(c, p.Count, total, "bucket counts must add up to the count")
				// The app's SDK records them, and no processor converts delta points.
				assert.NotNil(c, p.Sum, "sum")
				assert.NotNil(c, p.Min, "min")
				assert.NotNil(c, p.Max, "max")
			}
		}
	}, 5*time.Minute, 10*time.Second)

	sketches, err := s.Env().FakeIntake.Client().FilterSketches(calendarHistogram)
	require.NoError(s.T(), err)
	assert.Empty(s.T(), sketches, "%s must not be converted to a DDSketch", calendarHistogram)
}

// TestHistogramsCompression checks that the v3 sketches payloads are compressed with zstd,
// the default `serializer_compressor_kind`.
func (s *nativeHistogramsTestSuite) TestHistogramsCompression() {
	utils.TestCompression(s, []utils.SignalCompression{
		{Name: "metrics (v3 sketches)", Endpoint: sketchesV3Endpoint, Encoding: "zstd", Required: true},
	})
}

// TestOTLPMetrics checks that the other metrics keep reaching the intake: the gate swaps the
// metrics translator for every metric, not only histograms.
func (s *nativeHistogramsTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s, nativeHistogramsParams)
}

// TestPrometheusMetrics checks that DDOT's own metrics keep reaching the intake. The
// converter adds a pipeline that scrapes them with the Prometheus receiver, and most of them
// are cumulative, which the translator selected by the gate drops unless converted to delta.
func (s *nativeHistogramsTestSuite) TestPrometheusMetrics() {
	utils.TestPrometheusMetrics(s)
}
