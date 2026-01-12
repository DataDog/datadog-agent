// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test || functionaltests || stresstests

package telemetryimpl

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type testDependencies struct {
	fx.In

	Lyfecycle fx.Lifecycle
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
		fx.Provide(func(m telemetry.Mock) telemetry.Component { return m }))
}

// NewMock returns a new mock for telemetry
func NewMock(t testing.TB) telemetry.Mock {
	reg := prometheus.NewRegistry()

	telemetry := &telemetryImplMock{
		telemetryImpl{
			mutex:           &mutex,
			registry:        reg,
			defaultRegistry: prometheus.NewRegistry(),
		},
	}

	t.Cleanup(telemetry.Reset)
	return telemetry
}

type telemetryImplMock struct {
	telemetryImpl
}

func newMock(deps testDependencies) telemetry.Mock {
	reg := prometheus.NewRegistry()

	telemetry := &telemetryImplMock{
		telemetryImpl{
			mutex:           &mutex,
			registry:        reg,
			defaultRegistry: prometheus.NewRegistry(),
		},
	}

	deps.Lyfecycle.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			telemetry.Reset()

			return nil
		},
	})

	return telemetry
}

type internalMetric struct {
	metric     *dto.Metric
	metricType dto.MetricType
}

func (m *internalMetric) Tags() map[string]string {
	labels := m.metric.GetLabel()
	labelMap := make(map[string]string, len(labels))
	for _, label := range labels {
		labelMap[label.GetName()] = label.GetValue()
	}
	return labelMap
}

func (m *internalMetric) Value() float64 {
	var value float64
	switch m.metricType {
	case dto.MetricType_COUNTER:
		value = m.metric.GetCounter().GetValue()
	case dto.MetricType_GAUGE:
		value = m.metric.GetGauge().GetValue()
	case dto.MetricType_HISTOGRAM:
		value = m.metric.GetHistogram().GetSampleSum()
	}

	return value
}

func (t *telemetryImplMock) GetRegistry() *prometheus.Registry {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.registry
}

func (t *telemetryImplMock) GetCountMetric(subsystem, name string) ([]telemetry.Metric, error) {
	return t.getMetric(dto.MetricType_COUNTER, subsystem, name)
}

func (t *telemetryImplMock) GetGaugeMetric(subsystem, name string) ([]telemetry.Metric, error) {
	return t.getMetric(dto.MetricType_GAUGE, subsystem, name)
}

func (t *telemetryImplMock) GetHistogramMetric(subsystem, name string) ([]telemetry.Metric, error) {
	return t.getMetric(dto.MetricType_HISTOGRAM, subsystem, name)
}

func (t *telemetryImplMock) getMetric(metricType dto.MetricType, subsystem, name string) ([]telemetry.Metric, error) {
	metricFamily, err := t.GetRegistry().Gather()
	if err != nil {
		return nil, err
	}

	metricName := fmt.Sprintf("%s__%s", subsystem, name)
	idx := slices.IndexFunc(metricFamily, func(mf *dto.MetricFamily) bool {
		return mf.GetName() == metricName
	})

	if idx == -1 {
		return nil, fmt.Errorf("metric: %s not found", metricName)
	}

	dtoMetric := metricFamily[idx]

	metrics := dtoMetric.GetMetric()
	dtoMetricType := dtoMetric.GetType()

	if dtoMetricType != metricType {
		return nil, fmt.Errorf("metric: %s is not %s, but %s", metricName, metricType.String(), dtoMetricType.String())
	}

	internalMetrics := make([]telemetry.Metric, len(metrics))

	for i, metric := range metrics {
		internalMetrics[i] = &internalMetric{
			metric:     metric,
			metricType: dtoMetricType,
		}
	}

	return internalMetrics, nil
}
