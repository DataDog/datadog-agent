// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbacksender

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/infratags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedAppend struct {
	ctx     context.Context
	checkID checkid.ID
	samples []metrics.MetricSample
}

type recordingWriter struct {
	mu      sync.Mutex
	appends []recordedAppend
	err     error
}

func (w *recordingWriter) Append(ctx context.Context, checkID checkid.ID, samples []metrics.MetricSample) error {
	if w.err != nil {
		return w.err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	samplesCopy := make([]metrics.MetricSample, len(samples))
	for i := range samples {
		samplesCopy[i] = *samples[i].Copy()
	}
	w.appends = append(w.appends, recordedAppend{ctx: ctx, checkID: checkID, samples: samplesCopy})
	return nil
}

func (w *recordingWriter) snapshots() []recordedAppend {
	w.mu.Lock()
	defer w.mu.Unlock()

	appends := make([]recordedAppend, len(w.appends))
	copy(appends, w.appends)
	return appends
}

func TestSenderCommitWritesScalarMetricBatch(t *testing.T) {
	writer := &recordingWriter{}
	ctx := context.WithValue(context.Background(), testContextKey{}, "test")
	manager := NewSenderManager(ctx, "default-host", writer, func() float64 { return 42 })

	gotSender, err := manager.GetSender(checkid.ID("cpu:shadow"))
	require.NoError(t, err)
	lookbackSender := gotSender.(*sender)
	lookbackSender.SetCheckCustomTags([]string{"check:tag"})
	lookbackSender.SetCheckService("service1")
	lookbackSender.SetCheckService("service2")
	lookbackSender.FinalizeCheckServiceTag()
	lookbackSender.SetNoIndex(true)

	lookbackSender.Gauge("metric.gauge", 1, "", []string{"metric:tag"})
	lookbackSender.MonotonicCountWithFlushFirstValue("metric.monotonic", 2, "explicit-host", nil, true)
	err = lookbackSender.CountWithTimestamp("metric.count", 3, "", nil, 123)
	require.NoError(t, err)
	lookbackSender.Commit()

	appends := writer.snapshots()
	require.Len(t, appends, 1)
	assert.Equal(t, "test", appends[0].ctx.Value(testContextKey{}))
	assert.Equal(t, checkid.ID("cpu:shadow"), appends[0].checkID)
	require.Len(t, appends[0].samples, 3)

	assert.Equal(t, metrics.MetricSample{
		Name:       "metric.gauge",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"metric:tag", "check:tag", "service:service2"},
		Host:       "default-host",
		SampleRate: 1,
		Timestamp:  42,
		NoIndex:    true,
		Source:     metrics.CheckNameToMetricSource("cpu"),
	}, appends[0].samples[0])

	assert.Equal(t, metrics.MetricSample{
		Name:            "metric.monotonic",
		Value:           2,
		Mtype:           metrics.MonotonicCountType,
		Tags:            []string{"check:tag", "service:service2"},
		Host:            "explicit-host",
		SampleRate:      1,
		Timestamp:       42,
		FlushFirstValue: true,
		NoIndex:         true,
		Source:          metrics.CheckNameToMetricSource("cpu"),
	}, appends[0].samples[1])

	assert.Equal(t, metrics.MetricSample{
		Name:       "metric.count",
		Value:      3,
		Mtype:      metrics.CountWithTimestampType,
		Tags:       []string{"check:tag", "service:service2"},
		Host:       "default-host",
		SampleRate: 1,
		Timestamp:  123,
		NoIndex:    true,
		Source:     metrics.CheckNameToMetricSource("cpu"),
	}, appends[0].samples[2])

	assert.Equal(t, int64(3), lookbackSender.GetSenderStats().MetricSamples)
	lookbackSender.Commit()
	assert.Len(t, writer.snapshots(), 1)
	assert.Equal(t, int64(0), lookbackSender.GetSenderStats().MetricSamples)
}

func TestSenderCopiesTagsBeforeBuffering(t *testing.T) {
	writer := &recordingWriter{}
	manager := NewSenderManager(context.Background(), "default-host", writer, func() float64 { return 42 })

	gotSender, err := manager.GetSender(checkid.ID("cpu:shadow"))
	require.NoError(t, err)
	lookbackSender := gotSender.(*sender)

	tags := []string{"device:first"}
	lookbackSender.GaugeNoIndex("metric.gauge", 1, "", tags)
	tags[0] = "device:second"

	err = lookbackSender.GaugeWithTimestamp("metric.timestamped", 2, "", tags, 123)
	require.NoError(t, err)
	tags[0] = "device:third"

	lookbackSender.Commit()

	appends := writer.snapshots()
	require.Len(t, appends, 1)
	require.Len(t, appends[0].samples, 2)
	assert.Equal(t, []string{"device:first"}, appends[0].samples[0].Tags)
	assert.Equal(t, []string{"device:second"}, appends[0].samples[1].Tags)
	assert.True(t, appends[0].samples[0].NoIndex)
}

func TestSenderAppendsInfraTags(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("infrastructure_mode", "cloud_cost_only", pkgconfigmodel.SourceFile)
	tagger := infratags.NewTagger(cfg)
	require.NotNil(t, tagger)

	writer := &recordingWriter{}
	manager := NewSenderManager(context.Background(), "default-host", writer, func() float64 { return 42 })

	gotSender, err := manager.GetSender(checkid.ID("cpu:shadow"))
	require.NoError(t, err)
	lookbackSender := gotSender.(*sender)
	lookbackSender.SetCheckCustomTags([]string{"check:tag"})
	lookbackSender.SetInfraTagger(tagger)

	lookbackSender.Gauge("metric.gauge", 1, "", []string{"metric:tag"})
	lookbackSender.Commit()

	appends := writer.snapshots()
	require.Len(t, appends, 1)
	require.Len(t, appends[0].samples, 1)
	assert.Equal(t, []string{"metric:tag", "check:tag", "infra_mode:cloud_cost_only"}, appends[0].samples[0].Tags)
}

