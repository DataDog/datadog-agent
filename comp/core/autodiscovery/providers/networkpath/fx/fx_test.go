// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package fx

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	providertypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
)

type providerRecorder struct {
	providers []providertypes.ConfigProvider
}

func (r *providerRecorder) AddConfigProvider(provider providertypes.ConfigProvider, _ bool, _ time.Duration) {
	r.providers = append(r.providers, provider)
}

func TestNewListener(t *testing.T) {
	for _, tt := range []struct {
		name        string
		globalRC    bool
		networkPath bool
		enabled     bool
	}{
		{name: "disabled"},
		{name: "global only", globalRC: true},
		{name: "network path only", networkPath: true},
		{name: "enabled", globalRC: true, networkPath: true, enabled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewMockWithOverrides(t, map[string]any{
				"remote_configuration.enabled":       tt.globalRC,
				"network_path.remote_config.enabled": tt.networkPath,
			})
			recorder := &providerRecorder{}

			listener := newListener(cfg, recorder)
			callback, subscribed := listener.ListenerProvider[data.ProductNetworkPath]
			assert.Equal(t, tt.enabled, subscribed)
			assert.Len(t, recorder.providers, boolToInt(tt.enabled))
			if tt.enabled {
				require.NotNil(t, callback)
			}
		})
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
