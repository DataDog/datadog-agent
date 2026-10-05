// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder && python

package fx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestTaggedModuleWithoutProvider(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"anomaly_detection.recording.enabled":    true,
		"anomaly_detection.recording.output_dir": "",
	})
	var component option.Option[recorder.Component]
	var provider option.Option[recorder.WriterFactory]
	app := fx.New(
		fx.NopLogger,
		Module().Option,
		fx.Provide(func() config.Component { return cfg }),
		fx.Populate(&component, &provider),
	)
	require.NoError(t, app.Err())
	_, hasComponent := component.Get()
	_, hasProvider := provider.Get()
	require.False(t, hasComponent)
	require.False(t, hasProvider)
}

type fxMetricWriter struct{ closes int }

func (*fxMetricWriter) WriteMetric(recorder.MetricData) bool { return true }
func (w *fxMetricWriter) Close() error                       { w.closes++; return nil }

type fxLogWriter struct{ closes int }

func (*fxLogWriter) WriteLog(recorder.LogData) bool { return true }
func (w *fxLogWriter) Close() error                 { w.closes++; return nil }

type fxWriterFactory struct {
	metric      *fxMetricWriter
	log         *fxLogWriter
	metricCalls int
	logCalls    int
}

func (f *fxWriterFactory) NewMetricWriter(recorder.WriterConfig) (recorder.MetricWriter, error) {
	f.metricCalls++
	return f.metric, nil
}
func (f *fxWriterFactory) NewLogWriter(recorder.WriterConfig) (recorder.LogWriter, error) {
	f.logCalls++
	return f.log, nil
}

func TestTaggedModuleWithProviderStopsWriters(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"anomaly_detection.recording.enabled":    true,
		"anomaly_detection.recording.output_dir": t.TempDir(),
	})
	factory := &fxWriterFactory{metric: &fxMetricWriter{}, log: &fxLogWriter{}}
	var provided option.Option[recorder.Component]
	app := fx.New(
		fx.NopLogger,
		fxutil.FxLifecycleAdapter(),
		Module().Option,
		fx.Provide(func() config.Component { return cfg }),
		fx.Replace(option.New[recorder.WriterFactory](factory)),
		fx.Populate(&provided),
	)
	require.NoError(t, app.Err())
	_, present := provided.Get()
	require.True(t, present)
	require.Equal(t, 1, factory.metricCalls)
	require.Equal(t, 1, factory.logCalls)
	require.NoError(t, app.Start(context.Background()))
	require.NoError(t, app.Stop(context.Background()))
	require.Equal(t, 1, factory.metric.closes)
	require.Equal(t, 1, factory.log.closes)
}
