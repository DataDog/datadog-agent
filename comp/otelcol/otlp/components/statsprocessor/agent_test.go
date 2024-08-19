// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package statsprocessor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/otel/sdk/metric"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
)

func setupMetricClient() (*metric.ManualReader, statsd.ClientInterface, timing.Reporter) {
	reader := metric.NewManualReader()
	meterProvider := metric.NewMeterProvider(metric.WithReader(reader))
	metricClient := metricsclient.InitializeMetricClient(meterProvider, metricsclient.ExporterSourceTag)
	timingReporter := timing.New(metricClient)
	return reader, metricClient, timingReporter
}

func TestTraceAgentConfig(t *testing.T) {
	cfg := traceconfig.New()
	require.NotZero(t, cfg.ReceiverPort)
	require.True(t, cfg.ReceiverEnabled)

	out := make(chan *pb.StatsPayload)
	_, metricClient, timingReporter := setupMetricClient()
	_ = NewAgentWithConfig(context.Background(), cfg, out, metricClient, timingReporter)
	require.False(t, cfg.ReceiverEnabled)
	require.NotEmpty(t, cfg.Endpoints[0].APIKey)
	require.Equal(t, "__unset__", cfg.Hostname)
}

func TestTraceAgent(t *testing.T) {
	cfg := traceconfig.New()
	attributesTranslator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	require.NoError(t, err)
	cfg.OTLPReceiver.AttributesTranslator = attributesTranslator
	cfg.BucketInterval = 50 * time.Millisecond
	out := make(chan *pb.StatsPayload, 10)
	ctx := context.Background()
	_, metricClient, timingReporter := setupMetricClient()
	a := NewAgentWithConfig(ctx, cfg, out, metricClient, timingReporter)
	a.Start()
	defer a.Stop()

	traces := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
		{
			LibName:    "libname",
			LibVersion: "1.2",
			Attributes: map[string]any{},
			Spans: []*testutil.OTLPSpan{
				{Name: "1"},
				{Name: "2"},
				{Name: "3"},
			},
		},
		{
			LibName:    "other-libname",
			LibVersion: "2.1",
			Attributes: map[string]any{},
			Spans: []*testutil.OTLPSpan{
				{Name: "4", TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
				{Name: "5", TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}},
			},
		},
	}).Traces()

	a.Ingest(ctx, traces)
	var stats *pb.StatsPayload
	timeout := time.After(5 * time.Second)
	var actual []*pb.ClientGroupedStats
	// Depending on the time bucket boundaries, the stats can come in one or multiple payloads.
	// Wait until all payloads are received.
	for len(actual) < traces.SpanCount() {
		select {
		case stats = <-out:
			if len(stats.Stats) != 0 {
				require.Len(t, stats.Stats, 1)
				require.Len(t, stats.Stats[0].Stats, 1)
				assert.Greater(t, len(stats.Stats[0].Stats[0].Stats), 0)
				actual = append(actual, stats.Stats[0].Stats[0].Stats...)
			}
		case <-timeout:
			t.Fatal("timed out")
		}
	}

	// considering all spans in rspans have distinct aggregations, we should have an equal amount
	// of groups
	require.Len(t, actual, traces.SpanCount())
}
