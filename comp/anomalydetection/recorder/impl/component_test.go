// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

package recorderimpl

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type lifecycleTestMetricWriter struct {
	closeErr error
	closes   int
}

func (*lifecycleTestMetricWriter) WriteMetric(recorder.MetricData) bool { return true }
func (w *lifecycleTestMetricWriter) Close() error {
	w.closes++
	return w.closeErr
}

type lifecycleTestLogWriter struct {
	closeErr error
	closes   int
}

func (*lifecycleTestLogWriter) WriteLog(recorder.LogData) bool { return true }
func (w *lifecycleTestLogWriter) Close() error {
	w.closes++
	return w.closeErr
}

type lifecycleTestFactory struct {
	metric      *lifecycleTestMetricWriter
	log         *lifecycleTestLogWriter
	metricErr   error
	logErr      error
	metricCfg   recorder.WriterConfig
	logCfg      recorder.WriterConfig
	metricCalls int
	logCalls    int
}

func (f *lifecycleTestFactory) NewMetricWriter(cfg recorder.WriterConfig) (recorder.MetricWriter, error) {
	f.metricCalls++
	f.metricCfg = cfg
	return f.metric, f.metricErr
}
func (f *lifecycleTestFactory) NewLogWriter(cfg recorder.WriterConfig) (recorder.LogWriter, error) {
	f.logCalls++
	f.logCfg = cfg
	return f.log, f.logErr
}

type lifecycleTestHooks struct{ hooks []compdef.Hook }

func (l *lifecycleTestHooks) Append(h compdef.Hook) { l.hooks = append(l.hooks, h) }

func testRecordingConfig(t *testing.T, overrides map[string]interface{}) config.Component {
	t.Helper()
	values := map[string]interface{}{
		"anomaly_detection.recording.enabled":        true,
		"anomaly_detection.recording.output_dir":     t.TempDir(),
		"anomaly_detection.recording.flush_interval": 10,
		"anomaly_detection.recording.retention":      "48h",
	}
	for key, value := range overrides {
		values[key] = value
	}
	return config.NewMockWithOverrides(t, values)
}

func TestDisabledAndAbsentProviderNeedNoLifecycle(t *testing.T) {
	factory := &lifecycleTestFactory{}
	for _, tc := range []struct {
		name    string
		config  map[string]interface{}
		factory option.Option[recorder.WriterFactory]
	}{
		{"disabled", map[string]interface{}{"anomaly_detection.recording.enabled": false}, option.New[recorder.WriterFactory](factory)},
		{"no provider", map[string]interface{}{"anomaly_detection.recording.output_dir": ""}, option.None[recorder.WriterFactory]()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provided, err := NewConfiguredComponent(Requires{Config: testRecordingConfig(t, tc.config), Factory: tc.factory})
			require.NoError(t, err)
			_, present := provided.Comp.Get()
			require.False(t, present)
		})
	}
	require.Zero(t, factory.metricCalls)
	require.Zero(t, factory.logCalls)
}

func TestWriterConfigurationAndDefaults(t *testing.T) {
	for _, tc := range []struct {
		name      string
		overrides map[string]interface{}
		flush     time.Duration
		retention time.Duration
	}{
		{"explicit", nil, 10 * time.Second, 48 * time.Hour},
		{"fallbacks", map[string]interface{}{
			"anomaly_detection.recording.flush_interval": 0,
			"anomaly_detection.recording.retention":      "-1h",
		}, 60 * time.Second, 24 * time.Hour},
		{"zero retention", map[string]interface{}{
			"anomaly_detection.recording.retention": "0s",
		}, 10 * time.Second, 24 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory := &lifecycleTestFactory{metric: &lifecycleTestMetricWriter{}, log: &lifecycleTestLogWriter{}}
			lifecycle := &lifecycleTestHooks{}
			cfg := testRecordingConfig(t, tc.overrides)
			provided, err := NewConfiguredComponent(Requires{Config: cfg, Lifecycle: lifecycle, Factory: option.New[recorder.WriterFactory](factory)})
			require.NoError(t, err)
			_, present := provided.Comp.Get()
			require.True(t, present)
			require.Equal(t, 1, factory.metricCalls)
			require.Equal(t, 1, factory.logCalls)
			require.Equal(t, factory.metricCfg, factory.logCfg)
			require.Equal(t, cfg.GetString("anomaly_detection.recording.output_dir"), factory.metricCfg.OutputDir)
			require.Equal(t, tc.flush, factory.metricCfg.FlushInterval)
			require.Equal(t, tc.retention, factory.metricCfg.Retention)
			require.Len(t, lifecycle.hooks, 1)
			require.NoError(t, lifecycle.hooks[0].OnStop(context.Background()))
			require.Equal(t, 1, factory.metric.closes)
			require.Equal(t, 1, factory.log.closes)
		})
	}
}

