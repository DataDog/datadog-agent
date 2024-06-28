// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package telemetryimpl

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	sdk "go.opentelemetry.io/otel/sdk/metric"
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

type telemetryImplMock struct {
	telemetryImpl
}

func newMock(deps testDependencies) telemetry.Mock {
	reg := prometheus.NewRegistry()
	provider := newProvider(reg)

	telemetry := &telemetryImplMock{
		telemetryImpl{
			mutex:         &mutex,
			registry:      reg,
			meterProvider: provider,
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
	metricType string
}

func (m *internalMetric) Labels() map[string]string {
	labels := m.metric.GetLabel()
	// labels are not necessarily in the order they were declared
	// so we use a map to compare them
	labelMap := make(map[string]string, len(labels))
	for _, label := range labels {
		labelMap[label.GetName()] = label.GetValue()
	}
	return labelMap
}

func (m *internalMetric) Value() float64 {
	var value float64
	switch m.metricType {
	case "count":
		value = m.metric.GetCounter().GetValue()
	case "gauge":
		value = m.metric.GetGauge().GetValue()
	case "histogram":
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
	metricFamily, err := t.GetRegistry().Gather()
	if err != nil {
		return nil, err
	}

	return getMetric(metricFamily, "count", subsystem, name)
}

func (t *telemetryImplMock) GetGaugeMetric(subsystem, name string) ([]telemetry.Metric, error) {
	metricFamily, err := t.GetRegistry().Gather()
	if err != nil {
		return nil, err
	}

	return getMetric(metricFamily, "gauge", subsystem, name)
}

func (t *telemetryImplMock) GetHistogramMetric(subsystem, name string) ([]telemetry.Metric, error) {
	metricFamily, err := t.GetRegistry().Gather()
	if err != nil {
		return nil, err
	}
	return getMetric(metricFamily, "histogram", subsystem, name)
}

func getMetric(metricFamily []*dto.MetricFamily, metricType, subsystem, name string) ([]telemetry.Metric, error) {
	metricName := fmt.Sprintf("%s__%s", subsystem, name)
	for _, mf := range metricFamily {
		if mf.GetName() == metricName {
			metrics := mf.GetMetric()
			internalMetrics := make([]telemetry.Metric, len(metrics))

			for i, metric := range metrics {
				internalMetrics[i] = &internalMetric{
					metric:     metric,
					metricType: metricType,
				}
			}

			return internalMetrics, nil
		}
	}

	return nil, fmt.Errorf("%s metric %s not found", metricType, metricName)
}

func (t *telemetryImplMock) GetMeterProvider() *sdk.MeterProvider {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.meterProvider
}
