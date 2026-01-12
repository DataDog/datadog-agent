// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux || windows

package config

import (
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

// variables for testing config options
const (
	driverDefaultNotificationThreshold = 512
	driverMaxFragmentLimit             = 512
	validNotificationThreshold         = 100
	invalidNotificationThreshold       = 1200
	invalidHTTPRequestFragment         = 600
)

// ========================================
// Global USM Configuration Tests
// ========================================

func TestUSMEventStream(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		expected := sysconfig.ProcessEventDataStreamSupported()
		assert.Equal(t, expected, cfg.EnableUSMEventStream)
	})

	t.Run("via yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_event_stream", false)
		cfg := New()

		assert.False(t, cfg.EnableUSMEventStream)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_EVENT_STREAM", "false")
		cfg := New()

		assert.False(t, cfg.EnableUSMEventStream)
	})
}

func TestUSMKernelBufferPages(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Equal(t, cfg.USMKernelBufferPages, 16)
	})

	t.Run("via yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.kernel_buffer_pages", 109)
		cfg := New()

		assert.Equal(t, cfg.USMKernelBufferPages, 109)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_KERNEL_BUFFER_PAGES", "109")
		cfg := New()

		assert.Equal(t, cfg.USMKernelBufferPages, 109)
	})
}

func TestUSMDataChannelSize(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Equal(t, cfg.USMDataChannelSize, 100)
	})

	t.Run("via yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.data_channel_size", 109)
		cfg := New()

		assert.Equal(t, cfg.USMDataChannelSize, 109)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_DATA_CHANNEL_SIZE", "109")
		cfg := New()

		assert.Equal(t, cfg.USMDataChannelSize, 109)
	})
}

func TestMaxUSMConcurrentRequests(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		// Assert that if not explicitly set this param defaults to `MaxTrackedConnections`
		// Note this behavior should be deprecated on 7.50
		assert.Equal(t, cfg.MaxTrackedConnections, cfg.MaxUSMConcurrentRequests)
	})

	t.Run("via yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.max_concurrent_requests", 1000)
		cfg := New()

		assert.Equal(t, uint32(1000), cfg.MaxUSMConcurrentRequests)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_CONCURRENT_REQUESTS", "3000")
		cfg := New()

		assert.Equal(t, uint32(3000), cfg.MaxUSMConcurrentRequests)
	})
}

func TestUSMDirectBufferWakeupCount(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()
		assert.Equal(t, 8, cfg.DirectConsumerBufferWakeupCountPerCPU)
	})

	t.Run("via yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.direct_consumer.buffer_wakeup_count_per_cpu", 64)
		cfg := New()
		assert.Equal(t, 64, cfg.DirectConsumerBufferWakeupCountPerCPU)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_DIRECT_CONSUMER_BUFFER_WAKEUP_COUNT_PER_CPU", "128")
		cfg := New()
		assert.Equal(t, 128, cfg.DirectConsumerBufferWakeupCountPerCPU)
	})
}

func TestUSMDirectChannelSize(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()
		assert.Equal(t, 1000, cfg.DirectConsumerChannelSize)
	})

	t.Run("via yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.direct_consumer.channel_size", 2000)
		cfg := New()
		assert.Equal(t, 2000, cfg.DirectConsumerChannelSize)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_DIRECT_CONSUMER_CHANNEL_SIZE", "3000")
		cfg := New()
		assert.Equal(t, 3000, cfg.DirectConsumerChannelSize)
	})
}

func TestUSMDirectKernelBufferSize(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()
		assert.Equal(t, 65536, cfg.DirectConsumerKernelBufferSizePerCPU)
	})

	t.Run("via yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.direct_consumer.kernel_buffer_size_per_cpu", 131072)
		cfg := New()
		assert.Equal(t, 131072, cfg.DirectConsumerKernelBufferSizePerCPU)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_DIRECT_CONSUMER_KERNEL_BUFFER_SIZE_PER_CPU", "262144")
		cfg := New()
		assert.Equal(t, 262144, cfg.DirectConsumerKernelBufferSizePerCPU)
	})
}

// ========================================
// HTTP Protocol Configuration Tests
// ========================================

