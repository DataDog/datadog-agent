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
	})

	t.Run("config", func(t *testing.T) {
		configmock.NewSystemProbe(t).SetInTest("event_monitoring_config.network.tcx.enabled", true)

		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.True(t, cfg.NetworkTCXEnabled)
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
		})
	}
}