func TestInvalidSettingsFailBeforeWriterCreation(t *testing.T) {
	tooLarge := int((1<<63-1)/int64(time.Second) + 1)
	for _, tc := range []struct {
		name      string
		overrides map[string]interface{}
	}{
		{"empty output", map[string]interface{}{"anomaly_detection.recording.output_dir": ""}},
		{"whitespace output", map[string]interface{}{"anomaly_detection.recording.output_dir": "  "}},
		{"negative flush", map[string]interface{}{"anomaly_detection.recording.flush_interval": -1}},
		{"overflow flush", map[string]interface{}{"anomaly_detection.recording.flush_interval": tooLarge}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory := &lifecycleTestFactory{}
			lifecycle := &lifecycleTestHooks{}
			_, err := NewConfiguredComponent(Requires{Config: testRecordingConfig(t, tc.overrides), Lifecycle: lifecycle, Factory: option.New[recorder.WriterFactory](factory)})
			require.Error(t, err)
			require.Zero(t, factory.metricCalls)
			require.Zero(t, factory.logCalls)
			require.Empty(t, lifecycle.hooks)
		})
	}
}

func TestMissingLifecycleFailsBeforeWriterCreation(t *testing.T) {
	factory := &lifecycleTestFactory{}
	_, err := NewConfiguredComponent(Requires{Config: testRecordingConfig(t, nil), Factory: option.New[recorder.WriterFactory](factory)})
	require.ErrorContains(t, err, "lifecycle")
	require.Zero(t, factory.metricCalls)
}

func TestPartialInitializationClosesMetricWriterAndPreservesErrors(t *testing.T) {
	logErr := errors.New("log creation")
	closeErr := errors.New("metric final write")
	metric := &lifecycleTestMetricWriter{closeErr: closeErr}
	factory := &lifecycleTestFactory{metric: metric, logErr: logErr}
	lifecycle := &lifecycleTestHooks{}
	_, err := NewConfiguredComponent(Requires{Config: testRecordingConfig(t, nil), Lifecycle: lifecycle, Factory: option.New[recorder.WriterFactory](factory)})
	require.ErrorIs(t, err, logErr)
	require.ErrorIs(t, err, closeErr)
	require.Equal(t, 1, metric.closes)
	require.Empty(t, lifecycle.hooks)
}

func TestMetricWriterCreationFailureDoesNotCreateLogWriter(t *testing.T) {
	metricErr := errors.New("metric creation")
	factory := &lifecycleTestFactory{metricErr: metricErr}
	lifecycle := &lifecycleTestHooks{}
	_, err := NewConfiguredComponent(Requires{Config: testRecordingConfig(t, nil), Lifecycle: lifecycle, Factory: option.New[recorder.WriterFactory](factory)})
	require.ErrorIs(t, err, metricErr)
	require.Equal(t, 1, factory.metricCalls)
	require.Zero(t, factory.logCalls)
	require.Empty(t, lifecycle.hooks)
}

func TestShutdownClosesBothWritersAndPreservesErrors(t *testing.T) {
	metricErr := errors.New("metric final write")
	logErr := errors.New("log final write")
	factory := &lifecycleTestFactory{
		metric: &lifecycleTestMetricWriter{closeErr: metricErr},
		log:    &lifecycleTestLogWriter{closeErr: logErr},
	}
	lifecycle := &lifecycleTestHooks{}
	_, err := NewConfiguredComponent(Requires{Config: testRecordingConfig(t, nil), Lifecycle: lifecycle, Factory: option.New[recorder.WriterFactory](factory)})
	require.NoError(t, err)
	require.Len(t, lifecycle.hooks, 1)
	err = lifecycle.hooks[0].OnStop(context.Background())
	require.ErrorIs(t, err, metricErr)
	require.ErrorIs(t, err, logErr)
	require.Equal(t, 1, factory.metric.closes)
	require.Equal(t, 1, factory.log.closes)
}
