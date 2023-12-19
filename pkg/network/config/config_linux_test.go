// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	aconfig "github.com/DataDog/datadog-agent/pkg/config"
	ebpfkernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestDisableRootNetNamespace(t *testing.T) {
	aconfig.ResetSystemProbeConfig(t)
	t.Setenv("DD_NETWORK_CONFIG_ENABLE_ROOT_NETNS", "false")

	cfg := New()
	require.False(t, cfg.EnableConntrackAllNamespaces)
	require.False(t, cfg.EnableRootNetNs)

	rootNs, err := cfg.GetRootNetNs()
	require.NoError(t, err)
	defer rootNs.Close()
	require.False(t, netns.None().Equal(rootNs))

	ns, err := netns.GetFromPid(os.Getpid())
	require.NoError(t, err)
	defer ns.Close()
	require.True(t, ns.Equal(rootNs))
}

func TestEventStreamEnabledForSupportedKernelsLinux(t *testing.T) {
	t.Run("for kernels <4.15.0", func(t *testing.T) {
		kv, err := ebpfkernel.NewKernelVersion()
		kv4150 := kernel.VersionCode(4, 15, 0)
		require.NoError(t, err)
		if kv.Code >= kv4150 || kv.IsRH8Kernel() || kv.IsRH7Kernel() {
			t.Skip("This test should only be run on kernels < 4.15.0")
		}
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED", strconv.FormatBool(true))

		cfg := aconfig.SystemProbe
		sysconfig.Adjust(cfg)

		require.False(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
	})
	t.Run("for kernels >=4.15.0 with default value", func(t *testing.T) {
		kv, err := ebpfkernel.NewKernelVersion()
		kv4150 := kernel.VersionCode(4, 15, 0)
		require.NoError(t, err)
		if kv.Code < kv4150 {
			t.Skip("This test should only be run on kernels >= 4.15.0 or RH7 and 8")
		}
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED", strconv.FormatBool(true))

		cfg := aconfig.SystemProbe
		sysconfig.Adjust(cfg)

		require.True(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
	})
}
