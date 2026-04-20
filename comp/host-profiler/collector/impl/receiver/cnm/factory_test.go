// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cnm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory()
	require.NotNil(t, factory)
	assert.Equal(t, component.MustNewType("cnm"), factory.Type())
}

func TestCreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	require.NotNil(t, cfg)

	cnmCfg, ok := cfg.(*Config)
	require.True(t, ok)
	assert.True(t, cnmCfg.CollectTCPv4)
	assert.Equal(t, defaultMaxTrackedConnections, cnmCfg.MaxTrackedConnections)
}

func TestCreateMetricsReceiver(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	sink := consumertest.NewNop()

	recv, err := factory.CreateMetrics(
		context.Background(),
		receivertest.NewNopSettings(component.MustNewType("cnm")),
		cfg,
		sink,
	)
	require.NoError(t, err)
	require.NotNil(t, recv)
}

func TestCreateMetricsReceiverInvalidConfig(t *testing.T) {
	factory := NewFactory()
	// Pass a non-Config type
	recv, err := factory.CreateMetrics(
		context.Background(),
		receivertest.NewNopSettings(component.MustNewType("cnm")),
		&struct{}{},
		consumertest.NewNop(),
	)
	require.Error(t, err)
	require.Nil(t, recv)
}
