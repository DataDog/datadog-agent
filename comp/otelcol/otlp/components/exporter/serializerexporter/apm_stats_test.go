// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test

package serializerexporter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/featuregates"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/util/otel"
)

var sp = &pb.StatsPayload{
	Stats: []*pb.ClientStatsPayload{
		{
			Hostname:         "host",
			Env:              "prod",
			Version:          "v1.2",
			Lang:             "go",
			TracerVersion:    "v44",
			RuntimeID:        "123jkl",
			Sequence:         2,
			AgentAggregation: "blah",
			Service:          "mysql",
			ContainerID:      "abcdef123456",
			Tags:             []string{"a:b", "c:d"},
			Stats: []*pb.ClientStatsBucket{
				{
					Start:    10,
					Duration: 1,
					Stats: []*pb.ClientGroupedStats{
						{
							Service:        "kafka",
							Name:           "queue.add",
							Resource:       "append",
							HTTPStatusCode: 220,
							Type:           "queue",
							Hits:           15,
							Errors:         3,
							Duration:       143,
							OkSummary:      nil,
							ErrorSummary:   nil,
							TopLevelHits:   5,
						},
					},
				},
			},
		},
	},
}

func testAPMStatsMetric(t *testing.T) pmetric.Metrics {
	attributesTranslator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	require.NoError(t, err)
	//nolint:staticcheck // Using deprecated NewTranslator to access StatsToMetrics for test
	tr, err := metrics.NewTranslator(componenttest.NewNopTelemetrySettings(), attributesTranslator)
	require.NoError(t, err)
	m, err := tr.StatsToMetrics(sp)
	require.NoError(t, err)
	return m
}

func TestAPMStats_OSS(t *testing.T) {
	statsIn := make(chan []byte, 1000)
	factory := NewFactoryForOSSExporter(component.MustNewType("datadog"), statsIn)
	testAPMStats(t, factory, statsIn)
}

func TestAPMStats_OTelAgent(t *testing.T) {
	statsIn := make(chan []byte, 1000)
	factory := NewFactoryForOTelAgent(&metricRecorder{}, func(context.Context) (string, error) {
		return "", nil
	}, statsIn, nil, otel.NewDisabledGatewayUsage(), TelemetryStore{}, nil)
	testAPMStats(t, factory, statsIn)
}

// End-to-end: the agent OTLP-ingest exporter posts an SDK duration histogram to /v0.6/stats.
func TestSDKTraceStats_AgentOTLPIngest(t *testing.T) {
	type result struct {
		payload     *pb.OTLPIntakeStatsPayload
		path        string
		contentType string
		protocol    string
		err         error
	}
	results := make(chan result, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		payload := &pb.OTLPIntakeStatsPayload{}
		body, err := io.ReadAll(req.Body)
		if err == nil {
			err = proto.Unmarshal(body, payload)
		}
		results <- result{
			payload: payload, path: req.URL.Path,
			contentType: req.Header.Get("Content-Type"), protocol: req.Header.Get("Dd-Protocol"), err: err,
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	registry := featuregate.GlobalRegistry()
	previous := featuregates.DisableMetricRemappingFeatureGate.IsEnabled()
	require.NoError(t, registry.Set(featuregates.DisableMetricRemappingFeatureGate.ID(), true))
	t.Cleanup(func() {
		require.NoError(t, registry.Set(featuregates.DisableMetricRemappingFeatureGate.ID(), previous))
	})

	factory := NewFactoryForAgent(&metricRecorder{}, func(context.Context) (string, error) { return "agent-host", nil }, TelemetryStore{})
	cfg := factory.CreateDefaultConfig().(*ExporterConfig)
	cfg.Metrics.APMStatsReceiverAddr = server.URL + "/v0.6/stats"
	cfg.QueueBatchConfig = configoptional.None[exporterhelper.QueueBatchConfig]()
	cfg.RetryConfig.Enabled = false
	ctx := context.Background()
	exp, err := factory.CreateMetrics(ctx, exportertest.NewNopSettings(factory.Type()), cfg)
	require.NoError(t, err)
	require.NoError(t, exp.Start(ctx, componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, exp.Shutdown(ctx)) })

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "checkout")
	metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("traces.span.sdk.metrics.duration")
	metric.SetUnit("s")
	histogram := metric.SetEmptyHistogram()
	histogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := histogram.DataPoints().AppendEmpty()
	dp.SetCount(2)
	dp.SetSum(1)
	dp.BucketCounts().FromRaw([]uint64{2})
	dp.Attributes().PutStr("datadog.operation.name", "http.request")
	dp.Attributes().PutStr("span.name", "GET /checkout")

	require.NoError(t, exp.ConsumeMetrics(ctx, md))
	got := <-results
	require.NoError(t, got.err)
	require.Equal(t, "/v0.6/stats", got.path)
	require.Equal(t, "application/x-protobuf", got.contentType)
	require.Equal(t, "otlp", got.protocol)
	require.Equal(t, "otlp-intake-metrics-sdk", got.payload.Source)
	require.True(t, got.payload.Aggregate)
	require.Len(t, got.payload.Stats, 1)
	require.Len(t, got.payload.Stats[0].Stats, 1)
	require.Equal(t, "http.request", got.payload.Stats[0].Stats[0].Name)
	require.Equal(t, "checkout", got.payload.Stats[0].Stats[0].Service)
	require.Equal(t, uint64(2), got.payload.Stats[0].Stats[0].Hits)
}

func testAPMStats(t *testing.T, factory exporter.Factory, statsIn chan []byte) {
	cfg, ok := factory.CreateDefaultConfig().(*ExporterConfig)
	require.True(t, ok)
	cfg.ShutdownFunc = func(_ context.Context) error {
		close(statsIn)
		return nil
	}
	ctx := context.Background()
	set := exportertest.NewNopSettings(factory.Type())
	exp, err := factory.CreateMetrics(ctx, set, cfg)
	require.NoError(t, err)
	require.NoError(t, exp.Start(ctx, componenttest.NewNopHost()))
	md := testAPMStatsMetric(t)
	require.NoError(t, exp.ConsumeMetrics(ctx, md))
	require.NoError(t, exp.Shutdown(ctx))
	require.Len(t, statsIn, 1)
	msg := <-statsIn
	got := &pb.StatsPayload{}
	require.NoError(t, proto.Unmarshal(msg, got))
	if diff := cmp.Diff(
		sp,
		got,
		protocmp.Transform()); diff != "" {
		t.Errorf("Diff between APM stats -want +got:\n%v", diff)
	}
}
