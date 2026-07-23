// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package serializerexporter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/otel"

	goatomic "go.uber.org/atomic"
	"go.uber.org/zap"
)

// fakeIntake is an httptest.Server that accepts requests and tracks volume.
// Optional artificialLatency simulates a backend round-trip so concurrency
// benchmarks show parallelism — set to 0 for the fastest local loopback.
type fakeIntake struct {
	*httptest.Server
	requests          goatomic.Int64
	bytes             goatomic.Int64
	status            int
	artificialLatency time.Duration
}

func newFakeIntake(status int) *fakeIntake {
	return newFakeIntakeWithLatency(status, 0)
}

func newFakeIntakeWithLatency(status int, latency time.Duration) *fakeIntake {
	if status == 0 {
		status = http.StatusOK
	}
	fi := &fakeIntake{status: status, artificialLatency: latency}
	fi.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fi.requests.Add(1)
		if r.ContentLength > 0 {
			fi.bytes.Add(r.ContentLength)
		}
		_, _ = http.MaxBytesReader(w, r.Body, 1<<24).Read(make([]byte, 0))
		_ = r.Body.Close()
		if fi.artificialLatency > 0 {
			time.Sleep(fi.artificialLatency)
		}
		w.WriteHeader(fi.status)
	}))
	return fi
}

// newFakeIntakeWithHandler returns a fakeIntake whose response is fully
// controlled by the provided handler function. The requests counter is
// incremented before the handler is called so the handler can read it.
func newFakeIntakeWithHandler(handler func(n int64, w http.ResponseWriter, r *http.Request)) *fakeIntake {
	fi := &fakeIntake{}
	fi.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := fi.requests.Add(1)
		if r.ContentLength > 0 {
			fi.bytes.Add(r.ContentLength)
		}
		_, _ = http.MaxBytesReader(w, r.Body, 1<<24).Read(make([]byte, 0))
		_ = r.Body.Close()
		handler(n, w, r)
	}))
	return fi
}

// retryExporterConfig builds a config suitable for retry tests: no queue
// (synchronous ConsumeMetrics), retry enabled with very short backoff so
// tests complete quickly, and no artificial HTTP client timeout that would
// race with the retry budget.
func retryExporterConfig(t testing.TB, intakeURL string) *ExporterConfig {
	t.Helper()
	cfg := benchExporterConfig(t, intakeURL) // starts with retry disabled, no queue
	cfg.RetryConfig.Enabled = true
	cfg.RetryConfig.InitialInterval = 5 * time.Millisecond
	cfg.RetryConfig.MaxInterval = 20 * time.Millisecond
	cfg.RetryConfig.MaxElapsedTime = 10 * time.Second
	return cfg
}

// makeGaugeMetrics builds a pmetric.Metrics payload with n gauge data points.
func makeGaugeMetrics(n int) pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	now := pcommon.NewTimestampFromTime(time.Now())
	for i := 0; i < n; i++ {
		m := sm.Metrics().AppendEmpty()
		m.SetName("bench.gauge." + strconv.Itoa(i%64))
		dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
		dp.SetTimestamp(now)
		dp.SetDoubleValue(float64(i))
		dp.Attributes().PutStr("tag1", "value"+strconv.Itoa(i%8))
		dp.Attributes().PutStr("tag2", "value"+strconv.Itoa(i%16))
	}
	return md
}

// benchExporterConfig builds an ExporterConfig wired to the fake intake.
func benchExporterConfig(t testing.TB, intakeURL string) *ExporterConfig {
	t.Helper()
	cfg := newDefaultConfig().(*ExporterConfig)
	cfg.API.Key = configopaque.String("0000000000000000000000000000000000000000")
	cfg.API.Site = "datadoghq.com"
	cfg.Metrics.Metrics.TCPAddrConfig.Endpoint = intakeURL
	cfg.HostMetadata.Enabled = false
	// Drain the OTel queue inline so benchmark numbers measure time spent in
	// ConsumeMetrics end-to-end rather than enqueue time.
	cfg.QueueBatchConfig = configoptional.None[exporterhelper.QueueBatchConfig]()
	// Disable retry so failures surface in a single ConsumeMetrics call rather
	// than blocking the test on the legacy 15-min retry budget.
	cfg.RetryConfig.Enabled = false
	cfg.HTTPConfig = confighttp.NewDefaultClientConfig()
	cfg.HTTPConfig.Timeout = 10 * time.Second
	return cfg
}

// setSyncForwarderGate flips the feature gate and returns a cleanup function.
func setSyncForwarderGate(t testing.TB, enabled bool) func() {
	t.Helper()
	prev := useSyncForwarderGate.IsEnabled()
	require.NoError(t, featuregate.GlobalRegistry().Set(useSyncForwarderGate.ID(), enabled))
	return func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(useSyncForwarderGate.ID(), prev))
	}
}

// buildBenchExporter builds an exporter wired to a fake intake. When the
// UseSyncForwarder gate is on, it simulates the DDOT production path by
// injecting an OTelSyncForwarder into the shared serializer before calling the
// factory, mirroring cmd/otel-agent/subcommands/run/command.go (OTAGENT-1024).
// The caller is responsible for flipping the feature gate before invoking.
func buildBenchExporter(t testing.TB, cfg *ExporterConfig) component.Component {
	t.Helper()
	hostGetter := SourceProviderFunc(func(context.Context) (string, error) { return "bench-host", nil })

	var injectedSerializer serializer.MetricSerializer
	if useSyncForwarderGate.IsEnabled() {
		httpClient := &http.Client{Timeout: cfg.HTTPConfig.Timeout}
		ser, _, err := initSyncSerializerForTest(t, zap.NewNop(), cfg, hostGetter, httpClient)
		require.NoError(t, err)
		injectedSerializer = ser
	}

	f := NewFactoryForOTelAgent(injectedSerializer, hostGetter, nil, otel.NewDisabledGatewayUsage(), TelemetryStore{}, nil, nil)

	exp, err := f.CreateMetrics(
		context.Background(),
		exportertest.NewNopSettings(component.MustNewType("datadog")),
		cfg,
	)
	require.NoError(t, err)
	require.NoError(t, exp.Start(context.Background(), componenttest.NewNopHost()))
	return exp
}

