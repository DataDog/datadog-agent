// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build test

package datadogexporter

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	datadogconfig "github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/datadogconfig"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/metricsclient"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/tracestats"
	stats "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics/tracestats/pb"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"
	"github.com/DataDog/datadog-agent/pkg/util/otel"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"google.golang.org/protobuf/proto"
)

type sdkTraceComponent struct {
	traceagent.Component
}

type sdkStatsComponent struct {
	writer *writer.DatadogStatsWriter
	sent   int
}

func (sdkTraceComponent) SetOTelAttributeTranslator(*attributes.Translator) {}
func (c *sdkStatsComponent) SendOTLPStatsPayload(raw []byte) {
	c.sent++
	c.writer.SendOTLPIntakePayload(raw)
}

type sdkMetricSerializer struct {
	serializer.MetricSerializer
	mu       sync.Mutex
	names    []string
	attempts int
	retried  chan struct{}
}

func (s *sdkMetricSerializer) SendIterableSeries(src metrics.SerieSource) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts++
	for src.MoveNext() {
		s.names = append(s.names, src.Current().Name)
	}
	if s.attempts == 1 {
		return errors.New("transient serializer failure")
	}
	if s.attempts == 2 {
		close(s.retried)
	}
	return nil
}
func (*sdkMetricSerializer) SendSketch(src metrics.SketchesSource) error {
	for src.MoveNext() {
	}
	return nil
}

