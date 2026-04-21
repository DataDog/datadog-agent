// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// discoveryKey is the config key guarding discovery service map mode.
var discoveryKey = discoveryNS("service_map", "enabled")

// protocolsDisabledByDiscovery lists the USM protocol flags that discovery
// mode must force off (kept minimal per the RFC — only HTTP + TLS probes).
var protocolsDisabledByDiscovery = []string{
	smNS("http2", "enabled"),
	smNS("kafka", "enabled"),
	smNS("postgres", "enabled"),
	smNS("redis", "enabled"),
}

// setBool sets a bool config key from an explicit source so the mock treats
// it as user-configured (vs. a default).
func setBool(cfg model.Config, key string, value bool) {
	cfg.Set(key, value, model.SourceUnknown)
}

func TestAdjustDiscovery_Coexistence(t *testing.T) {
	tests := []struct {
		name                     string
		discovery, usm           bool
		wantDiscoveryAfterAdjust bool
	}{
		{"discovery only", true, false, true},
		{"both on — usm wins, discovery disabled", true, true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			setBool(cfg, discoveryKey, tc.discovery)
			setBool(cfg, smNS("enabled"), tc.usm)

			adjustDiscovery(cfg)

			assert.Equal(t, tc.wantDiscoveryAfterAdjust, cfg.GetBool(discoveryKey))
		})
	}
}

func TestAdjustDiscovery_ForceDisablesUnusedProtocols(t *testing.T) {
	tests := []struct {
		name                 string
		discovery            bool
		wantProtocolsEnabled bool
	}{
		{"discovery on disables protocols", true, false},
		{"discovery off leaves protocols alone", false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			setBool(cfg, discoveryKey, tc.discovery)
			for _, key := range protocolsDisabledByDiscovery {
				setBool(cfg, key, true)
			}

			adjustDiscovery(cfg)

			for _, key := range protocolsDisabledByDiscovery {
				assert.Equal(t, tc.wantProtocolsEnabled, cfg.GetBool(key),
					"unexpected value for %s", key)
			}
		})
	}
}

func TestAdjustDiscovery_EnablesNetworkTracerModule(t *testing.T) {
	_ = mock.NewSystemProbe(t)
	t.Setenv("DD_DISCOVERY_SERVICE_MAP_ENABLED", "true")

	cfg, err := New("", "")
	require.NoError(t, err)
	assert.True(t, cfg.ModuleIsEnabled(NetworkTracerModule))
}
