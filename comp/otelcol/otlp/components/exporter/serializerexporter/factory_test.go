// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test

package serializerexporter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	exp "go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exportertest"

	otlpmetrics "github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/metrics"
)

type MockTagEnricher struct{}

func (m *MockTagEnricher) SetCardinality(_ string) error {
	return nil
}

func (m *MockTagEnricher) Enrich(_ context.Context, extraTags []string, dimensions *otlpmetrics.Dimensions) []string {
	enrichedTags := make([]string, 0, len(extraTags)+len(dimensions.Tags()))
	enrichedTags = append(enrichedTags, extraTags...)
	enrichedTags = append(enrichedTags, dimensions.Tags()...)

	return enrichedTags
}

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