func TestSenderDropsUnsupportedPayloadsAndRejectsInvalidTimestamps(t *testing.T) {
	writer := &recordingWriter{}
	manager := NewSenderManager(context.Background(), "default-host", writer, func() float64 { return 42 })

	gotSender, err := manager.GetSender(checkid.ID("cpu:shadow"))
	require.NoError(t, err)
	lookbackSender := gotSender.(*sender)

	lookbackSender.Distribution("metric.distribution", 1, "", nil)
	lookbackSender.ServiceCheck("check.service", servicecheck.ServiceCheckOK, "", nil, "")
	lookbackSender.OpenmetricsBucket("metric.bucket", 1, 0, 1, true, "", nil, false)
	lookbackSender.HistogramBucket("metric.histogram.bucket", 1, 0, 1, true, "", nil, false)
	lookbackSender.Event(event.Event{Title: "event"})
	lookbackSender.EventPlatformEvent([]byte(`{}`), "event-type")
	assert.Error(t, lookbackSender.GaugeWithTimestamp("metric.gauge", 1, "", nil, 0))
	assert.Error(t, lookbackSender.CountWithTimestamp("metric.count", 1, "", nil, -1))
	lookbackSender.Commit()

	assert.Empty(t, writer.snapshots())
	assert.Equal(t, int64(0), lookbackSender.GetSenderStats().MetricSamples)
}

func TestSenderTelemetry(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("telemetry.metric_lookback", "*")

	writer := &recordingWriter{}
	manager := NewSenderManager(context.Background(), "default-host", writer, func() float64 { return 42 })

	gotSender, err := manager.GetSender(checkid.ID("cpu:first:shadow"))
	require.NoError(t, err)
	lookbackSender := gotSender.(*sender)

	distributionDrops := tlmUnsupportedDrops.WithValues("Distribution")
	dropsBefore := distributionDrops.Get()
	lookbackSender.Distribution("metric.distribution", 1, "", nil)
	lookbackSender.Distribution("metric.distribution", 2, "", nil)
	assert.Equal(t, dropsBefore, distributionDrops.Get())

	lookbackSender.Commit()
	assert.Equal(t, dropsBefore+2, distributionDrops.Get())
	lookbackSender.Commit()
	assert.Equal(t, dropsBefore+2, distributionDrops.Get())

	lookbackSender.Gauge("metric.gauge", 1, "", nil)
	lookbackSender.Distribution("metric.distribution", 1, "", nil)
	lookbackSender.Commit()

	telemetry := getTelemetry(t)
	assert.Contains(t, telemetry, `metric_lookback__writer_append_samples{check_name="cpu",state="ok"}`)
	assert.Contains(t, telemetry, `metric_lookback__writer_append_duration_seconds{check_name="cpu",state="ok"}`)
	assert.Equal(t, dropsBefore+3, distributionDrops.Get())

	writer.err = errors.New("append failed")
	lookbackSender.Gauge("metric.gauge", 1, "", nil)
	lookbackSender.Commit()

	telemetry = getTelemetry(t)
	assert.Contains(t, telemetry, `metric_lookback__writer_append_samples{check_name="cpu",state="error"}`)
	assert.Contains(t, telemetry, `metric_lookback__writer_append_duration_seconds{check_name="cpu",state="error"}`)
}

func TestSenderManagerReusesAndDestroysSendersByCheckID(t *testing.T) {
	manager := NewSenderManager(context.Background(), "default-host", &recordingWriter{}, func() float64 { return 42 })
	checkID := checkid.ID("cpu:shadow")

	first, err := manager.GetSender(checkID)
	require.NoError(t, err)
	second, err := manager.GetSender(checkID)
	require.NoError(t, err)
	assert.Same(t, first, second)

	manager.DestroySender(checkID)
	third, err := manager.GetSender(checkID)
	require.NoError(t, err)
	assert.NotSame(t, first, third)
}

func TestSenderManagerRejectsSenderForDifferentCheckID(t *testing.T) {
	manager := NewSenderManager(context.Background(), "default-host", &recordingWriter{}, func() float64 { return 42 })

	gotSender, err := manager.GetSender(checkid.ID("cpu:shadow"))
	require.NoError(t, err)

	err = manager.SetSender(gotSender, checkid.ID("disk:shadow"))
	require.Error(t, err)
	assert.ErrorContains(t, err, "sender ID cpu:shadow does not match check ID disk:shadow")
}

func TestSenderManagerDefaultSenderAndNoopWriter(t *testing.T) {
	manager := NewSenderManager(context.Background(), "default-host", nil, nil)

	defaultSender, err := manager.GetDefaultSender()
	require.NoError(t, err)
	defaultSender.Gauge("metric.default", 1, "", nil)
	defaultSender.Commit()
	assert.Equal(t, int64(0), defaultSender.GetSenderStats().MetricSamples)

	gotSender, err := manager.GetSender(checkid.ID("cpu:shadow"))
	require.NoError(t, err)
	gotSender.Gauge("metric.gauge", 1, "", nil)
	gotSender.Commit()
	assert.Equal(t, int64(1), gotSender.GetSenderStats().MetricSamples)
}

type testContextKey struct{}

func getTelemetry(t *testing.T) string {
	t.Helper()

	req, err := http.NewRequest("GET", "/", nil)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	telemetryimpl.GetCompatComponent().Handler().ServeHTTP(rec, req)
	return strings.ReplaceAll(rec.Body.String(), "\r\n", "\n")
}