func TestEnableHTTPMonitoring(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()
		assert.True(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.enable_http_monitoring", false)
		cfg := New()

		assert.False(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING", "false")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.False(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via deprecated flat YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_http_monitoring", false)
		cfg := New()

		assert.False(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via deprecated flat ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP_MONITORING", "false")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.False(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via tree structure YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.enabled", false)
		cfg := New()

		assert.False(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via tree structure ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_ENABLED", "false")
		cfg := New()

		assert.False(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP_MONITORING", "false")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.False(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING", "false")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP_MONITORING", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.True(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("Both enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP_MONITORING", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.True(t, cfg.EnableHTTPMonitoring)
	})
}

func TestHTTPReplaceRules(t *testing.T) {
	expected := []*ReplaceRule{
		{
			Pattern: "/users/(.*)",
			Re:      regexp.MustCompile("/users/(.*)"),
			Repl:    "/users/?",
		},
		{
			Pattern: "foo",
			Re:      regexp.MustCompile("foo"),
			Repl:    "bar",
		},
		{
			Pattern: "payment_id",
			Re:      regexp.MustCompile("payment_id"),
		},
	}

	envContent := `
        [
          {
            "pattern": "/users/(.*)",
            "repl": "/users/?"
          },
          {
            "pattern": "foo",
            "repl": "bar"
          },
          {
            "pattern": "payment_id"
          }
        ]
	`

	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.http_replace_rules", []map[string]string{
			{"pattern": "/users/(.*)", "repl": "/users/?"},
			{"pattern": "foo", "repl": "bar"},
			{"pattern": "payment_id"},
		})
		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_HTTP_REPLACE_RULES", envContent)
		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.http_replace_rules", []map[string]string{
			{"pattern": "/users/(.*)", "repl": "/users/?"},
			{"pattern": "foo", "repl": "bar"},
			{"pattern": "payment_id"},
		})
		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("via deprecated flat ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_REPLACE_RULES", envContent)
		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("via tree structure YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.replace_rules", []map[string]string{
			{"pattern": "/users/(.*)", "repl": "/users/?"},
			{"pattern": "foo", "repl": "bar"},
			{"pattern": "payment_id"},
		})
		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("via tree structure ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_REPLACE_RULES", envContent)
		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_HTTP_REPLACE_RULES", envContent)
		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_REPLACE_RULES", envContent)
		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("Both enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_REPLACE_RULES", envContent)
		cfg := New()

		// Setting a different value for the old value, as we should override.
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_HTTP_REPLACE_RULES", `
        [
          {
            "pattern": "payment_id"
          }
        ]
        `)

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("Not enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Empty(t, cfg.HTTPReplaceRules)
	})
}

func TestMaxTrackedHTTPConnections(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.max_tracked_http_connections", 1025)
		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_MAX_TRACKED_HTTP_CONNECTIONS", "1025")
		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.max_tracked_http_connections", 1025)
		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_TRACKED_HTTP_CONNECTIONS", "1025")
		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_MAX_TRACKED_HTTP_CONNECTIONS", "1025")
		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_TRACKED_HTTP_CONNECTIONS", "1025")
		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("Both enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_MAX_TRACKED_HTTP_CONNECTIONS", "1026")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_TRACKED_HTTP_CONNECTIONS", "1025")
		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("Not enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		// Default value.
		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1024))
	})
}

func TestMaxHTTPStatsBuffered(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.max_http_stats_buffered", 513)
		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_MAX_HTTP_STATS_BUFFERED", "513")
		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.max_http_stats_buffered", 513)
		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_HTTP_STATS_BUFFERED", "513")
		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_MAX_HTTP_STATS_BUFFERED", "513")
		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_HTTP_STATS_BUFFERED", "513")
		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("Both enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_MAX_HTTP_STATS_BUFFERED", "514")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_HTTP_STATS_BUFFERED", "513")
		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("Not enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		// Default value.
		require.Equal(t, cfg.MaxHTTPStatsBuffered, 100000)
	})
}

func TestHTTPMapCleanerInterval(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("system_probe_config.http_map_cleaner_interval_in_s", 1025)
		cfg := New()

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_HTTP_MAP_CLEANER_INTERVAL_IN_S", "1025")
		cfg := New()

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_map_cleaner_interval_in_s", 1025)
		cfg := New()

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAP_CLEANER_INTERVAL_IN_S", "1025")
		cfg := New()

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_HTTP_MAP_CLEANER_INTERVAL_IN_S", "1025")
		cfg := New()

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAP_CLEANER_INTERVAL_IN_S", "1025")
		cfg := New()

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("Both enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_HTTP_MAP_CLEANER_INTERVAL_IN_S", "1026")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAP_CLEANER_INTERVAL_IN_S", "1025")
		cfg := New()

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("Not enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		// Default value.
		require.Equal(t, cfg.HTTPMapCleanerInterval, 300*time.Second)
	})
}

func TestHTTPIdleConnectionTTL(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("system_probe_config.http_idle_connection_ttl_in_s", 1025)
		cfg := New()

		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_HTTP_IDLE_CONNECTION_TTL_IN_S", "1025")
		cfg := New()

		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_idle_connection_ttl_in_s", 1025)
		cfg := New()

		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_IDLE_CONNECTION_TTL_IN_S", "1025")
		cfg := New()

		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_HTTP_IDLE_CONNECTION_TTL_IN_S", "1025")
		cfg := New()

		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_IDLE_CONNECTION_TTL_IN_S", "1025")
		cfg := New()

		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("Both enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_HTTP_IDLE_CONNECTION_TTL_IN_S", "1026")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_IDLE_CONNECTION_TTL_IN_S", "1025")
		cfg := New()

		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("Not enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		// Default value.
		require.Equal(t, cfg.HTTPIdleConnectionTTL, 30*time.Second)
	})
}

func TestHTTPNotificationThreshold(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.http_notification_threshold", validNotificationThreshold)
		cfg := New()
		require.Equal(t, cfg.HTTPNotificationThreshold, int64(validNotificationThreshold))
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_NOTIFICATION_THRESHOLD", strconv.Itoa(validNotificationThreshold))
		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(validNotificationThreshold))
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_notification_threshold", validNotificationThreshold)
		cfg := New()
		require.Equal(t, cfg.HTTPNotificationThreshold, int64(validNotificationThreshold))
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_NOTIFICATION_THRESHOLD", strconv.Itoa(validNotificationThreshold))
		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(validNotificationThreshold))
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_NOTIFICATION_THRESHOLD", strconv.Itoa(validNotificationThreshold))
		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(validNotificationThreshold))
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_NOTIFICATION_THRESHOLD", strconv.Itoa(validNotificationThreshold))
		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(validNotificationThreshold))
	})

	t.Run("Both enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_NOTIFICATION_THRESHOLD", strconv.Itoa(validNotificationThreshold+1))
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_NOTIFICATION_THRESHOLD", strconv.Itoa(validNotificationThreshold))
		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(validNotificationThreshold))
	})

	t.Run("Not enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		// Default value.
		require.Equal(t, cfg.HTTPNotificationThreshold, int64(driverDefaultNotificationThreshold))
	})
}

// Testing we're not exceeding the limit for http_notification_threshold.
func TestHTTPNotificationThresholdOverLimit(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.http_notification_threshold", invalidNotificationThreshold)
		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(driverDefaultNotificationThreshold))
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_NOTIFICATION_THRESHOLD", strconv.Itoa(invalidNotificationThreshold))
		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(driverDefaultNotificationThreshold))
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_notification_threshold", invalidNotificationThreshold)
		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(driverDefaultNotificationThreshold))
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_NOTIFICATION_THRESHOLD", strconv.Itoa(invalidNotificationThreshold))
		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(driverDefaultNotificationThreshold))
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_NOTIFICATION_THRESHOLD", strconv.Itoa(invalidNotificationThreshold))
		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(driverDefaultNotificationThreshold))
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_NOTIFICATION_THRESHOLD", strconv.Itoa(invalidNotificationThreshold))
		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(driverDefaultNotificationThreshold))
	})

	t.Run("Both enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_NOTIFICATION_THRESHOLD", strconv.Itoa(invalidNotificationThreshold+1))
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_NOTIFICATION_THRESHOLD", strconv.Itoa(invalidNotificationThreshold))
		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(driverDefaultNotificationThreshold))
	})

	t.Run("Not enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		// Default value.
		require.Equal(t, cfg.HTTPNotificationThreshold, int64(driverDefaultNotificationThreshold))
	})
}

func TestHTTPMaxRequestFragment(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.http_max_request_fragment", 155)
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "155")
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_max_request_fragment", 155)
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "155")
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "155")
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "155")
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("Both enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "151")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "155")
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("Not enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		// Default value.
		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(driverMaxFragmentLimit))
	})
}

// Testing we're not exceeding the hard coded limit of http_max_request_fragment.
func TestHTTPMaxRequestFragmentLimit(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.http_max_request_fragment", invalidHTTPRequestFragment)
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(driverMaxFragmentLimit))
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", strconv.Itoa(invalidHTTPRequestFragment))
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(driverMaxFragmentLimit))
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_max_request_fragment", invalidHTTPRequestFragment)
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(driverMaxFragmentLimit))
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", strconv.Itoa(invalidHTTPRequestFragment))
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(driverMaxFragmentLimit))
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", strconv.Itoa(invalidHTTPRequestFragment))
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(512))
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", strconv.Itoa(invalidHTTPRequestFragment))
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(driverMaxFragmentLimit))
	})

	t.Run("Both enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", strconv.Itoa(invalidHTTPRequestFragment))
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", strconv.Itoa(invalidHTTPRequestFragment+1))
		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(driverMaxFragmentLimit))
	})

	t.Run("Not enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		// Default value.
		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(driverMaxFragmentLimit))
	})
}

// ========================================
// HTTP2 Protocol Configuration Tests
// ========================================

func TestEnableHTTP2Monitoring(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http2.enabled", true)
		cfg := New()

		// HTTP2 may be disabled by adjust_usm.go on kernels < 5.2
		// We test that the config respects the kernel limitation
		assert.Equal(t, sysconfig.HTTP2MonitoringSupported(), cfg.EnableHTTP2Monitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP2_MONITORING", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		// HTTP2 may be disabled by adjust_usm.go on kernels < 5.2
		// We test that the config respects the kernel limitation
		assert.Equal(t, sysconfig.HTTP2MonitoringSupported(), cfg.EnableHTTP2Monitoring)
	})
}

func TestDefaultDisabledHTTP2Support(t *testing.T) {
	mock.NewSystemProbe(t)
	cfg := New()

	_, err := sysconfig.New("", "")
	require.NoError(t, err)

	assert.False(t, cfg.EnableHTTP2Monitoring)
}

func TestHTTP2DynamicTableMapCleanerInterval(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http2.dynamic_table_map_cleaner_interval_seconds", 1025)
		cfg := New()

		require.Equal(t, cfg.HTTP2DynamicTableMapCleanerInterval, 1025*time.Second)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP2_DYNAMIC_TABLE_MAP_CLEANER_INTERVAL_SECONDS", "1025")
		cfg := New()

		require.Equal(t, cfg.HTTP2DynamicTableMapCleanerInterval, 1025*time.Second)
	})

	t.Run("Not enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		// Default value.
		require.Equal(t, cfg.HTTP2DynamicTableMapCleanerInterval, 30*time.Second)
	})
}

// ========================================
// Kafka Protocol Configuration Tests
// ========================================

func TestEnableKafkaMonitoring(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.kafka.enabled", true)
		cfg := New()

		assert.True(t, cfg.EnableKafkaMonitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_KAFKA_ENABLED", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.True(t, cfg.EnableKafkaMonitoring)
	})

	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_kafka_monitoring", true)
		cfg := New()

		assert.True(t, cfg.EnableKafkaMonitoring)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_KAFKA_MONITORING", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.True(t, cfg.EnableKafkaMonitoring)
	})

	t.Run("both enabled - new config takes precedence", func(t *testing.T) {
		// Set both old and new config keys via ENV
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_KAFKA_MONITORING", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_KAFKA_ENABLED", "false")
		mock.NewSystemProbe(t)
		cfg := New()

		// New config should take precedence
		assert.False(t, cfg.EnableKafkaMonitoring)
	})

	t.Run("default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.False(t, cfg.EnableKafkaMonitoring)
	})
}

func TestMaxKafkaStatsBuffered(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.kafka.max_stats_buffered", 30000)
		cfg := New()

		assert.Equal(t, 30000, cfg.MaxKafkaStatsBuffered)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_KAFKA_MAX_STATS_BUFFERED", "50000")
		cfg := New()

		assert.Equal(t, 50000, cfg.MaxKafkaStatsBuffered)
	})

	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.max_kafka_stats_buffered", 30000)
		cfg := New()

		assert.Equal(t, 30000, cfg.MaxKafkaStatsBuffered)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_KAFKA_STATS_BUFFERED", "50000")
		cfg := New()

		assert.Equal(t, 50000, cfg.MaxKafkaStatsBuffered)
	})

	t.Run("both enabled - new config takes precedence", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		// Set both old and new config keys
		mockSystemProbe.SetWithoutSource("service_monitoring_config.max_kafka_stats_buffered", 40000)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.kafka.max_stats_buffered", 50000)
		cfg := New()

		// New config should take precedence
		assert.Equal(t, 50000, cfg.MaxKafkaStatsBuffered)
	})

	t.Run("default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Equal(t, 100000, cfg.MaxKafkaStatsBuffered)
	})
}

// ========================================
// Postgres Protocol Configuration Tests
// ========================================

func TestEnablePostgresMonitoring(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.postgres.enabled", true)
		cfg := New()

		assert.True(t, cfg.EnablePostgresMonitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_POSTGRES_ENABLED", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.True(t, cfg.EnablePostgresMonitoring)
	})

	t.Run("default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.False(t, cfg.EnablePostgresMonitoring)
	})
}

func TestMaxPostgresTelemetryBuffered(t *testing.T) {
	t.Run("value set through env var", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_POSTGRES_MAX_TELEMETRY_BUFFER", "50000")

		cfg := New()
		assert.Equal(t, 50000, cfg.MaxPostgresTelemetryBuffer)
	})

	t.Run("value set through yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.postgres.max_telemetry_buffer", 30000)

		cfg := New()
		assert.Equal(t, 30000, cfg.MaxPostgresTelemetryBuffer)
	})
}

func TestMaxPostgresStatsBuffered(t *testing.T) {
	t.Run("value set through env var", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_POSTGRES_MAX_STATS_BUFFERED", "50000")
		cfg := New()

		assert.Equal(t, 50000, cfg.MaxPostgresStatsBuffered)
	})

	t.Run("value set through yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.postgres.max_stats_buffered", 30000)
		cfg := New()

		assert.Equal(t, 30000, cfg.MaxPostgresStatsBuffered)
	})

	t.Run("default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Equal(t, 100000, cfg.MaxPostgresStatsBuffered)
	})
}

// ========================================
// Redis Protocol Configuration Tests
// ========================================

func TestEnableRedisMonitoring(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.redis.enabled", true)
		cfg := New()

		// Redis may be disabled by adjust_usm.go on kernels < 5.4
		// We test that the config respects the kernel limitation
		assert.Equal(t, sysconfig.RedisMonitoringSupported(), cfg.EnableRedisMonitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_REDIS_ENABLED", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		// Redis may be disabled by adjust_usm.go on kernels < 5.4
		// We test that the config respects the kernel limitation
		assert.Equal(t, sysconfig.RedisMonitoringSupported(), cfg.EnableRedisMonitoring)
	})

	t.Run("default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.False(t, cfg.EnableRedisMonitoring)
	})
}

func TestRedisTrackResources(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.redis.track_resources", true)
		cfg := New()

		assert.True(t, cfg.RedisTrackResources)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_REDIS_TRACK_RESOURCES", "true")
		cfg := New()

		assert.True(t, cfg.RedisTrackResources)
	})

	t.Run("default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.False(t, cfg.RedisTrackResources)
	})
}

func TestMaxRedisStatsBuffered(t *testing.T) {
	t.Run("value set through env var", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_REDIS_MAX_STATS_BUFFERED", "50000")
		cfg := New()

		assert.Equal(t, 50000, cfg.MaxRedisStatsBuffered)
	})

	t.Run("value set through yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.redis.max_stats_buffered", 30000)
		cfg := New()

		assert.Equal(t, 30000, cfg.MaxRedisStatsBuffered)
	})

	t.Run("default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Equal(t, 100000, cfg.MaxRedisStatsBuffered)
	})
}

// ========================================
// Native TLS Configuration Tests
// ========================================

func TestUSMTLSNativeEnabled(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.enable_https_monitoring", false)
		cfg := New()

		assert.False(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING", "false")
		cfg := New()

		assert.False(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.tls.native.enabled", false)
		cfg := New()

		assert.False(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_NATIVE_ENABLED", "false")
		cfg := New()

		assert.False(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_NATIVE_ENABLED", "false")
		cfg := New()

		assert.False(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING", "false")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_NATIVE_ENABLED", "true")
		cfg := New()

		assert.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("Both enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_NATIVE_ENABLED", "true")
		cfg := New()

		assert.True(t, cfg.EnableNativeTLSMonitoring)
	})
}

// ========================================
// Go TLS Configuration Tests
// ========================================

func TestUSMTLSGoEnabled(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_go_tls_support", false)
		cfg := New()

		require.False(t, cfg.EnableGoTLSSupport)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_GO_TLS_SUPPORT", "false")
		cfg := New()

		require.False(t, cfg.EnableGoTLSSupport)
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.tls.go.enabled", false)
		cfg := New()

		require.False(t, cfg.EnableGoTLSSupport)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_GO_ENABLED", "false")
		cfg := New()

		require.False(t, cfg.EnableGoTLSSupport)
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_GO_ENABLED", "false")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_GO_TLS_SUPPORT", "true")
		cfg := New()

		require.False(t, cfg.EnableGoTLSSupport)
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_GO_TLS_SUPPORT", "false")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_GO_ENABLED", "true")
		cfg := New()

		require.True(t, cfg.EnableGoTLSSupport)
	})

	t.Run("Both disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_GO_TLS_SUPPORT", "false")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_GO_ENABLED", "false")
		cfg := New()

		require.False(t, cfg.EnableGoTLSSupport)
	})

	t.Run("Deprecated is disabled takes precedence over default", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_go_tls_support", false)
		cfg := New()

		require.False(t, cfg.EnableGoTLSSupport)
	})

	t.Run("Enabled by default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		require.True(t, cfg.EnableGoTLSSupport)
	})
}

func TestUSMTLSGoExcludeSelf(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.tls.go.exclude_self", false)
		cfg := New()

		require.False(t, cfg.GoTLSExcludeSelf)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_GO_EXCLUDE_SELF", "false")
		cfg := New()

		require.False(t, cfg.GoTLSExcludeSelf)
	})

	t.Run("Not disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		// Default value.
		require.True(t, cfg.GoTLSExcludeSelf)
	})
}

// ========================================
// NodeJS TLS Configuration Tests
// ========================================

func TestNodeJSMonitoring(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.False(t, cfg.EnableNodeJSMonitoring)
	})

	t.Run("via yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.tls.nodejs.enabled", true)
		cfg := New()

		assert.True(t, cfg.EnableNodeJSMonitoring)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_NODEJS_ENABLED", "true")
		cfg := New()

		assert.True(t, cfg.EnableNodeJSMonitoring)
	})
}

// ========================================
// Istio Service Mesh TLS Configuration Tests
// ========================================

func TestIstioMonitoring(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.True(t, cfg.EnableIstioMonitoring)
	})

	t.Run("via yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.tls.istio.enabled", false)
		cfg := New()

		assert.False(t, cfg.EnableIstioMonitoring)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_ISTIO_ENABLED", "false")
		cfg := New()

		assert.False(t, cfg.EnableIstioMonitoring)
	})
}

func TestEnvoyPathConfig(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.EqualValues(t, cfg.EnvoyPath, "/bin/envoy")
	})

	t.Run("via yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.tls.istio.envoy_path", "/test/envoy")
		cfg := New()

		assert.EqualValues(t, "/test/envoy", cfg.EnvoyPath)
	})

	t.Run("value set through env var", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_ISTIO_ENVOY_PATH", "/test/envoy")
		cfg := New()

		assert.EqualValues(t, "/test/envoy", cfg.EnvoyPath)
	})
}

func TestHTTP2ConfigMigration(t *testing.T) {
	t.Run("new tree structure config", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http2.enabled", true)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http2.dynamic_table_map_cleaner_interval_seconds", 45)
		cfg := New()

		// HTTP2 may be disabled by adjust_usm.go on kernels < 5.2
		// We test that the config respects the kernel limitation
		assert.Equal(t, sysconfig.HTTP2MonitoringSupported(), cfg.EnableHTTP2Monitoring)
		assert.Equal(t, 45*time.Second, cfg.HTTP2DynamicTableMapCleanerInterval)
	})

	t.Run("backward compatibility with old flat keys", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_http2_monitoring", true)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http2_dynamic_table_map_cleaner_interval_seconds", 60)
		cfg := New()

		// HTTP2 may be disabled by adjust_usm.go on kernels < 5.2
		// We test that the config respects the kernel limitation
		assert.Equal(t, sysconfig.HTTP2MonitoringSupported(), cfg.EnableHTTP2Monitoring)
		assert.Equal(t, 60*time.Second, cfg.HTTP2DynamicTableMapCleanerInterval)
	})

	t.Run("new tree structure takes precedence", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		// Set both old and new
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_http2_monitoring", false)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http2.enabled", true)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http2_dynamic_table_map_cleaner_interval_seconds", 30)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http2.dynamic_table_map_cleaner_interval_seconds", 90)
		cfg := New()

		// HTTP2 may be disabled by adjust_usm.go on kernels < 5.2
		// We test that the config respects the kernel limitation (new tree structure wins)
		assert.Equal(t, sysconfig.HTTP2MonitoringSupported(), cfg.EnableHTTP2Monitoring)
		assert.Equal(t, 90*time.Second, cfg.HTTP2DynamicTableMapCleanerInterval) // new tree structure wins
	})

	t.Run("environment variables work", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP2_ENABLED", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP2_DYNAMIC_TABLE_MAP_CLEANER_INTERVAL_SECONDS", "120")
		cfg := New()

		// HTTP2 may be disabled by adjust_usm.go on kernels < 5.2
		// We test that the config respects the kernel limitation
		assert.Equal(t, sysconfig.HTTP2MonitoringSupported(), cfg.EnableHTTP2Monitoring)
		assert.Equal(t, 120*time.Second, cfg.HTTP2DynamicTableMapCleanerInterval)
	})

	t.Run("defaults work correctly", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.False(t, cfg.EnableHTTP2Monitoring)
		assert.Equal(t, 30*time.Second, cfg.HTTP2DynamicTableMapCleanerInterval)
	})
}

func TestHTTPConfigMigration(t *testing.T) {
	t.Run("new tree structure config", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.enabled", false)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.max_stats_buffered", 50000)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.max_tracked_connections", 2048)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.notification_threshold", 256)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.max_request_fragment", 256)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.map_cleaner_interval_seconds", 600)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.idle_connection_ttl_seconds", 60)
		cfg := New()

		assert.False(t, cfg.EnableHTTPMonitoring)
		assert.Equal(t, 50000, cfg.MaxHTTPStatsBuffered)
		assert.Equal(t, int64(2048), cfg.MaxTrackedHTTPConnections)
		assert.Equal(t, int64(256), cfg.HTTPNotificationThreshold)
		assert.Equal(t, int64(256), cfg.HTTPMaxRequestFragment)
		assert.Equal(t, 600*time.Second, cfg.HTTPMapCleanerInterval)
		assert.Equal(t, 60*time.Second, cfg.HTTPIdleConnectionTTL)
	})

	t.Run("backward compatibility with old flat keys", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_http_monitoring", false)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.max_http_stats_buffered", 75000)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.max_tracked_http_connections", 4096)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_notification_threshold", 128)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_max_request_fragment", 128)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_map_cleaner_interval_in_s", 900)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_idle_connection_ttl_in_s", 90)
		cfg := New()

		assert.False(t, cfg.EnableHTTPMonitoring)
		assert.Equal(t, 75000, cfg.MaxHTTPStatsBuffered)
		assert.Equal(t, int64(4096), cfg.MaxTrackedHTTPConnections)
		assert.Equal(t, int64(128), cfg.HTTPNotificationThreshold)
		assert.Equal(t, int64(128), cfg.HTTPMaxRequestFragment)
		assert.Equal(t, 900*time.Second, cfg.HTTPMapCleanerInterval)
		assert.Equal(t, 90*time.Second, cfg.HTTPIdleConnectionTTL)
	})

	t.Run("new tree structure takes precedence", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		// Set both old and new
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_http_monitoring", true)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.enabled", false)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.max_http_stats_buffered", 75000)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.max_stats_buffered", 50000)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.max_tracked_http_connections", 4096)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.max_tracked_connections", 2048)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_notification_threshold", 128)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.notification_threshold", 256)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_max_request_fragment", 128)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.max_request_fragment", 256)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_map_cleaner_interval_in_s", 900)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.map_cleaner_interval_seconds", 600)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http_idle_connection_ttl_in_s", 90)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.idle_connection_ttl_seconds", 60)
		cfg := New()

		assert.False(t, cfg.EnableHTTPMonitoring)                    // new tree structure wins
		assert.Equal(t, 50000, cfg.MaxHTTPStatsBuffered)             // new tree structure wins
		assert.Equal(t, int64(2048), cfg.MaxTrackedHTTPConnections)  // new tree structure wins
		assert.Equal(t, int64(256), cfg.HTTPNotificationThreshold)   // new tree structure wins
		assert.Equal(t, int64(256), cfg.HTTPMaxRequestFragment)      // new tree structure wins
		assert.Equal(t, 600*time.Second, cfg.HTTPMapCleanerInterval) // new tree structure wins
		assert.Equal(t, 60*time.Second, cfg.HTTPIdleConnectionTTL)   // new tree structure wins
	})

	t.Run("environment variables work", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_ENABLED", "false")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_STATS_BUFFERED", "80000")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_TRACKED_CONNECTIONS", "8192")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_NOTIFICATION_THRESHOLD", "512")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "384")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAP_CLEANER_INTERVAL_SECONDS", "1200")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_IDLE_CONNECTION_TTL_SECONDS", "120")
		cfg := New()

		assert.False(t, cfg.EnableHTTPMonitoring)
		assert.Equal(t, 80000, cfg.MaxHTTPStatsBuffered)
		assert.Equal(t, int64(8192), cfg.MaxTrackedHTTPConnections)
		assert.Equal(t, int64(512), cfg.HTTPNotificationThreshold)
		assert.Equal(t, int64(384), cfg.HTTPMaxRequestFragment)
		assert.Equal(t, 1200*time.Second, cfg.HTTPMapCleanerInterval)
		assert.Equal(t, 120*time.Second, cfg.HTTPIdleConnectionTTL)
	})

	t.Run("defaults work correctly", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.True(t, cfg.EnableHTTPMonitoring)
		assert.Equal(t, 100000, cfg.MaxHTTPStatsBuffered)
		assert.Equal(t, int64(1024), cfg.MaxTrackedHTTPConnections)
		assert.Equal(t, int64(512), cfg.HTTPNotificationThreshold)
		assert.Equal(t, int64(512), cfg.HTTPMaxRequestFragment)
		assert.Equal(t, 300*time.Second, cfg.HTTPMapCleanerInterval)
		assert.Equal(t, 30*time.Second, cfg.HTTPIdleConnectionTTL)
	})
}

func TestHTTPUseDirectConsumer(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.False(t, cfg.HTTPUseDirectConsumer)
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http.use_direct_consumer", true)
		cfg := New()

		assert.True(t, cfg.HTTPUseDirectConsumer)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_USE_DIRECT_CONSUMER", "true")
		cfg := New()

		assert.True(t, cfg.HTTPUseDirectConsumer)
	})
}
