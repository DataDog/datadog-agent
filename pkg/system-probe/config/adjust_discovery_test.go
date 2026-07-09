// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

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

// setBool sets a bool config key from an explicit source so the mock treats
// it as user-configured (vs. a default).
func setBool(cfg model.Config, key string, value bool) {
	cfg.Set(key, value, model.SourceUnknown)
}

func setInt(cfg model.Config, key string, value int) {
	cfg.Set(key, value, model.SourceUnknown)
}

func TestAdjustDiscoveryServiceCollectionBatchSize(t *testing.T) {
	key := discoveryNS("service_collection_batch_size")
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"positive value preserved", 123, 123},
		{"zero disables batching", 0, 0},
		{"negative value falls back to default", -1, defaultDiscoveryServiceCollectionBatchSize},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			setInt(cfg, key, tc.input)

			adjustDiscovery(cfg)

			assert.Equal(t, tc.want, cfg.GetInt(key))
		})
	}
}

func TestAdjustDiscoveryServiceCollectionMaxConsecutiveTimeouts(t *testing.T) {
	key := discoveryNS("service_collection_max_consecutive_timeouts")
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"positive value preserved", 7, 7},
		{"zero disables timeout guard", 0, 0},
		{"negative value falls back to default", -1, defaultDiscoveryServiceCollectionMaxConsecutiveTimeouts},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			setInt(cfg, key, tc.input)

			adjustDiscovery(cfg)

			assert.Equal(t, tc.want, cfg.GetInt(key))
		})
	}
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

func TestAdjustDiscovery_DisablesWhenSKTracerEnabled(t *testing.T) {
	// SK tracer disables USM in adjustNetwork; discovery requires USM, so
	// adjustDiscovery must disable itself rather than silently fail later.
	cfg := mock.NewSystemProbe(t)
	setBool(cfg, discoveryKey, true)
	setBool(cfg, netNS("enable_sk_tracer"), true)

	adjustDiscovery(cfg)

	assert.False(t, cfg.GetBool(discoveryKey),
		"discovery should be disabled when sk tracer is enabled")
}

func TestAdjustDiscovery_DisablesWhenEbpfless(t *testing.T) {
	// eBPF-less mode is rejected by CheckUSMSupported; discovery requires
	// USM, so adjustDiscovery must disable itself rather than silently
	// fail later with a misleading "USM unsupported" error.
	cfg := mock.NewSystemProbe(t)
	setBool(cfg, discoveryKey, true)
	setBool(cfg, netNS("enable_ebpfless"), true)

	adjustDiscovery(cfg)

	assert.False(t, cfg.GetBool(discoveryKey),
		"discovery should be disabled when ebpfless is enabled")
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
			for _, key := range discoveryForceDisabledProtocols {
				setBool(cfg, key, true)
			}

			adjustDiscovery(cfg)

			for _, key := range discoveryForceDisabledProtocols {
				assert.Equal(t, tc.wantProtocolsEnabled, cfg.GetBool(key),
					"unexpected value for %s", key)
			}
		})
	}
}

func TestAdjustDiscovery_ForceEnablesRequiredProtocols(t *testing.T) {
	tests := []struct {
		name            string
		discovery       bool
		userSetTo       bool // explicit user value before adjust
		wantAfterAdjust bool
	}{
		{"discovery on overrides explicit disable", true, false, true},
		{"discovery on keeps explicit enable", true, true, true},
		{"discovery off leaves explicit disable alone", false, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			setBool(cfg, discoveryKey, tc.discovery)
			for _, key := range discoveryForceEnabledProtocols {
				setBool(cfg, key, tc.userSetTo)
			}

			adjustDiscovery(cfg)

			for _, key := range discoveryForceEnabledProtocols {
				assert.Equal(t, tc.wantAfterAdjust, cfg.GetBool(key),
					"unexpected value for %s", key)
			}
		})
	}
}

// TestAdjustDiscovery_ForceEnablesHTTP2 pins the requirement that discovery
// mode force-enables HTTP/2 (like HTTP) rather than leaving it off. It guards
// against http2 being accidentally moved back into the force-disabled set.
func TestAdjustDiscovery_ForceEnablesHTTP2(t *testing.T) {
	http2Key := smNS("http2", "enabled")

	assert.Contains(t, discoveryForceEnabledProtocols, http2Key,
		"http2 should be force-enabled in discovery mode")
	assert.NotContains(t, discoveryForceDisabledProtocols, http2Key,
		"http2 should not be force-disabled in discovery mode")

	cfg := mock.NewSystemProbe(t)
	setBool(cfg, discoveryKey, true)
	setBool(cfg, http2Key, false) // user explicitly disabled it

	adjustDiscovery(cfg)

	assert.True(t, cfg.GetBool(http2Key),
		"discovery mode should override an explicit http2 disable")
}

func TestAdjustDiscovery_ForceEnablesProcessServiceInference(t *testing.T) {
	psiKey := spNS("process_service_inference", "enabled")
	tests := []struct {
		name      string
		discovery bool
		userSetTo bool
		want      bool
	}{
		{"discovery on overrides explicit disable", true, false, true},
		{"discovery on keeps explicit enable", true, true, true},
		{"discovery off leaves explicit disable alone", false, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			setBool(cfg, discoveryKey, tc.discovery)
			setBool(cfg, psiKey, tc.userSetTo)

			adjustDiscovery(cfg)

			assert.Equal(t, tc.want, cfg.GetBool(psiKey),
				"unexpected value for %s", psiKey)
		})
	}
}

func TestAdjustDiscovery_ForceEnablesUSMConnectionRollup(t *testing.T) {
	rollupKey := smNS("enable_connection_rollup")
	tests := []struct {
		name      string
		discovery bool
		userSetTo bool
		want      bool
	}{
		{"discovery on overrides explicit disable", true, false, true},
		{"discovery on keeps explicit enable", true, true, true},
		{"discovery off leaves explicit disable alone", false, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			setBool(cfg, discoveryKey, tc.discovery)
			setBool(cfg, rollupKey, tc.userSetTo)

			adjustDiscovery(cfg)

			assert.Equal(t, tc.want, cfg.GetBool(rollupKey),
				"unexpected value for %s", rollupKey)
		})
	}
}

// TestAdjustDiscovery_FullAdjustKeepsProcessServiceInference is a regression
// test for the bug where the post-adjust guard in Adjust() captured the
// pre-adjust value of service_monitoring_config.enabled and would silently
// re-disable process_service_inference after adjustDiscovery had just
// force-enabled it in discovery-only mode.
func TestAdjustDiscovery_FullAdjustKeepsProcessServiceInference(t *testing.T) {
	psiKey := spNS("process_service_inference", "enabled")
	cfg := mock.NewSystemProbe(t)
	setBool(cfg, discoveryKey, true)

	Adjust(cfg)

	assert.True(t, cfg.GetBool(psiKey),
		"process_service_inference should remain enabled after the full Adjust() in discovery-only mode")
	assert.True(t, cfg.GetBool(smNS("enabled")),
		"service_monitoring_config.enabled should be force-enabled in discovery mode")
}

func TestAdjustDiscovery_EnablesNetworkTracerModule(t *testing.T) {
	_ = mock.NewSystemProbe(t)
	t.Setenv("DD_DISCOVERY_SERVICE_MAP_ENABLED", "true")

	cfg, err := New("", "")
	require.NoError(t, err)
	assert.True(t, cfg.ModuleIsEnabled(NetworkTracerModule))
}
