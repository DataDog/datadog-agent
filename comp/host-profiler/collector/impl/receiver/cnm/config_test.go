// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cnm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig().(*Config)

	assert.True(t, cfg.CollectTCPv4)
	assert.True(t, cfg.CollectTCPv6)
	assert.True(t, cfg.CollectUDPv4)
	assert.True(t, cfg.CollectUDPv6)
	assert.True(t, cfg.DNSInspection)
	assert.True(t, cfg.CollectDNSStats)
	assert.True(t, cfg.ProtocolClassification)
	assert.True(t, cfg.EnableConntrack)
	assert.Equal(t, defaultMaxTrackedConnections, cfg.MaxTrackedConnections)
	assert.Equal(t, defaultMaxConnsPerMessage, cfg.MaxConnsPerMessage)
	assert.Equal(t, defaultCheckInterval, cfg.CheckInterval)
}

func TestConfigValidate(t *testing.T) {
	t.Run("valid default config", func(t *testing.T) {
		cfg := defaultConfig().(*Config)
		require.NoError(t, cfg.Validate())
	})

	t.Run("zero max tracked connections", func(t *testing.T) {
		cfg := defaultConfig().(*Config)
		cfg.MaxTrackedConnections = 0
		require.Error(t, cfg.Validate())
	})

	t.Run("negative max conns per message", func(t *testing.T) {
		cfg := defaultConfig().(*Config)
		cfg.MaxConnsPerMessage = -1
		require.Error(t, cfg.Validate())
	})

	t.Run("zero check interval", func(t *testing.T) {
		cfg := defaultConfig().(*Config)
		cfg.CheckInterval = 0
		require.Error(t, cfg.Validate())
	})
}

func TestConfigUnmarshal(t *testing.T) {
	cm := confmap.NewFromStringMap(map[string]any{
		"collect_tcp_v4":          false,
		"collect_tcp_v6":          true,
		"collect_udp_v4":          false,
		"collect_udp_v6":          true,
		"dns_inspection":          false,
		"collect_dns_stats":       false,
		"max_tracked_connections": 32768,
		"max_conns_per_message":   500,
		"check_interval":          "10s",
		"protocol_classification": false,
		"enable_conntrack":        false,
	})

	cfg := defaultConfig().(*Config)
	require.NoError(t, cm.Unmarshal(cfg))

	assert.False(t, cfg.CollectTCPv4)
	assert.True(t, cfg.CollectTCPv6)
	assert.False(t, cfg.CollectUDPv4)
	assert.True(t, cfg.CollectUDPv6)
	assert.False(t, cfg.DNSInspection)
	assert.False(t, cfg.CollectDNSStats)
	assert.Equal(t, 32768, cfg.MaxTrackedConnections)
	assert.Equal(t, 500, cfg.MaxConnsPerMessage)
	assert.Equal(t, 10*time.Second, cfg.CheckInterval)
	assert.False(t, cfg.ProtocolClassification)
	assert.False(t, cfg.EnableConntrack)
}

func TestToNetworkConfig(t *testing.T) {
	cfg := defaultConfig().(*Config)
	cfg.CollectTCPv4 = true
	cfg.CollectUDPv6 = false
	cfg.MaxTrackedConnections = 32768
	cfg.DNSInspection = false

	netCfg := cfg.toNetworkConfig()

	assert.True(t, netCfg.NPMEnabled)
	assert.True(t, netCfg.CollectTCPv4Conns)
	assert.True(t, netCfg.CollectTCPv6Conns)
	assert.True(t, netCfg.CollectUDPv4Conns)
	assert.False(t, netCfg.CollectUDPv6Conns)
	assert.Equal(t, uint32(32768), netCfg.MaxTrackedConnections)
	assert.False(t, netCfg.DNSInspection)

	// Verify defaults for unexposed fields
	assert.Equal(t, 2*time.Minute, netCfg.TCPConnTimeout)
	assert.Equal(t, 30*time.Second, netCfg.UDPConnTimeout)
	assert.Equal(t, 120*time.Second, netCfg.UDPStreamTimeout)
	assert.Equal(t, 2*time.Minute, netCfg.ClientStateExpiry)
	assert.Equal(t, defaultMaxDNSStats, netCfg.MaxDNSStats)
	assert.Equal(t, defaultMaxDNSStatsBuffered, netCfg.MaxDNSStatsBuffered)
	assert.Equal(t, defaultDNSTimeout, netCfg.DNSTimeout)
	assert.Equal(t, []int{53}, netCfg.DNSMonitoringPortList)
	assert.True(t, netCfg.EnableConntrack)
	assert.True(t, netCfg.EnableEbpfConntracker)
	assert.True(t, netCfg.EnableConntrackAllNamespaces)
	assert.True(t, netCfg.EnableCORETracer)
	assert.True(t, netCfg.EnableFentry)
}

func TestToNetworkConfigDNSLinkage(t *testing.T) {
	cfg := defaultConfig().(*Config)
	cfg.DNSInspection = true
	cfg.CollectDNSStats = true

	netCfg := cfg.toNetworkConfig()

	assert.True(t, netCfg.DNSInspection)
	assert.True(t, netCfg.CollectDNSStats)
}
