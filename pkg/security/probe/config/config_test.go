// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestNetworkTCXEnabled(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		configmock.NewSystemProbe(t)

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.False(t, cfg.NetworkTCXEnabled)
		assert.False(t, cfg.NetworkTCXConfigured)
	})

	t.Run("config", func(t *testing.T) {
		configmock.NewSystemProbe(t).SetInTest("event_monitoring_config.network.tcx.enabled", true)

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.True(t, cfg.NetworkTCXEnabled)
		assert.True(t, cfg.NetworkTCXConfigured)
	})

	t.Run("disabled explicitly", func(t *testing.T) {
		configmock.NewSystemProbe(t).SetInTest("event_monitoring_config.network.tcx.enabled", false)

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.False(t, cfg.NetworkTCXEnabled)
		assert.True(t, cfg.NetworkTCXConfigured)
	})

	for _, envVar := range []string{
		"DD_EVENT_MONITORING_CONFIG_NETWORK_TCX_ENABLED",
		"DD_RUNTIME_SECURITY_CONFIG_NETWORK_TCX_ENABLED",
	} {
		t.Run(envVar, func(t *testing.T) {
			t.Setenv(envVar, "true")
			configmock.NewSystemProbe(t)

			cfg, err := NewConfig()
			require.NoError(t, err)
			assert.True(t, cfg.NetworkTCXEnabled)
			assert.True(t, cfg.NetworkTCXConfigured)
		})
	}
}

func TestNetworkTCFilterSettings(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		configmock.NewSystemProbe(t)

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.Empty(t, cfg.NetworkTCFilterSettings)
	})

	t.Run("priority", func(t *testing.T) {
		configmock.NewSystemProbe(t).SetInTest("event_monitoring_config.network.classifier_priority", 1)

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.Equal(t, []string{"event_monitoring_config.network.classifier_priority"}, cfg.NetworkTCFilterSettings)
	})

	t.Run("handles", func(t *testing.T) {
		t.Setenv("DD_RUNTIME_SECURITY_CONFIG_NETWORK_CLASSIFIER_HANDLE", "2")
		configmock.NewSystemProbe(t).SetInTest("event_monitoring_config.network.raw_classifier_handle", 3)

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.Equal(t, []string{
			"event_monitoring_config.network.classifier_handle",
			"event_monitoring_config.network.raw_classifier_handle",
		}, cfg.NetworkTCFilterSettings)
	})
}
