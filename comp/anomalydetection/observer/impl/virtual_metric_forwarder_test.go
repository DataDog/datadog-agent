// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit/fitcore"
	anomalydetectionconfig "github.com/DataDog/datadog-agent/comp/anomalydetection/config"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// newFITEndpoint returns a FIT setup endpoint inside a private directory. FIT
// requires the socket's parent to deny group and other access, and the path must
// stay short enough for a unix socket (about 100 bytes on macOS).
func newFITEndpoint(t *testing.T) (string, fitcore.SetupEndpoint) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "aadfit")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	require.NoError(t, os.Chmod(dir, 0o700))

	value := "unix:" + filepath.Join(dir, "aad.sock")
	endpoint, err := fitcore.ParseEndpoint(value)
	require.NoError(t, err)
	return value, endpoint
}

// fitConsumer is a DDCHECKS consumer running in the background, standing in for
// the isolated analysis process.
type fitConsumer struct {
	metrics chan checksfit.Metric
	cancel  context.CancelFunc
}

// startConsumer opens the endpoint and streams decoded metrics until closed.
func startConsumer(t *testing.T, endpoint fitcore.SetupEndpoint) *fitConsumer {
	t.Helper()
	consumer := &fitConsumer{metrics: make(chan checksfit.Metric, 4096)}
	ctx, cancel := context.WithCancel(context.Background())
	consumer.cancel = cancel
	go func() {
		config := fitcore.NewConsumerConfig(endpoint)
		// Keep a cancelled test from waiting out the full setup deadline.
		config.SetupTimeout = 30 * time.Second
		inner, err := checksfit.OpenContext(ctx, config)
		if err != nil {
			return
		}
		defer inner.Close()
		for {
			metric, err := inner.ReceiveContext(ctx)
			if err != nil {
				return
			}
			select {
			case consumer.metrics <- metric:
			default:
			}
		}
	}()
	t.Cleanup(cancel)
	return consumer
}

func (c *fitConsumer) close() { c.cancel() }

// collect waits until want is satisfied or the deadline passes, returning what
// was received.
func (c *fitConsumer) collect(t *testing.T, timeout time.Duration, want func([]checksfit.Metric) bool) []checksfit.Metric {
	t.Helper()
	deadline := time.After(timeout)
	var received []checksfit.Metric
	for {
		if want != nil && want(received) {
			return received
		}
		select {
		case metric := <-c.metrics:
			received = append(received, metric)
		case <-deadline:
			return received
		}
	}
}

// metricNames lists the names of the received metrics for diagnostics.
func metricNames(metrics []checksfit.Metric) []string {
	names := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		names = append(names, metric.Name)
	}
	return names
}

