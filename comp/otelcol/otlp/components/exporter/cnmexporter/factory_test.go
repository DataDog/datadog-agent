// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cnmexporter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/exportertest"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory()
	require.NotNil(t, factory)
	assert.Equal(t, component.MustNewType("datadog_cnm"), factory.Type())
}

func TestCreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	require.NotNil(t, cfg)

	cnmCfg, ok := cfg.(*Config)
	require.True(t, ok)
	assert.Equal(t, defaultMaxConnsPerMessage, cnmCfg.MaxConnsPerMessage)
}

func TestCreateMetricsExporter(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()

	exp, err := factory.CreateMetrics(
		context.Background(),
		exportertest.NewNopSettings(component.MustNewType("datadog_cnm")),
		cfg,
	)
	require.NoError(t, err)
	require.NotNil(t, exp)
}
