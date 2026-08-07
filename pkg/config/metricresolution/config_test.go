// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metricresolution_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/metricresolution"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestDefaults(t *testing.T) {
	cfg := configmock.New(t)

	got, err := metricresolution.Read(cfg)
	require.NoError(t, err)
	require.False(t, got.Enabled)
	require.Equal(t, time.Second, got.CheckInterval)
	require.Equal(t, time.Second, got.DogStatsDAggregationInterval)
	require.Equal(t, time.Second, got.SerializerFlushInterval)
}

func TestEnabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set(metricresolution.EnabledKey, true, pkgconfigmodel.SourceFile)

	got, err := metricresolution.Read(cfg)
	require.NoError(t, err)
	require.True(t, got.Enabled)
}

func TestRejectsInvalidEnabledIntervals(t *testing.T) {
	for _, test := range []struct {
		name  string
		key   string
		value time.Duration
	}{
		{name: "zero", key: metricresolution.CheckIntervalKey, value: 0},
		{name: "negative", key: metricresolution.DogStatsDAggregationIntervalKey, value: -time.Second},
		{name: "sub-second", key: metricresolution.SerializerFlushIntervalKey, value: 500 * time.Millisecond},
		{name: "fractional-second", key: metricresolution.CheckIntervalKey, value: 1500 * time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.Set(metricresolution.EnabledKey, true, pkgconfigmodel.SourceFile)
			cfg.Set(test.key, test.value, pkgconfigmodel.SourceFile)

			_, err := metricresolution.Read(cfg)
			require.ErrorContains(t, err, test.key)
		})
	}
}

func TestIgnoresDormantInvalidIntervals(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set(metricresolution.CheckIntervalKey, 500*time.Millisecond, pkgconfigmodel.SourceFile)

	got, err := metricresolution.Read(cfg)
	require.NoError(t, err)
	require.False(t, got.Enabled)
}