func TestVirtualMetricForwarderAggregatesCountersPerSecond(t *testing.T) {
	if !checksfit.Supported {
		t.Skip("FIT is unavailable on this platform")
	}
	endpointValue, endpoint := newFITEndpoint(t)
	consumer := startConsumer(t, endpoint)

	forwarder, err := newVirtualMetricForwarder(anomalydetectionconfig.LogPatternForwardingConfig{
		Enabled:       true,
		Endpoint:      endpointValue,
		RetryInterval: 20 * time.Millisecond,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, forwarder.close()) })

	// Three hits of the same pattern in the same second must arrive as one
	// counter holding their sum, which is what the analysis process stores per
	// whole second.
	for _, value := range []float64{1, 1, 2} {
		forwarder.ObserveVirtualMetric(counterMetric("log.log_pattern_extractor.deadbeef.count", value, 1_700_000_000))
	}
	// A second series, and a gauge, travel as their own records.
	forwarder.ObserveVirtualMetric(counterMetric("log.pattern.cafebabe.count", 1, 1_700_000_000))
	forwarder.ObserveVirtualMetric(observerdef.VirtualMetric{
		Extractor: "log_metrics_extractor",
		Name:      "log.field.duration_ms",
		Value:     2.5,
		Host:      "web-1",
		Tags:      tagset.CompositeTagsFromSlice([]string{"observer_source:logs"}),
		Timestamp: 1_700_000_000,
	})

	received := consumer.collect(t, 30*time.Second, func(received []checksfit.Metric) bool {
		return len(received) >= 3
	})
	require.Len(t, received, 3, "received %v", metricNames(received))

	byName := map[string]checksfit.Metric{}
	for _, metric := range received {
		byName[metric.Name] = metric
	}

	counters, ok := byName["log.log_pattern_extractor.deadbeef.count"]
	require.True(t, ok, "missing the aggregated pattern counter, received %v", metricNames(received))
	assert.Equal(t, checksfit.MetricTypeCounter, counters.MetricType)
	assert.Equal(t, float64(4), counters.Value, "same-second hits must be summed into one record")
	assert.Equal(t, uint64(1_700_000_000), counters.Timestamp)
	assert.Equal(t, "web-1", counters.Hostname)
	assert.Contains(t, counters.Tags, "observer_source:logs")
	assert.Contains(t, counters.Tags, "host:web-1")

	other, ok := byName["log.pattern.cafebabe.count"]
	require.True(t, ok, "missing the signature counter, received %v", metricNames(received))
	assert.Equal(t, float64(1), other.Value)

	gauge, ok := byName["log.field.duration_ms"]
	require.True(t, ok, "missing the gauge, received %v", metricNames(received))
	assert.Equal(t, checksfit.MetricTypeGauge, gauge.MetricType)
	assert.Equal(t, 2.5, gauge.Value)

	submitted, dropped := forwarder.stats()
	assert.Equal(t, uint64(3), submitted)
	assert.Equal(t, uint64(0), dropped)
}

func TestVirtualMetricForwarderRetriesUntilTheConsumerIsReady(t *testing.T) {
	if !checksfit.Supported {
		t.Skip("FIT is unavailable on this platform")
	}
	endpointValue, endpoint := newFITEndpoint(t)

	forwarder, err := newVirtualMetricForwarder(anomalydetectionconfig.LogPatternForwardingConfig{
		Enabled:       true,
		Endpoint:      endpointValue,
		RetryInterval: 20 * time.Millisecond,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, forwarder.close()) })

	// Observations made before the analysis process starts must be kept until a
	// session exists.
	forwarder.ObserveVirtualMetric(counterMetric("log.pattern.cafebabe.count", 1, 1_700_000_000))

	time.Sleep(100 * time.Millisecond)
	consumer := startConsumer(t, endpoint)

	received := consumer.collect(t, 30*time.Second, func(received []checksfit.Metric) bool {
		return len(received) >= 1
	})
	require.Len(t, received, 1, "received %v", metricNames(received))
	assert.Equal(t, "log.pattern.cafebabe.count", received[0].Name)
}

func TestVirtualMetricForwarderDropsWhenPendingIsFull(t *testing.T) {
	if !checksfit.Supported {
		t.Skip("FIT is unavailable on this platform")
	}
	endpointValue, _ := newFITEndpoint(t)

	// No consumer is listening, so nothing drains the pending queue.
	forwarder, err := newVirtualMetricForwarder(anomalydetectionconfig.LogPatternForwardingConfig{
		Enabled:       true,
		Endpoint:      endpointValue,
		RetryInterval: time.Hour,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, forwarder.close()) })

	for index := 0; index < pendingCapacity+10; index++ {
		forwarder.ObserveVirtualMetric(counterMetric("log.pattern.cafebabe.count", 1, 1_700_000_000))
	}

	submitted, dropped := forwarder.stats()
	assert.Equal(t, uint64(0), submitted)
	assert.Equal(t, uint64(10), dropped)
}