// Exercises the public factory, real metrics serializer path, and trace stats HTTP writer.
func TestSDKMetricsExporterIntegration(t *testing.T) {
	for _, queued := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "queued"}[queued], func(t *testing.T) {
			requests := make(chan []byte, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v0.2/stats", r.URL.Path)
				assert.Equal(t, "application/x-protobuf", r.Header.Get("Content-Type"))
				assert.Equal(t, "gzip", r.Header.Get("Content-Encoding"))
				assert.Equal(t, "test-key", r.Header.Get("DD-Api-Key"))
				assert.Equal(t, "otlp", r.Header.Get("Dd-Protocol"))
				reader, err := gzip.NewReader(r.Body)
				if !assert.NoError(t, err) {
					w.WriteHeader(400)
					return
				}
				defer reader.Close()
				raw, err := io.ReadAll(reader)
				assert.NoError(t, err)
				requests <- raw
				w.WriteHeader(http.StatusAccepted)
			}))
			t.Cleanup(server.Close)
			cfg := config.New()
			cfg.Endpoints[0].Host = server.URL
			cfg.Endpoints[0].APIKey = "test-key"
			sw := writer.NewStatsWriter(cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, nil)
			go sw.Run()
			t.Cleanup(func() {
				sw.Stop()
				assert.Len(t, requests, 1, "shutdown drains the SDK-only payload without duplicate delivery")
			})
			trace := &sdkStatsComponent{writer: sw}
			recorder := &sdkMetricSerializer{retried: make(chan struct{})}
			f := NewFactory(sdkTraceComponent{}, trace, recorder, nil, func(context.Context) (string, error) { return "fallback-host", nil }, metricsclient.NewStatsdClientWrapper(&statsd.NoOpClient{}), otel.NewDisabledGatewayUsage(), serializerexporter.TelemetryStore{}, nil)
			ecfg := f.CreateDefaultConfig().(*datadogconfig.Config)
			ecfg.API.Key = "test-key"
			ecfg.HostMetadata.Enabled = false
			queue := exporterhelper.NewDefaultQueueConfig()
			queue.NumConsumers = 1
			ecfg.QueueSettings = configoptional.None[exporterhelper.QueueBatchConfig]()
			if queued {
				ecfg.QueueSettings = configoptional.Some(queue)
			}
			ecfg.BackOffConfig.Enabled = queued
			ecfg.BackOffConfig.InitialInterval = time.Millisecond
			ecfg.BackOffConfig.MaxInterval = time.Millisecond
			ecfg.BackOffConfig.MaxElapsedTime = time.Second
			ctx := context.Background()
			exp, err := f.CreateMetrics(ctx, exportertest.NewNopSettings(Type), ecfg)
			require.NoError(t, err)
			require.True(t, exp.Capabilities().MutatesData)
			require.NoError(t, exp.Start(ctx, componenttest.NewNopHost()))
			t.Cleanup(func() { require.NoError(t, exp.Shutdown(ctx)) })
			md := sdkMetricsContractFixture()
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			require.ErrorIs(t, exp.ConsumeMetrics(canceled, md), context.Canceled)
			require.Zero(t, trace.sent)
			before, err := (&pmetric.ProtoMarshaler{}).MarshalMetrics(md)
			require.NoError(t, err)
			if !queued {
				require.ErrorContains(t, exp.ConsumeMetrics(ctx, md), "transient serializer failure")
				require.Zero(t, trace.sent, "failed mixed batch must not send SDK stats before an upstream retry")
			}
			require.NoError(t, exp.ConsumeMetrics(ctx, md))
			require.Equal(t, 1, trace.sent)
			after, err := (&pmetric.ProtoMarshaler{}).MarshalMetrics(md)
			require.NoError(t, err)
			require.Equal(t, before, after, "fan-out and upstream retries must see the original input")
			select {
			case <-recorder.retried:
			case <-time.After(5 * time.Second):
				t.Fatal("generic export did not retry")
			}

			recorder.mu.Lock()
			names := append([]string(nil), recorder.names...)
			recorder.mu.Unlock()
			assert.Contains(t, names, "control.gauge")
			assert.NotContains(t, names, tracestats.SDKTraceMetricName)
			var raw []byte
			select {
			case raw = <-requests:
			case <-time.After(5 * time.Second):
				t.Fatal("stats did not reach intake")
			}
			payload := &stats.OTLPIntakeStatsPayload{}
			require.NoError(t, proto.Unmarshal(raw, payload))
			assert.Equal(t, "resource-host", payload.HostName)
			assert.Contains(t, payload.HostTags, "team:apm")
			assert.Contains(t, payload.HostTags, "process.executable.name:node")
			require.Len(t, payload.Stats, 2, "malformed sibling is discarded without losing valid points")
			for i, bucket := range payload.Stats {
				assert.Equal(t, int64(time.Second), bucket.Duration)
				require.Len(t, bucket.Stats, 1)
				group := bucket.Stats[0]
				assert.Equal(t, "override-service", group.Service)
				assert.Equal(t, "test", group.Env)
				assert.Equal(t, "1", group.Version)
				assert.Equal(t, "server", group.SpanKind)
				assert.Equal(t, "5", group.GrpcStatusCode)
				assert.True(t, group.Synthetics)
				assert.Equal(t, "http.request", group.Name)
				assert.Equal(t, "GET /items", group.Resource)
				assert.Equal(t, uint64(1), group.Hits)
				assert.Equal(t, uint64(250000000), group.Duration)
				assert.Equal(t, uint64(1), group.TopLevelHits)
				assert.Equal(t, stats.Trilean_FALSE, group.IsTraceRoot)
				assert.Equal(t, []string{"peer.service:inventory"}, group.PeerTags)
				assert.Contains(t, group.OtherTags, &stats.Tag{Name: "custom", Value: "value"})
				sketch := &stats.SparseSketch{}
				if i == 0 {
					assert.Zero(t, group.Errors)
					require.NoError(t, proto.Unmarshal(group.OkSparseSketch, sketch))
				} else {
					assert.Equal(t, uint64(1), group.Errors)
					require.NoError(t, proto.Unmarshal(group.ErrorSparseSketch, sketch))
				}
				require.Len(t, sketch.N, 34)
				assert.Equal(t, math.Float32bits(0.002), sketch.N[1])
				assert.Equal(t, math.Float32bits(15), sketch.N[16])
				assert.Equal(t, uint32(1), sketch.N[25])
				assert.Equal(t, int64(1), sketch.Basic.Count)
			}
			sdkOnly := sdkMetricsContractFixture()
			sdkOnly.ResourceMetrics().RemoveIf(func(rm pmetric.ResourceMetrics) bool { return rm.Resource().Attributes().Len() == 0 })
			require.NoError(t, exp.ConsumeMetrics(ctx, sdkOnly))
			require.Equal(t, 2, trace.sent, "SDK-only batches bypass the generic serializer")
			sdkOnly.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).SetEmptyGauge()
			require.NoError(t, exp.ConsumeMetrics(ctx, sdkOnly))
			require.Equal(t, 2, trace.sent, "wrong-type reserved metric must not leak to ordinary metrics")
			recorder.mu.Lock()
			assert.Equal(t, 2, recorder.attempts)
			recorder.mu.Unlock()

		})
	}
}

