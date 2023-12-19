// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	aconfig "github.com/DataDog/datadog-agent/pkg/config"
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

func newSystemProbeConfig(t *testing.T) {
	originalConfig := aconfig.SystemProbe
	t.Cleanup(func() {
		aconfig.SystemProbe = originalConfig
	})
	aconfig.SystemProbe = aconfig.NewConfig("system-probe", "DD", strings.NewReplacer(".", "_"))
	aconfig.InitSystemProbeConfig(aconfig.SystemProbe)
}

func TestEventStreamEnabledForSupportedKernelsLinux2(t *testing.T) {

	newSystemProbeConfig(t)
	t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED", strconv.FormatBool(true))

	cfg := aconfig.SystemProbe
	sysconfig.Adjust(cfg)

	if processEventDataStreamSupported() {
		require.True(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
		sysProbeConfig, err := sysconfig.New("")
		require.NoError(t, err)

		emconfig := emconfig.NewConfig(sysProbeConfig)
		secconfig, err := secconfig.NewConfig()
		require.NoError(t, err)

		opts := eventmonitor.Opts{}
		evm, err := eventmonitor.NewEventMonitor(emconfig, secconfig, opts)
		require.NoError(t, err)
		require.NoError(t, evm.Init())
	} else {
		require.False(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
	}
}
