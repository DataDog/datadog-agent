// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package config

import (
	"fmt"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestEventMonitor(t *testing.T) {
	mock.NewSystemProbe(t)

	for i, tc := range []struct {
		cws, fim, processEvents, networkEvents, gpu bool
		usmEvents                                   bool
		enabled                                     bool
	}{
		{cws: false, fim: false, processEvents: false, networkEvents: false, enabled: false},
		{cws: false, fim: false, processEvents: true, networkEvents: false, enabled: true},
		{cws: false, fim: true, processEvents: false, networkEvents: false, enabled: true},
		{cws: false, fim: true, processEvents: true, networkEvents: false, enabled: true},
		{cws: true, fim: false, processEvents: false, networkEvents: false, enabled: true},
		{cws: true, fim: false, processEvents: true, networkEvents: false, enabled: true},
		{cws: true, fim: true, processEvents: false, networkEvents: false, enabled: true},
		{cws: true, fim: true, processEvents: true, networkEvents: false, enabled: true},
		{cws: false, fim: false, processEvents: false, networkEvents: true, enabled: true},
		{cws: false, fim: false, processEvents: true, networkEvents: true, enabled: true},
		{cws: false, fim: true, processEvents: false, networkEvents: true, enabled: true},
		{cws: false, fim: true, processEvents: true, networkEvents: true, enabled: true},
		{cws: true, fim: false, processEvents: false, networkEvents: true, enabled: true},
		{cws: true, fim: false, processEvents: true, networkEvents: true, enabled: true},
		{cws: true, fim: true, processEvents: false, networkEvents: true, enabled: true},
		{cws: true, fim: true, processEvents: true, networkEvents: true, enabled: true},
		{cws: false, fim: false, processEvents: false, networkEvents: false, gpu: true, enabled: true},
		{usmEvents: true, enabled: true},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			t.Logf("%+v\n", tc)
			t.Setenv("DD_RUNTIME_SECURITY_CONFIG_ENABLED", strconv.FormatBool(tc.cws))
			t.Setenv("DD_RUNTIME_SECURITY_CONFIG_FIM_ENABLED", strconv.FormatBool(tc.fim))
			t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_PROCESS_ENABLED", strconv.FormatBool(tc.processEvents))
			t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED", strconv.FormatBool(tc.networkEvents))
			t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLED", strconv.FormatBool(tc.networkEvents))
			t.Setenv("DD_GPU_MONITORING_ENABLED", strconv.FormatBool(tc.gpu))
			t.Setenv("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", strconv.FormatBool(tc.usmEvents))
			t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_EVENT_STREAM", strconv.FormatBool(tc.usmEvents))

			cfg, err := New("/doesnotexist", "")
			t.Logf("%+v\n", cfg)
			require.NoError(t, err)
			assert.Equal(t, tc.enabled, cfg.ModuleIsEnabled(EventMonitorModule))
		})
	}
}

func TestEventStreamEnabledForSupportedKernelsWindowsUnsupported(t *testing.T) {
	t.Run("does nothing for unsupported", func(t *testing.T) {
		if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
			t.Skip("This is only for unsupported")
		}
		t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED", strconv.FormatBool(true))
		cfg := mock.NewSystemProbe(t)
		Adjust(cfg)

		require.False(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
	})
}

func TestEnableDiscovery(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		cfg := mock.NewSystemProbe(t)
		cfg.SetWithoutSource("discovery.enabled", true)
		assert.True(t, cfg.GetBool(discoveryNS("enabled")))
	})

	t.Run("via ENV variable", func(t *testing.T) {
		t.Setenv("DD_DISCOVERY_ENABLED", "true")
		cfg := mock.NewSystemProbe(t)
		assert.True(t, cfg.GetBool(discoveryNS("enabled")))
	})

	t.Run("default", func(t *testing.T) {
		cfg := mock.NewSystemProbe(t)
		assert.False(t, cfg.GetBool(discoveryNS("enabled")))
	})
}