// Shape and values from system-tests' OTLP trace-metrics contract (SEMCON-1466).
func sdkMetricsContractFixture() pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	attrs := rm.Resource().Attributes()
	attrs.PutStr("host.name", "resource-host")
	attrs.PutStr("service.name", "resource-service")
	attrs.PutStr("deployment.environment.name", "test")
	attrs.PutStr("service.version", "1")
	attrs.PutEmptySlice("datadog.tracer_tags").AppendEmpty().SetStr("team:apm")
	attrs.PutEmptySlice("datadog.process_tags").AppendEmpty().SetStr("process.executable.name:node")
	metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName(tracestats.SDKTraceMetricName)
	metric.SetUnit("s")
	hist := metric.SetEmptyHistogram()
	hist.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	for _, status := range []string{"STATUS_CODE_OK", "STATUS_CODE_ERROR"} {
		dp := hist.DataPoints().AppendEmpty()
		dp.SetStartTimestamp(pcommon.Timestamp(time.Second))
		dp.SetTimestamp(pcommon.Timestamp(2 * time.Second))
		dp.SetCount(1)
		dp.SetSum(0.25)
		dp.SetMin(0.25)
		dp.SetMax(0.25)
		dp.ExplicitBounds().FromRaw([]float64{.002, .004, .006, .008, .01, .05, .1, .2, .4, .8, 1, 1.4, 2, 5, 10, 15})
		counts := make([]uint64, 17)
		counts[8] = 1
		dp.BucketCounts().FromRaw(counts)
		a := dp.Attributes()
		a.PutStr("service.name", "override-service")
		a.PutStr("span.name", "GET /items")
		a.PutStr("span.kind", "SPAN_KIND_SERVER")
		a.PutStr("datadog.operation.name", "http.request")
		a.PutBool("datadog.span.top_level", true)
		a.PutBool("datadog.is_trace_root", false)
		a.PutStr("datadog.span.type", "web")
		a.PutStr("status.code", status)
		a.PutInt("http.response.status_code", 200)
		a.PutStr("custom", "value")
		a.PutStr("rpc.response.status_code", "NOT_FOUND")
		a.PutStr("datadog.origin", "synthetics")
		a.PutEmptySlice("datadog.peer_tags").AppendEmpty().SetStr("peer.service:inventory")
	}
	hist.DataPoints().AppendEmpty().SetCount(1)
	gauge := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	gauge.SetName("control.gauge")
	dp := gauge.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	dp.SetDoubleValue(42)
	return md
}

type sdkMetricsSink struct {
	exporter.Metrics
	consume func(context.Context, pmetric.Metrics) error
}

func (*sdkMetricsSink) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

func (s *sdkMetricsSink) ConsumeMetrics(ctx context.Context, md pmetric.Metrics) error {
	return s.consume(ctx, md)
}

func TestSDKMetricsExporterOrdinaryPassthrough(t *testing.T) {
	md := pmetric.NewMetrics()
	metric := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("ordinary.gauge")
	metric.SetEmptyGauge().DataPoints().AppendEmpty().SetDoubleValue(1)
	wantErr := errors.New("ordinary export failed")
	exp := &sdkMetricsExporter{Metrics: &sdkMetricsSink{consume: func(ctx context.Context, got pmetric.Metrics) error {
		assert.Equal(t, md, got, "ordinary-only input must pass through without a copy")
		got.ResourceMetrics().At(0).Resource().Attributes().PutBool("serializer.mutated", true)
		return wantErr
	}}}
	require.True(t, exp.Capabilities().MutatesData)
	require.ErrorIs(t, exp.ConsumeMetrics(context.Background(), md), wantErr)
	_, mutated := md.ResourceMetrics().At(0).Resource().Attributes().Get("serializer.mutated")
	require.True(t, mutated, "the wrapper must advertise the underlying serializer's ownership contract")
}

func BenchmarkSDKMetricsExporterOrdinaryPassthrough(b *testing.B) {
	md := sdkMetricsContractFixture()
	md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).SetName("ordinary.duration")
	exp := &sdkMetricsExporter{Metrics: &sdkMetricsSink{consume: func(context.Context, pmetric.Metrics) error { return nil }}}
	b.ReportAllocs()
	for b.Loop() {
		if err := exp.ConsumeMetrics(context.Background(), md); err != nil {
			b.Fatal(err)
		}
	}
}

func TestSDKMetricsExporterRequiresStatsSender(t *testing.T) {
	f := NewFactory(nil, nil, nil, nil, nil, nil, otel.NewDisabledGatewayUsage(), serializerexporter.TelemetryStore{}, nil)
	_, err := f.CreateMetrics(context.Background(), exportertest.NewNopSettings(Type), f.CreateDefaultConfig())
	require.ErrorContains(t, err, "requires an OTLP stats sender")
}
