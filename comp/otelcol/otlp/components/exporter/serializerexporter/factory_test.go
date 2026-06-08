// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test

package serializerexporter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	exp "go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// newFactory creates a factory for test-only
func newFactory() exp.Factory {
	return NewFactoryForOSSExporter(component.MustNewType(TypeStr), nil)
}

func TestNewFactory(t *testing.T) {
	factory := newFactory()
	cfg := factory.CreateDefaultConfig()
	assert.NoError(t, componenttest.CheckConfigStruct(cfg))
	_, ok := factory.CreateDefaultConfig().(*ExporterConfig)
	assert.True(t, ok)
}

func TestNewMetricsExporter(t *testing.T) {
	factory := newFactory()
	cfg := factory.CreateDefaultConfig()
	set := exportertest.NewNopSettings(component.MustNewType(TypeStr))
	exp, err := factory.CreateMetrics(context.Background(), set, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, exp)
}

func TestNewMetricsExporterInvalid(t *testing.T) {
	factory := newFactory()
	cfg := factory.CreateDefaultConfig()

	expCfg := cfg.(*ExporterConfig)
	expCfg.Metrics.Metrics.HistConfig.Mode = "InvalidMode"

	set := exportertest.NewNopSettings(component.MustNewType(TypeStr))
	_, err := factory.CreateMetrics(context.Background(), set, cfg)
	assert.Error(t, err)
}

func TestNewTracesExporter(t *testing.T) {
	factory := newFactory()
	cfg := factory.CreateDefaultConfig()

	set := exportertest.NewNopSettings(component.MustNewType(TypeStr))
	_, err := factory.CreateTraces(context.Background(), set, cfg)
	assert.Error(t, err)
}

func TestNewLogsExporter(t *testing.T) {
	factory := newFactory()
	cfg := factory.CreateDefaultConfig()

	set := exportertest.NewNopSettings(component.MustNewType(TypeStr))
	_, err := factory.CreateLogs(context.Background(), set, cfg)
	assert.Error(t, err)
}

// TestNativeHistogramFeatureGateWiring verifies that when the NativeOTelHistograms feature gate
// is enabled AND wired into the factory, delta explicit-bound histograms flow through SendSketch
// as ExplicitBoundProvider instead of being converted to DDSketches.
//
// TODO(OTAGENT-1079): Un-skip once v3 serialization is implemented and the gate is re-enabled
// in newFactoryForAgentWithType.
func TestNativeHistogramFeatureGateWiring(t *testing.T) {
	t.Skip("OTAGENT-1079: NativeHistogramFeatureGate is temporarily disabled to prevent silent data loss until v3 serialization is implemented")

	require.NoError(t, featuregate.GlobalRegistry().Set(NativeHistogramFeatureGate.ID(), true))
	t.Cleanup(func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(NativeHistogramFeatureGate.ID(), false))
	})

	mock := &capturingMockSerializer{}
	hostGetter := SourceProviderFunc(func(_ context.Context) (string, error) { return "testhost", nil })
	factory := NewFactoryForAgent(mock, hostGetter, TelemetryStore{})

	cfg := factory.CreateDefaultConfig()
	set := exportertest.NewNopSettings(component.MustNewType(TypeStr))
	exporter, err := factory.CreateMetrics(context.Background(), set, cfg)
	require.NoError(t, err)
	require.NotNil(t, exporter)

	err = exporter.Start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, exporter.Shutdown(context.Background()))
	}()

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("host.name", "testhost")
	sm := rm.ScopeMetrics().AppendEmpty()
	m := sm.Metrics().AppendEmpty()
	m.SetName("gate.test.histogram")
	h := m.SetEmptyHistogram()
	h.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := h.DataPoints().AppendEmpty()
	dp.ExplicitBounds().FromRaw([]float64{1, 5, 10})
	dp.BucketCounts().FromRaw([]uint64{1, 3, 5, 2})
	dp.SetCount(11)
	dp.SetSum(42.0)
	now := time.Now()
	dp.SetTimestamp(pcommon.NewTimestampFromTime(now))
	dp.SetStartTimestamp(pcommon.NewTimestampFromTime(now.Add(-10 * time.Second)))

	err = exporter.ConsumeMetrics(context.Background(), md)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		for _, s := range mock.sketches {
			if len(s.Points) > 0 {
				if _, ok := s.Points[0].Sketch.(metrics.ExplicitBoundProvider); ok {
					return true
				}
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond,
		"with NativeHistogramFeatureGate ON, delta explicit-bound histograms should flow through SendSketch as ExplicitBoundProvider")
}
