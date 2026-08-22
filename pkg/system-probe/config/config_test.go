// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package config

import (
	"bytes"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestEventMonitor(t *testing.T) {
	mock.NewSystemProbe(t)

	for i, tc := range []struct {
		cws, fim, networkEvents, gpu bool
		gpuEBPFProbes                bool
		usmEvents                    bool
		enabled                      bool
	}{
		{cws: false, fim: false, networkEvents: false, enabled: false},
		{cws: false, fim: true, networkEvents: false, enabled: true},
		{cws: true, fim: false, networkEvents: false, enabled: true},
		{cws: true, fim: true, networkEvents: false, enabled: true},
		{cws: false, fim: false, networkEvents: true, enabled: true},
		{cws: false, fim: true, networkEvents: true, enabled: true},
		{cws: true, fim: false, networkEvents: true, enabled: true},
		{cws: true, fim: true, networkEvents: true, enabled: true},
		// GPU monitoring only needs the event monitor to feed its eBPF probes,
		// so both settings have to be enabled for the module to be pulled in.
		{cws: false, fim: false, networkEvents: false, gpu: true, gpuEBPFProbes: true, enabled: true},
		{cws: false, fim: false, networkEvents: false, gpu: true, gpuEBPFProbes: false, enabled: false},
		{cws: false, fim: false, networkEvents: false, gpu: false, gpuEBPFProbes: true, enabled: false},
		{usmEvents: true, enabled: true},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Logf("%+v\n", tc)
			t.Setenv("DD_RUNTIME_SECURITY_CONFIG_ENABLED", strconv.FormatBool(tc.cws))
			t.Setenv("DD_RUNTIME_SECURITY_CONFIG_FIM_ENABLED", strconv.FormatBool(tc.fim))
			t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED", strconv.FormatBool(tc.networkEvents))
			t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLED", strconv.FormatBool(tc.networkEvents))
			t.Setenv("DD_GPU_MONITORING_ENABLED", strconv.FormatBool(tc.gpu))
			// Set explicitly rather than relying on the default, which is
			// subject to change as the eBPF probes are deprecated.
			t.Setenv("DD_GPU_MONITORING_ENABLE_EBPF_PROBES", strconv.FormatBool(tc.gpuEBPFProbes))
			t.Setenv("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", strconv.FormatBool(tc.usmEvents))
			t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_EVENT_STREAM", strconv.FormatBool(tc.usmEvents))

			cfg, err := New("/doesnotexist", "")
			t.Logf("%+v\n", cfg)
			require.NoError(t, err)
			assert.Equal(t, tc.enabled, cfg.ModuleIsEnabled(EventMonitorModule))
		})
	}
}

