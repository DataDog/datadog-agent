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
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/exporter/exportertest"

	"github.com/DataDog/datadog-agent/pkg/serializer"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory(&serializer.MockSerializer{})
	cfg := factory.CreateDefaultConfig()
	assert.NoError(t, componenttest.CheckConfigStruct(cfg))
	_, ok := factory.CreateDefaultConfig().(*exporterConfig)
	assert.True(t, ok)
}

func TestNewMetricsExporter(t *testing.T) {
	factory := NewFactory(&serializer.MockSerializer{})
	cfg := factory.CreateDefaultConfig()
	set := exportertest.NewNopCreateSettings()
	exp, err := factory.CreateMetricsExporter(context.Background(), set, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, exp)
}

func TestNewMetricsExporterInvalid(t *testing.T) {
	factory := NewFactory(&serializer.MockSerializer{})
	cfg := factory.CreateDefaultConfig()

	expCfg := cfg.(*exporterConfig)
	expCfg.Metrics.HistConfig.Mode = "InvalidMode"

	set := exportertest.NewNopCreateSettings()
	_, err := factory.CreateMetricsExporter(context.Background(), set, cfg)
	assert.Error(t, err)
}

func TestNewTracesExporter(t *testing.T) {
	factory := NewFactory(&serializer.MockSerializer{})
	cfg := factory.CreateDefaultConfig()

	set := exportertest.NewNopCreateSettings()
	_, err := factory.CreateTracesExporter(context.Background(), set, cfg)
	assert.Error(t, err)
}

func TestNewLogsExporter(t *testing.T) {
	factory := NewFactory(&serializer.MockSerializer{})
	cfg := factory.CreateDefaultConfig()

	set := exportertest.NewNopCreateSettings()
	_, err := factory.CreateLogsExporter(context.Background(), set, cfg)
	assert.Error(t, err)
}
