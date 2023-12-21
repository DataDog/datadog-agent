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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func newConfig(t *testing.T) {
	originalConfig := config.SystemProbe
	t.Cleanup(func() {
		config.SystemProbe = originalConfig
	})
	config.SystemProbe = config.NewConfig("system-probe", "DD", strings.NewReplacer(".", "_"))
	config.InitSystemProbeConfig(config.SystemProbe)
}

func TestEventMonitor(t *testing.T) {
	newConfig(t)

	for i, tc := range []struct {
		cws, fim, processEvents, networkEvents bool
		enabled                                bool
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
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			t.Logf("%+v\n", tc)
			t.Setenv("DD_RUNTIME_SECURITY_CONFIG_ENABLED", strconv.FormatBool(tc.cws))
			t.Setenv("DD_RUNTIME_SECURITY_CONFIG_FIM_ENABLED", strconv.FormatBool(tc.fim))
			t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_PROCESS_ENABLED", strconv.FormatBool(tc.processEvents))
			t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED", strconv.FormatBool(tc.networkEvents))
			t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLED", strconv.FormatBool(tc.networkEvents))

			cfg, err := New("/doesnotexist")
			t.Logf("%+v\n", cfg)
			require.NoError(t, err)
			assert.Equal(t, tc.enabled, cfg.ModuleIsEnabled(EventMonitorModule))
		})
	}
}

func TestEventStreamEnabledForSupportedKernelsWindowsUnsupported(t *testing.T) {
	t.Run("does nothing for windows", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("This is only for windows")
		}
		config.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED", strconv.FormatBool(true))

		cfg := config.SystemProbe
		Adjust(cfg)

		require.False(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
	})
	t.Run("does nothing for unsupported", func(t *testing.T) {
		if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
			t.Skip("This is only for unsupported")
		}
		config.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED", strconv.FormatBool(true))

		cfg := config.SystemProbe
		Adjust(cfg)

		require.False(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
	})
}
