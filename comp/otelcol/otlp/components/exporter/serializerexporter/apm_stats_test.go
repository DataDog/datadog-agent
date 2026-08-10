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
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/featuregates"
	"github.com/stretchr/testify/assert"
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
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/sdktracestats"
	otlpstatspb "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/sdktracestats/pb"

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

func sdkTraceStatsMetrics() pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "checkout")
	rm.Resource().Attributes().PutStr("telemetry.sdk.name", "datadog")
	metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName(sdktracestats.SDKTraceMetricName)
	metric.SetUnit("s")
	histogram := metric.SetEmptyHistogram()
	histogram.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := histogram.DataPoints().AppendEmpty()
	dp.SetCount(2)
	dp.SetSum(1)
	dp.BucketCounts().FromRaw([]uint64{2})
	dp.Attributes().PutStr("datadog.operation.name", "http.request")
	return md
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

func TestSDKTraceStats_AgentOTLPIngest(t *testing.T) {
	results := make(chan *otlpstatspb.OTLPIntakeStatsPayload, 1)
	var requests atomic.Int64
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer req.Body.Close()
		assert.Equal(t, "/v0.6/stats", req.URL.Path)
		assert.Equal(t, "application/x-protobuf", req.Header.Get("Content-Type"))
		assert.Equal(t, "otlp", req.Header.Get("Dd-Protocol"))
		payload := &otlpstatspb.OTLPIntakeStatsPayload{}
		body, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		assert.NoError(t, proto.Unmarshal(body, payload))
		results <- payload
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
	cfg.RetryConfig.InitialInterval = time.Millisecond
	cfg.RetryConfig.MaxInterval = time.Millisecond
	cfg.RetryConfig.MaxElapsedTime = 20 * time.Millisecond
	ctx := context.Background()
	exp, err := factory.CreateMetrics(ctx, exportertest.NewNopSettings(factory.Type()), cfg)
	require.NoError(t, err)
	require.NoError(t, exp.Start(ctx, componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, exp.Shutdown(ctx)) })

	require.NoError(t, exp.ConsumeMetrics(ctx, sdkTraceStatsMetrics()))
	got := <-results
	require.Equal(t, "otlp-intake-metrics", got.Source)
	require.NotContains(t, got.HostTags, "span_source:datadog")
	require.Len(t, got.Stats, 1)
	require.Len(t, got.Stats[0].Stats, 1)
	require.Equal(t, "http.request", got.Stats[0].Stats[0].Name)
	require.Equal(t, uint64(2), got.Stats[0].Stats[0].Hits)

	fail.Store(true)
	require.NoError(t, exp.ConsumeMetrics(ctx, sdkTraceStatsMetrics()))
	require.Equal(t, int64(2), requests.Load(), "failed APM export must not retry the metrics batch")
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
