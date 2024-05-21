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

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestNetworkProcessEventMonitoring(t *testing.T) {
	newConfig(t)

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

			cfg, err := New("")
			require.NoError(t, err)
			assert.Equal(t, te.enabled, cfg.ModuleIsEnabled(EventMonitorModule))
		})
	}

}

func TestDynamicInstrumentation(t *testing.T) {
	newConfig(t)
	os.Setenv("DD_DYNAMIC_INSTRUMENTATION_ENABLED", "true")
	defer os.Unsetenv("DD_DYNAMIC_INSTRUMENTATION_ENABLED")

	cfg, err := New("")
	require.NoError(t, err)
	assert.Equal(t, true, cfg.ModuleIsEnabled(DynamicInstrumentationModule))

	os.Unsetenv("DD_DYNAMIC_INSTRUMENTATION_ENABLED")
	cfg, err = New("")
	require.NoError(t, err)
	assert.Equal(t, false, cfg.ModuleIsEnabled(DynamicInstrumentationModule))

}

func TestEventStreamEnabledForSupportedKernelsLinux(t *testing.T) {
	config.ResetSystemProbeConfig(t)
	t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED", strconv.FormatBool(true))

	cfg := config.SystemProbe
	Adjust(cfg)

	if ProcessEventDataStreamSupported() {
		require.True(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
	} else {
		require.False(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
	}
}

func TestNPMEnabled(t *testing.T) {
	tests := []struct {
		npm, usm, ccm bool
		npmEnabled    bool
	}{
		{false, false, false, false},
		{false, false, true, true},
		{false, true, false, true},
		{false, true, true, true},
		{true, false, false, true},
		{true, false, true, true},
		{true, true, false, true},
		{true, true, true, true},
	}

	newConfig(t)
	for _, te := range tests {
		t.Run("", func(t *testing.T) {
			t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLED", strconv.FormatBool(te.npm))
			t.Setenv("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", strconv.FormatBool(te.usm))
			t.Setenv("DD_CCM_NETWORK_CONFIG_ENABLED", strconv.FormatBool(te.ccm))
			cfg, err := New("")
			require.NoError(t, err)
			assert.Equal(t, te.npmEnabled, cfg.ModuleIsEnabled(NetworkTracerModule), "unexpected network tracer module enablement: npm: %v, usm: %v, ccm: %v", te.npm, te.usm, te.ccm)
		})
	}
}