func TestTracerouteModule(t *testing.T) {
	boolPtr := func(value bool) *bool { return &value }
	tests := []struct {
		name                    string
		npmEnabled              bool
		standardTestsEnabled    bool
		baselineTestsEnabled    bool
		tracerouteEnabled       *bool
		expectTracerouteEnabled bool
		expectWarning           bool
	}{
		{name: "disabled by default"},
		{name: "NPM alone does not enable traceroute", npmEnabled: true},
		{name: "standard tests require NPM", standardTestsEnabled: true},
		{name: "baseline tests require NPM", baselineTestsEnabled: true},
		{name: "standard tests enable traceroute", npmEnabled: true, standardTestsEnabled: true, expectTracerouteEnabled: true},
		{name: "baseline tests enable traceroute", npmEnabled: true, baselineTestsEnabled: true, expectTracerouteEnabled: true},
		{name: "explicit true enables traceroute", tracerouteEnabled: boolPtr(true), expectTracerouteEnabled: true},
		{name: "explicit false without dynamic tests", npmEnabled: true, tracerouteEnabled: boolPtr(false)},
		{name: "explicit false overrides standard tests", npmEnabled: true, standardTestsEnabled: true, tracerouteEnabled: boolPtr(false), expectWarning: true},
		{name: "explicit false overrides baseline tests", npmEnabled: true, baselineTestsEnabled: true, tracerouteEnabled: boolPtr(false), expectWarning: true},
	}

	var logs bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(&logs, log.WarnLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "warn")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "debug") })

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coreCfg := mock.New(t)
			sysprobeCfg := mock.NewSystemProbe(t)
			coreCfg.SetInTest("network_path.connections_monitoring.enabled", test.standardTestsEnabled)
			coreCfg.SetInTest("network_path.connections_monitoring.baseline_tests_enabled", test.baselineTestsEnabled)
			sysprobeCfg.SetInTest("network_config.enabled", test.npmEnabled)
			if test.tracerouteEnabled != nil {
				sysprobeCfg.SetInTest("traceroute.enabled", *test.tracerouteEnabled)
			}
			assert.Equal(t, test.tracerouteEnabled != nil, sysprobeCfg.IsConfigured("traceroute.enabled"))

			logs.Reset()
			cfg, err := load()
			require.NoError(t, err)

			assert.Equal(t, test.expectTracerouteEnabled, cfg.ModuleIsEnabled(TracerouteModule))
			assert.Equal(t, test.tracerouteEnabled != nil || test.expectTracerouteEnabled, sysprobeCfg.IsConfigured("traceroute.enabled"))
			assert.Equal(t, test.expectTracerouteEnabled, sysprobeCfg.GetBool("traceroute.enabled"))
			assert.Equal(t, test.expectWarning, bytes.Contains(logs.Bytes(), []byte("system-probe traceroute was explicitly disabled")))
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
		cfg.SetInTest("discovery.enabled", true)
		assert.True(t, cfg.GetBool(discoveryNS("enabled")))
	})

	t.Run("via ENV variable", func(t *testing.T) {
		t.Setenv("DD_DISCOVERY_ENABLED", "true")
		cfg := mock.NewSystemProbe(t)
		assert.True(t, cfg.GetBool(discoveryNS("enabled")))
	})

	t.Run("default", func(t *testing.T) {
		cfg := mock.NewSystemProbe(t)
		assert.Equal(t, runtime.GOOS == "linux", cfg.GetBool(discoveryNS("enabled")))
	})

	t.Run("default disabled on ECS Fargate", func(t *testing.T) {
		t.Setenv("AWS_EXECUTION_ENV", "AWS_ECS_FARGATE")

		// Reset global config to avoid test interference.
		_ = mock.NewSystemProbe(t)

		cfg, err := New("", "")
		require.NoError(t, err)
		assert.False(t, cfg.ModuleIsEnabled(DiscoveryModule))
	})

	discoveryDefaultEnabled := runtime.GOOS == "linux"

	t.Run("default enabled with USM", func(t *testing.T) {
		// Reset global config to avoid test interference
		_ = mock.NewSystemProbe(t)

		t.Setenv("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", "true")

		cfg, err := New("", "")
		require.NoError(t, err)
		assert.Equal(t, discoveryDefaultEnabled, cfg.ModuleIsEnabled(DiscoveryModule))
	})

	t.Run("default enabled with NPM", func(t *testing.T) {
		// Reset global config to avoid test interference
		_ = mock.NewSystemProbe(t)

		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLED", "true")

		cfg, err := New("", "")
		require.NoError(t, err)
		assert.Equal(t, discoveryDefaultEnabled, cfg.ModuleIsEnabled(DiscoveryModule))
	})

	t.Run("default enabled standalone on linux", func(t *testing.T) {
		// Reset global config to avoid test interference
		_ = mock.NewSystemProbe(t)

		// No other modules enabled — discovery should still auto-enable on Linux
		cfg, err := New("", "")
		require.NoError(t, err)
		assert.Equal(t, discoveryDefaultEnabled, cfg.ModuleIsEnabled(DiscoveryModule))
	})

	t.Run("force disabled with USM via env var", func(t *testing.T) {
		// Reset global config to avoid test interference
		_ = mock.NewSystemProbe(t)

		t.Setenv("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", "true")
		t.Setenv("DD_DISCOVERY_ENABLED", "false")

		cfg, err := New("", "")
		require.NoError(t, err)
		assert.False(t, cfg.ModuleIsEnabled(DiscoveryModule))
	})

	t.Run("force disabled with USM via config file", func(t *testing.T) {
		// Reset global config to avoid test interference
		mockCfg := mock.NewSystemProbe(t)

		t.Setenv("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", "true")
		mockCfg.SetInTest("discovery.enabled", false)

		cfg, err := New("", "")
		require.NoError(t, err)
		assert.False(t, cfg.ModuleIsEnabled(DiscoveryModule))
	})
}