func TestNewVirtualMetricForwarderRejectsAnInvalidEndpoint(t *testing.T) {
	if !checksfit.Supported {
		t.Skip("FIT is unavailable on this platform")
	}
	_, err := newVirtualMetricForwarder(anomalydetectionconfig.LogPatternForwardingConfig{
		Enabled:  true,
		Endpoint: "tcp:not-an-address",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FIT endpoint")
}

func TestLogPatternForwardingConfigDefaultsAndOverrides(t *testing.T) {
	disabled := configmock.NewFromYAML(t, "")
	fc := anomalydetectionconfig.LogPatternForwarding(disabled)
	assert.False(t, fc.Enabled)
	assert.Equal(t, "unix:/tmp/aad-isolated/aad.sock", fc.Endpoint)
	assert.Equal(t, 5*time.Second, fc.RetryInterval)
	assert.False(t, fc.DebugLogs)
	assert.False(t, anomalydetectionconfig.ObserverRequired(disabled))

	enabled := configmock.NewFromYAML(t, `
anomaly_detection:
  log_pattern_forwarding:
    enabled: true
    endpoint: unix:/tmp/other/aad.sock
    connect_retry_interval: 2s
    debug_logs: true
`)
	fc = anomalydetectionconfig.LogPatternForwarding(enabled)
	assert.True(t, fc.Enabled)
	assert.Equal(t, "unix:/tmp/other/aad.sock", fc.Endpoint)
	assert.Equal(t, 2*time.Second, fc.RetryInterval)
	assert.True(t, fc.DebugLogs)
	// Enabling forwarding alone must start the observer pipeline that produces
	// the log pattern metrics.
	assert.True(t, anomalydetectionconfig.ObserverRequired(enabled))
}

func counterMetric(name string, value float64, timestamp uint64) observerdef.VirtualMetric {
	return observerdef.VirtualMetric{
		Extractor: "log_pattern_extractor",
		Name:      name,
		Value:     value,
		Host:      "web-1",
		Tags:      tagset.CompositeTagsFromSlice([]string{"service:checkout", "observer_source:logs"}),
		Timestamp: timestamp,
	}
}

// recordingVirtualMetricSink captures the virtual metrics the engine forwards.
type recordingVirtualMetricSink struct {
	metrics []observerdef.VirtualMetric
}

func (r *recordingVirtualMetricSink) ObserveVirtualMetric(metric observerdef.VirtualMetric) {
	r.metrics = append(r.metrics, metric)
}

// patternExtractorStub emits one virtual metric per log, like the log pattern
// extractor does.
type patternExtractorStub struct {
	metricName string
}

func (p *patternExtractorStub) Name() string { return "log_pattern_extractor" }

func (p *patternExtractorStub) ProcessLog(log observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	return observerdef.LogMetricsExtractorOutput{
		Metrics: []observerdef.MetricOutput{{
			Name:       p.metricName,
			Value:      1,
			Tags:       tagset.CompositeTagsFromSlice(log.Tags()),
			HasContext: true,
			Context:    observerdef.MetricContext{Pattern: "connection refused to <*>"},
		}},
	}
}

func TestEngineForwardsVirtualMetricsWithResolvedIdentity(t *testing.T) {
	sink := &recordingVirtualMetricSink{}
	extractor := &patternExtractorStub{metricName: "log.log_pattern_extractor.deadbeef.count"}

	e := newEngine(engineConfig{
		storage:    newTimeSeriesStorage(),
		extractors: []observerdef.LogMetricsExtractor{extractor},
		// The bucketizer intercepts ".count" metrics, which is exactly the log
		// pattern case: forwarding must not depend on it.
		logCountBuckets:   LogCountBucketConfig{Enabled: true, BucketSeconds: 5, IdleTTLSeconds: 300, RetentionSeconds: 600},
		virtualMetricSink: sink,
	})

	e.IngestLog("logs", &logObs{
		content:     "connection refused to 10.0.0.1",
		hostname:    "web-1",
		tags:        []string{"service:checkout"},
		timestampMs: 1_700_000_000_000,
	})

	require.Len(t, sink.metrics, 1)
	forwarded := sink.metrics[0]
	assert.Equal(t, "log_pattern_extractor", forwarded.Extractor)
	assert.Equal(t, "log.log_pattern_extractor.deadbeef.count", forwarded.Name)
	assert.Equal(t, float64(1), forwarded.Value)
	assert.Equal(t, "web-1", forwarded.Host)
	// The log timestamp travels as data time, in whole seconds.
	assert.Equal(t, uint64(1_700_000_000), forwarded.Timestamp)
	assert.True(t, forwarded.HasContext)
	assert.Equal(t, "connection refused to <*>", forwarded.Context.Pattern)

	tags := forwarded.Tags.UnsafeToReadOnlySliceString()
	assert.Contains(t, tags, "service:checkout")
	// The observer source tag identifies the data stream on the isolated side.
	assert.Contains(t, tags, "observer_source:logs")
}

func TestEngineWithoutSinkStillIngestsLogs(t *testing.T) {
	extractor := &patternExtractorStub{metricName: "log.log_pattern_extractor.deadbeef.count"}
	storage := newTimeSeriesStorage()

	e := newEngine(engineConfig{
		storage:    storage,
		extractors: []observerdef.LogMetricsExtractor{extractor},
	})

	e.IngestLog("logs", &logObs{content: "x", hostname: "web-1", timestampMs: 1_000_000})

	assert.Greater(t, storage.TotalSeriesCount(), 0)
}

// TestNewComponentForwardsLogPatternMetricsOverFIT exercises the full live
// wiring: config gate, observer component construction, log ingest, pattern
// extraction, and the FIT session an isolated analysis process consumes.
//
// Both default log metrics extractors are covered: log_metrics_extractor emits
// "log.pattern.<signature hash>.count" for every log, and log_pattern_extractor
// emits "log.log_pattern_extractor.<cluster hash>.count" once a cluster reaches
// its emission threshold (5 hits by default).
func TestNewComponentForwardsLogPatternMetricsOverFIT(t *testing.T) {
	if !checksfit.Supported {
		t.Skip("FIT is unavailable on this platform")
	}
	endpointValue, endpoint := newFITEndpoint(t)
	consumer := startConsumer(t, endpoint)

	cfg := configmock.NewFromYAML(t, fmt.Sprintf(`
anomaly_detection:
  log_pattern_forwarding:
    enabled: true
    endpoint: %s
    connect_retry_interval: 200ms
`, endpointValue))

	provides, err := NewComponent(Requires{
		Lifecycle: &testLifecycle{},
		Config:    cfg,
		Telemetry: telemetryimpl.NewMock(t),
	})
	require.NoError(t, err)

	// Eight identical logs: enough for the pattern clusterer to pass its default
	// 5-hit emission threshold.
	handle := provides.Comp.GetHandle("logs")
	for i := 0; i < 8; i++ {
		handle.ObserveLog(&logObs{
			content:     "connection refused to 10.0.0.1",
			hostname:    "web-1",
			tags:        []string{"service:checkout"},
			timestampMs: time.Now().UnixMilli(),
		})
	}

	received := consumer.collect(t, 30*time.Second, func(received []checksfit.Metric) bool {
		patterns, clusters := 0, 0
		for _, metric := range received {
			if strings.HasPrefix(metric.Name, "log.log_pattern_extractor.") {
				clusters++
			} else if strings.HasPrefix(metric.Name, "log.pattern.") {
				patterns++
			}
		}
		return patterns > 0 && clusters > 0
	})
	require.NotEmpty(t, received, "no metric was forwarded over FIT")

	for _, metric := range received {
		assert.Equal(t, checksfit.MetricTypeCounter, metric.MetricType, "metric %s", metric.Name)
		assert.Equal(t, "web-1", metric.Hostname, "metric %s", metric.Name)
		assert.Contains(t, metric.Tags, "observer_source:logs", "metric %s", metric.Name)
		assert.Contains(t, metric.Tags, "service:checkout", "metric %s", metric.Name)
		assert.NotZero(t, metric.Timestamp, "metric %s", metric.Name)
	}

	names := strings.Join(metricNames(received), " ")
	assert.Contains(t, names, "log.pattern.")
	assert.Contains(t, names, "log.log_pattern_extractor.")
}
