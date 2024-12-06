// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestNetworkProcessEventMonitoring(t *testing.T) {
	mock.NewSystemProbe(t)

	for i, te := range []struct {
		network, netProcEvents bool
		enabled                bool
	}{
		{network: false, netProcEvents: false, enabled: false},
		{network: false, netProcEvents: true, enabled: false},
		{network: true, netProcEvents: false, enabled: false},
		{network: true, netProcEvents: true, enabled: true},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			os.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLED", strconv.FormatBool(te.network))
			os.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED", strconv.FormatBool(te.netProcEvents))
			defer os.Unsetenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED")
			defer os.Unsetenv("DD_SYSTEM_PROBE_NETWORK_ENABLED")

			cfg, err := New("", "")
			require.NoError(t, err)
			assert.Equal(t, te.enabled, cfg.ModuleIsEnabled(EventMonitorModule))
		})
	}

}

func TestDynamicInstrumentation(t *testing.T) {
	mock.NewSystemProbe(t)
	os.Setenv("DD_DYNAMIC_INSTRUMENTATION_ENABLED", "true")
	defer os.Unsetenv("DD_DYNAMIC_INSTRUMENTATION_ENABLED")

	cfg, err := New("", "")
	require.NoError(t, err)
	assert.Equal(t, true, cfg.ModuleIsEnabled(DynamicInstrumentationModule))

	os.Unsetenv("DD_DYNAMIC_INSTRUMENTATION_ENABLED")
	cfg, err = New("", "")
	require.NoError(t, err)
	assert.Equal(t, false, cfg.ModuleIsEnabled(DynamicInstrumentationModule))

}

func TestEventStreamEnabledForSupportedKernelsLinux(t *testing.T) {
	t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED", strconv.FormatBool(true))
	cfg := mock.NewSystemProbe(t)
	Adjust(cfg)

	if ProcessEventDataStreamSupported() {
		require.True(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
	} else {
		require.False(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
	}
}

func TestNPMEnabled(t *testing.T) {
	tests := []struct {
		npm, usm, ccm, csm, csmNpm bool
		npmEnabled                 bool
	}{
		{false, false, false, false, false, false},
		{false, false, true, false, false, true},
		{false, true, false, false, false, true},
		{false, true, true, false, false, true},
		{true, false, false, false, false, true},
		{true, false, true, false, false, true},
		{true, true, false, false, false, true},
		{true, true, true, false, false, true},
		{false, false, false, true, false, false},
		{false, false, false, true, true, true},
		{false, false, true, true, false, true},
		{false, true, false, true, false, true},
		{false, true, true, true, false, true},
		{true, false, false, true, false, true},
		{true, false, true, true, false, true},
		{true, true, false, true, false, true},
		{true, true, true, true, false, true},
	}

	mock.NewSystemProbe(t)
	for _, te := range tests {
		t.Run("", func(t *testing.T) {
			t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLED", strconv.FormatBool(te.npm))
			t.Setenv("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", strconv.FormatBool(te.usm))
			t.Setenv("DD_CCM_NETWORK_CONFIG_ENABLED", strconv.FormatBool(te.ccm))
			t.Setenv("DD_RUNTIME_SECURITY_CONFIG_ENABLED", strconv.FormatBool(te.csm))
			t.Setenv("DD_RUNTIME_SECURITY_CONFIG_NETWORK_MONITORING_ENABLED", strconv.FormatBool(te.csmNpm))
			cfg, err := New("", "")
			require.NoError(t, err)
			assert.Equal(t, te.npmEnabled, cfg.ModuleIsEnabled(NetworkTracerModule), "unexpected network tracer module enablement: npm: %v, usm: %v, ccm: %v", te.npm, te.usm, te.ccm)
		})
	}
}

func TestEbpfPrebuiltFallbackDeprecation(t *testing.T) {
	family, err := kernel.Family()
	require.NoError(t, err, "could not determine kernel family")

	kv, err := kernel.HostVersion()
	require.NoError(t, err, "could not determine kernel version")

	deprecateVersion := prebuilt.DeprecatedKernelVersion
	if family == "rhel" {
		deprecateVersion = prebuilt.DeprecatedKernelVersionRhel
	}

	t.Run("default", func(t *testing.T) {
		cfg := mock.NewSystemProbe(t)
		assert.False(t, cfg.GetBool(allowPrebuiltFallbackKey))
		assert.False(t, cfg.GetBool(allowPrecompiledFallbackKey))
	})

	t.Run("not set in config", func(t *testing.T) {
		cfg := mock.NewSystemProbe(t)
		Adjust(cfg)

		switch {
		case kv < deprecateVersion:
			assert.True(t, cfg.GetBool(allowPrebuiltFallbackKey))
		default:
			// deprecated
			assert.False(t, cfg.GetBool(allowPrebuiltFallbackKey))
		}
	})

	// if system_probe_config.allow_precompiled_fallback is set
	// in config, that value should not be changed by Adjust() and
	// the new key, `system_probe_config.allow_prebuilt_fallback`
	// should be set to the same value
	t.Run("allow_precompiled_fallback set in config", func(t *testing.T) {
		t.Run("true", func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			cfg.Set(allowPrecompiledFallbackKey, true, model.SourceDefault)
			Adjust(cfg)

			assert.True(t, cfg.GetBool(allowPrecompiledFallbackKey))
			assert.Equal(t, cfg.GetBool(allowPrecompiledFallbackKey), cfg.GetBool(allowPrebuiltFallbackKey))
		})

		t.Run("false", func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			cfg.Set(allowPrecompiledFallbackKey, false, model.SourceDefault)
			Adjust(cfg)

			assert.False(t, cfg.GetBool(allowPrecompiledFallbackKey))
			assert.Equal(t, cfg.GetBool(allowPrecompiledFallbackKey), cfg.GetBool(allowPrebuiltFallbackKey))
		})
	})

	// if system_probe_config.allow_prebuilt_fallback is set
	// in config, that value should not be changed by Adjust()
	t.Run("allow_prebuilt_fallback set in config", func(t *testing.T) {
		t.Run("true", func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			cfg.Set(allowPrebuiltFallbackKey, true, model.SourceDefault)
			Adjust(cfg)

			assert.True(t, cfg.GetBool(allowPrebuiltFallbackKey))
		})

		t.Run("false", func(t *testing.T) {
			cfg := mock.NewSystemProbe(t)
			cfg.Set(allowPrebuiltFallbackKey, false, model.SourceDefault)
			Adjust(cfg)

			assert.False(t, cfg.GetBool(allowPrebuiltFallbackKey))
		})
	})
}