type metricsConsumer interface {
	ConsumeMetrics(context.Context, pmetric.Metrics) error
}

func benchConsumeMetrics(b *testing.B, useSync bool, metricsPerBatch int) {
	restore := setSyncForwarderGate(b, useSync)
	defer restore()

	intake := newFakeIntake(http.StatusOK)
	defer intake.Close()

	cfg := benchExporterConfig(b, intake.URL)
	exp := buildBenchExporter(b, cfg)
	defer func() { _ = exp.Shutdown(context.Background()) }()

	mc, ok := exp.(metricsConsumer)
	require.True(b, ok, "exporter does not implement ConsumeMetrics: %T", exp)

	md := makeGaugeMetrics(metricsPerBatch)

	b.ReportAllocs()
	b.ResetTimer()
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		if err := mc.ConsumeMetrics(ctx, md); err != nil {
			b.Fatalf("ConsumeMetrics: %v", err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(intake.requests.Load())/float64(b.N), "intake_reqs/op")
	b.ReportMetric(float64(intake.bytes.Load())/float64(b.N), "intake_bytes/op")
}

// BenchmarkConsumeMetrics_DefaultForwarder benchmarks the legacy async
// DefaultForwarder path (feature gate off).
func BenchmarkConsumeMetrics_DefaultForwarder(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("metrics=%d", n), func(b *testing.B) {
			benchConsumeMetrics(b, false, n)
		})
	}
}

// BenchmarkConsumeMetrics_SyncForwarder benchmarks the OTelSyncForwarder path
// (feature gate on — the post-OTAGENT-1024 default).
func BenchmarkConsumeMetrics_SyncForwarder(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("metrics=%d", n), func(b *testing.B) {
			benchConsumeMetrics(b, true, n)
		})
	}
}

// benchSyncForwarderConsumers measures end-to-end throughput of the sync
// forwarder path when the OTel sending_queue is enabled with the given number
// of consumers. The intake adds artificial latency so concurrency benefits are
// visible: with N consumers and L latency per request, ideal time to drain B
// batches is ceil(B/N)*L. Without latency the loopback HTTP cost is too small
// for parallelism to dominate.
func benchSyncForwarderConsumers(b *testing.B, numConsumers int, intakeLatency time.Duration, metricsPerBatch int) {
	restore := setSyncForwarderGate(b, true)
	defer restore()

	intake := newFakeIntakeWithLatency(http.StatusOK, intakeLatency)
	defer intake.Close()

	cfg := benchExporterConfig(b, intake.URL)
	// Enable the queue with the chosen consumer count; ConsumeMetrics
	// becomes a non-blocking enqueue and parallel consumers drain
	// concurrently into the sync forwarder.
	queue := exporterhelper.NewDefaultQueueConfig()
	queue.NumConsumers = numConsumers
	// Size the queue large enough that the producer is never blocked, so we
	// measure consumer-side throughput rather than producer back-pressure.
	queue.QueueSize = int64(max(b.N+numConsumers, 1024))
	cfg.QueueBatchConfig = configoptional.Some(queue)
	// Retry disabled: failures from the artificial intake aren't expected,
	// and we don't want retry overhead bleeding into the numbers.
	cfg.RetryConfig.Enabled = false

	exp := buildBenchExporter(b, cfg)
	mc, ok := exp.(metricsConsumer)
	require.True(b, ok)

	md := makeGaugeMetrics(metricsPerBatch)
	target := int64(b.N)

	b.ReportAllocs()
	b.ResetTimer()
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		if err := mc.ConsumeMetrics(ctx, md); err != nil {
			b.Fatalf("ConsumeMetrics: %v", err)
		}
	}
	// Drain: wait for at least one intake request per enqueued batch. With
	// payload splitting the intake may see > target requests; that's fine.
	deadline := time.Now().Add(60*time.Second + intakeLatency*time.Duration(b.N)/time.Duration(numConsumers))
	for intake.requests.Load() < target && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	b.StopTimer()
	require.GreaterOrEqual(b, intake.requests.Load(), target, "queue did not drain within deadline")

	// Shutdown after we stopped the timer, so its cost is excluded.
	_ = exp.Shutdown(context.Background())

	b.ReportMetric(float64(intake.requests.Load())/float64(b.N), "intake_reqs/op")
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "batches/sec")
}

// BenchmarkConsumeMetrics_SyncForwarder_Consumers compares throughput under
// the sync forwarder at different OTel queue consumer counts. Includes one
// realistic-latency sub-benchmark per consumer count so the parallelism
// benefit is visible — at zero added latency the loopback HTTP cost is too
// small for consumer count to matter.
func BenchmarkConsumeMetrics_SyncForwarder_Consumers(b *testing.B) {
	const metricsPerBatch = 100
	for _, latency := range []time.Duration{0, 25 * time.Millisecond} {
		for _, n := range []int{1, 4, 10, 25} {
			b.Run(fmt.Sprintf("consumers=%d/latency=%s", n, latency), func(b *testing.B) {
				benchSyncForwarderConsumers(b, n, latency, metricsPerBatch)
			})
		}
	}
}
