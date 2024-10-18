// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package config

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

// variables for testing config options
const (
	driverDefaultNotificationThreshold = 512
	driverMaxFragmentLimit             = 512
	validNotificationThreshold         = 100
	invalidNotificationThreshold       = 1200
	invalidHTTPRequestFragment         = 600
)

func TestDisablingDNSInspection(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("system_probe_config.enabled", true)
		mockSystemProbe.SetWithoutSource("system_probe_config.disable_dns_inspection", true)
		cfg := New()

		assert.False(t, cfg.DNSInspection)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_DISABLE_DNS_INSPECTION", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.False(t, cfg.DNSInspection)
	})
}

func TestDisablingProtocolClassification(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.enable_protocol_classification", false)
		cfg := New()

		assert.False(t, cfg.ProtocolClassificationEnabled)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_ENABLE_PROTOCOL_CLASSIFICATION", "false")
		cfg := New()
		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.False(t, cfg.ProtocolClassificationEnabled)
	})
}

func TestEnableHTTPMonitoring(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.enable_http_monitoring", true)
		cfg := New()

		assert.True(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.True(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_http_monitoring", true)
		cfg := New()

		assert.True(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP_MONITORING", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.True(t, cfg.EnableHTTPMonitoring)
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

	t.Run("Not enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()
		assert.False(t, cfg.EnableHTTPMonitoring)
	})
}

func TestEnableHTTP2Monitoring(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_http2_monitoring", true)
		cfg := New()

		assert.True(t, cfg.EnableHTTP2Monitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP2_MONITORING", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.True(t, cfg.EnableHTTP2Monitoring)
	})
}

func TestEnableKafkaMonitoring(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_kafka_monitoring", true)
		cfg := New()

		assert.True(t, cfg.EnableKafkaMonitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_KAFKA_MONITORING", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.True(t, cfg.EnableKafkaMonitoring)
	})
}

func TestEnablePostgresMonitoring(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_postgres_monitoring", true)
		cfg := New()

		assert.True(t, cfg.EnablePostgresMonitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_POSTGRES_MONITORING", "true")
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

func TestEnableRedisMonitoring(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_redis_monitoring", true)
		cfg := New()

		assert.True(t, cfg.EnableRedisMonitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_REDIS_MONITORING", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.True(t, cfg.EnableRedisMonitoring)
	})

	t.Run("default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.False(t, cfg.EnableRedisMonitoring)
	})
}

func TestDefaultDisabledHTTP2Support(t *testing.T) {
	mock.NewSystemProbe(t)
	cfg := New()

	_, err := sysconfig.New("", "")
	require.NoError(t, err)

	assert.False(t, cfg.EnableHTTP2Monitoring)
}

func TestDisableGatewayLookup(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		cfg := New()
		// default config
		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.True(t, cfg.EnableGatewayLookup)

		mockSystemProbe.SetWithoutSource("network_config.enable_gateway_lookup", false)
		cfg = New()

		assert.False(t, cfg.EnableGatewayLookup)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_GATEWAY_LOOKUP", "false")
		cfg := New()
		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.False(t, cfg.EnableGatewayLookup)
	})
}

func TestIgnoreConntrackInitFailure(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.ignore_conntrack_init_failure", true)
		cfg := New()

		assert.True(t, cfg.IgnoreConntrackInitFailure)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_IGNORE_CONNTRACK_INIT_FAILURE", "true")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.Nil(t, err)
		assert.True(t, cfg.IgnoreConntrackInitFailure)
	})
}

func TestEnablingDNSStatsCollection(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("system_probe_config.collect_dns_stats", true)
		cfg := New()

		assert.True(t, cfg.CollectDNSStats)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_COLLECT_DNS_STATS", "false")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.False(t, cfg.CollectDNSStats)

		t.Setenv("DD_COLLECT_DNS_STATS", "true")
		_, err = sysconfig.New("", "")
		require.NoError(t, err)
		cfg = New()
		assert.True(t, cfg.CollectDNSStats)
	})
}

func TestDisablingDNSDomainCollection(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("system_probe_config.collect_dns_domains", false)
		cfg := New()

		mockSystemProbe.SetWithoutSource("system_probe_config.max_dns_stats", 100)

		assert.False(t, cfg.CollectDNSDomains)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_COLLECT_DNS_DOMAINS", "false")
		cfg := New()

		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.False(t, cfg.CollectDNSDomains)

		mock.NewSystemProbe(t)
		t.Setenv("DD_COLLECT_DNS_DOMAINS", "true")
		_, err = sysconfig.New("", "")
		require.NoError(t, err)
		cfg = New()

		assert.True(t, cfg.CollectDNSDomains)
	})
}

func TestSettingMaxDNSStats(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("system_probe_config.collect_dns_domains", false)
		mockSystemProbe.SetWithoutSource("system_probe_config.max_dns_stats", 100)
		cfg := New()

		assert.Equal(t, 100, cfg.MaxDNSStats)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()
		os.Unsetenv("DD_SYSTEM_PROBE_CONFIG_MAX_DNS_STATS")
		_, err := sysconfig.New("", "")
		require.NoError(t, err)

		assert.Equal(t, 20000, cfg.MaxDNSStats) // default value

		t.Setenv("DD_SYSTEM_PROBE_CONFIG_MAX_DNS_STATS", "10000")
		_, err = sysconfig.New("", "")
		require.NoError(t, err)
		cfg = New()

		assert.Equal(t, 10000, cfg.MaxDNSStats)
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

	t.Run("via ENV variable", func(t *testing.T) {
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

func TestHTTP2DynamicTableMapCleanerInterval(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.http2_dynamic_table_map_cleaner_interval_seconds", 1025)
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

func TestMaxClosedConnectionsBuffered(t *testing.T) {
	maxTrackedConnections := New().MaxTrackedConnections

	t.Run("value set", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_MAX_CLOSED_CONNECTIONS_BUFFERED", fmt.Sprintf("%d", maxTrackedConnections-1))
		cfg := New()

		require.Equal(t, maxTrackedConnections-1, cfg.MaxClosedConnectionsBuffered)
	})

	t.Run("value not set", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		require.Equal(t, cfg.MaxTrackedConnections, cfg.MaxClosedConnectionsBuffered)
	})
}

func TestMaxFailedConnectionsBuffered(t *testing.T) {
	maxTrackedConnections := New().MaxTrackedConnections

	t.Run("value set", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_MAX_FAILED_CONNECTIONS_BUFFERED", fmt.Sprintf("%d", maxTrackedConnections-1))
		cfg := New()

		require.Equal(t, maxTrackedConnections-1, cfg.MaxFailedConnectionsBuffered)
	})

	t.Run("value not set", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		require.Equal(t, cfg.MaxTrackedConnections, cfg.MaxFailedConnectionsBuffered)
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

func TestMaxKafkaStatsBuffered(t *testing.T) {
	t.Run("value set through env var", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_KAFKA_STATS_BUFFERED", "50000")
		cfg := New()

		assert.Equal(t, 50000, cfg.MaxKafkaStatsBuffered)
	})

	t.Run("value set through yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.max_kafka_stats_buffered", 30000)
		cfg := New()

		assert.Equal(t, 30000, cfg.MaxKafkaStatsBuffered)
	})
}

func TestMaxPostgresTelemetryBuffered(t *testing.T) {
	t.Run("value set through env var", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_POSTGRES_TELEMETRY_BUFFER", "50000")

		cfg := New()
		assert.Equal(t, 50000, cfg.MaxPostgresTelemetryBuffer)
	})

	t.Run("value set through yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.max_postgres_telemetry_buffer", 30000)

		cfg := New()
		assert.Equal(t, 30000, cfg.MaxPostgresTelemetryBuffer)
	})
}

func TestMaxPostgresStatsBuffered(t *testing.T) {
	t.Run("value set through env var", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_POSTGRES_STATS_BUFFERED", "50000")
		cfg := New()

		assert.Equal(t, 50000, cfg.MaxPostgresStatsBuffered)
	})

	t.Run("value set through yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.max_postgres_stats_buffered", 30000)
		cfg := New()

		assert.Equal(t, 30000, cfg.MaxPostgresStatsBuffered)
	})

	t.Run("default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Equal(t, 100000, cfg.MaxPostgresStatsBuffered)
	})
}

func TestMaxRedisStatsBuffered(t *testing.T) {
	t.Run("value set through env var", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_REDIS_STATS_BUFFERED", "50000")
		cfg := New()

		assert.Equal(t, 50000, cfg.MaxRedisStatsBuffered)
	})

	t.Run("value set through yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.max_redis_stats_buffered", 30000)
		cfg := New()

		assert.Equal(t, 30000, cfg.MaxRedisStatsBuffered)
	})

	t.Run("default", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Equal(t, 100000, cfg.MaxRedisStatsBuffered)
	})
}

func TestNetworkConfigEnabled(t *testing.T) {
	ys := true

	for i, tc := range []struct {
		sysIn, npmIn, usmIn    *bool
		npmEnabled, usmEnabled bool
	}{
		{sysIn: nil, npmIn: nil, usmIn: nil, npmEnabled: false, usmEnabled: false},
		{sysIn: nil, npmIn: nil, usmIn: &ys, npmEnabled: false, usmEnabled: true},
		{sysIn: nil, npmIn: &ys, usmIn: nil, npmEnabled: true, usmEnabled: false},
		{sysIn: nil, npmIn: &ys, usmIn: &ys, npmEnabled: true, usmEnabled: true},
		{sysIn: &ys, npmIn: nil, usmIn: nil, npmEnabled: true, usmEnabled: false},
		// only set NPM enabled flag is sysprobe enabled and !USM
		{sysIn: &ys, npmIn: nil, usmIn: &ys, npmEnabled: false, usmEnabled: true},
		{sysIn: &ys, npmIn: &ys, usmIn: nil, npmEnabled: true, usmEnabled: false},
		{sysIn: &ys, npmIn: &ys, usmIn: &ys, npmEnabled: true, usmEnabled: true},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "emptyconfig*.yaml")
			require.NoError(t, err)
			t.Cleanup(func() { f.Close() })

			if tc.sysIn != nil {
				t.Setenv("DD_SYSTEM_PROBE_ENABLED", strconv.FormatBool(*tc.sysIn))
			}
			if tc.npmIn != nil {
				t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLED", strconv.FormatBool(*tc.npmIn))
			}
			if tc.usmIn != nil {
				t.Setenv("DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED", strconv.FormatBool(*tc.usmIn))
			}

			mock.NewSystemProbe(t)
			cfg := New()
			_, err = sysconfig.New(f.Name(), "")
			require.NoError(t, err)

			assert.Equal(t, tc.npmEnabled, cfg.NPMEnabled, "npm state")
			assert.Equal(t, tc.usmEnabled, cfg.ServiceMonitoringEnabled, "usm state")
		})
	}
}

func TestIstioMonitoring(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.False(t, cfg.EnableIstioMonitoring)
	})

	t.Run("via yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.tls.istio.enabled", true)
		cfg := New()

		assert.True(t, cfg.EnableIstioMonitoring)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_ISTIO_ENABLED", "true")
		cfg := New()

		assert.True(t, cfg.EnableIstioMonitoring)
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

func TestUSMEventStream(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.False(t, cfg.EnableUSMEventStream)
	})

	t.Run("via yaml", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_event_stream", true)
		cfg := New()

		assert.True(t, cfg.EnableUSMEventStream)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_EVENT_STREAM", "true")
		cfg := New()

		assert.True(t, cfg.EnableUSMEventStream)
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

func TestUSMTLSNativeEnabled(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.enable_https_monitoring", true)
		cfg := New()

		require.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING", "true")
		cfg := New()

		require.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.tls.native.enabled", true)
		cfg := New()

		require.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_NATIVE_ENABLED", "true")
		cfg := New()

		require.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_NATIVE_ENABLED", "false")
		cfg := New()

		require.False(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING", "false")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_NATIVE_ENABLED", "true")
		cfg := New()

		require.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("Both enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_NATIVE_ENABLED", "true")
		cfg := New()

		require.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("Not enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		// Default value.
		require.False(t, cfg.EnableNativeTLSMonitoring)
	})
}

func TestUSMTLSGoEnabled(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enable_go_tls_support", true)
		cfg := New()

		require.True(t, cfg.EnableGoTLSSupport)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_GO_TLS_SUPPORT", "true")
		cfg := New()

		require.True(t, cfg.EnableGoTLSSupport)
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.tls.go.enabled", true)
		cfg := New()

		require.True(t, cfg.EnableGoTLSSupport)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_GO_ENABLED", "true")
		cfg := New()

		require.True(t, cfg.EnableGoTLSSupport)
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

	t.Run("Both enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_GO_TLS_SUPPORT", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_GO_ENABLED", "true")
		cfg := New()

		require.True(t, cfg.EnableGoTLSSupport)
	})

	t.Run("Not enabled", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		// Default value.
		require.False(t, cfg.EnableGoTLSSupport)
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

func TestProcessServiceInference(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enabled", true)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.process_service_inference.enabled", true)
		New()

		require.True(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.enabled"))
	})
	t.Run("via ENV variable", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLED", "true")
		t.Setenv("DD_SYSTEM_PROBE_PROCESS_SERVICE_INFERENCE_ENABLED", "true")
		New()
		sysconfig.Adjust(mockSystemProbe)

		require.True(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.enabled"))
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.enabled", true)
		mockSystemProbe.SetWithoutSource("system_probe_config.process_service_inference.enabled", true)
		New()

		require.True(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.enabled"))
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enabled", true)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.process_service_inference.enabled", true)
		mockSystemProbe.SetWithoutSource("system_probe_config.process_service_inference.enabled", false)
		New()

		require.False(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.enabled"))
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enabled", true)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.process_service_inference.enabled", false)
		mockSystemProbe.SetWithoutSource("system_probe_config.process_service_inference.enabled", true)

		New()

		require.True(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.enabled"))
	})

	t.Run("Both enabled", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enabled", true)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.process_service_inference.enabled", true)
		mockSystemProbe.SetWithoutSource("system_probe_config.process_service_inference.enabled", true)

		New()

		require.True(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.enabled"))
	})

	t.Run("Not enabled", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		New()
		require.False(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.enabled"))
	})

	t.Run("Enabled without net, dsm, sm enabled", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("system_probe_config.process_service_inference.enabled", true)
		New()
		require.False(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.enabled"))
	})

	t.Run("test platform specific defaults", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		// usm or npm must be enabled for the process_service_inference to be enabled
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enabled", true)
		New()
		sysconfig.Adjust(mockSystemProbe)

		var expected bool
		if runtime.GOOS == "windows" {
			expected = true
		} else {
			expected = false
		}

		require.Equal(t, expected, mockSystemProbe.GetBool("system_probe_config.process_service_inference.enabled"))
	})
}

func TestProcessServiceInferenceWindows(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enabled", true)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.process_service_inference.use_windows_service_name", true)
		New()

		require.True(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.use_windows_service_name"))
	})
	t.Run("via ENV variable", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLED", "true")
		t.Setenv("DD_SYSTEM_PROBE_PROCESS_SERVICE_INFERENCE_USE_WINDOWS_SERVICE_NAME", "true")
		sysconfig.Adjust(mockSystemProbe)
		New()

		require.True(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.use_windows_service_name"))
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enabled", true)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.process_service_inference.use_windows_service_name", true)
		New()

		require.True(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.use_windows_service_name"))
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)

		mockSystemProbe.SetWithoutSource("service_monitoring_config.enabled", true)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.process_service_inference.use_windows_service_name", true)
		mockSystemProbe.SetWithoutSource("system_probe_config.process_service_inference.use_windows_service_name", false)
		New()

		require.False(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.use_windows_service_name"))
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enabled", true)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.process_service_inference.use_windows_service_name", false)
		mockSystemProbe.SetWithoutSource("system_probe_config.process_service_inference.use_windows_service_name", true)
		New()

		require.True(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.use_windows_service_name"))
	})

	t.Run("Both enabled", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.enabled", true)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.process_service_inference.use_windows_service_name", true)
		mockSystemProbe.SetWithoutSource("system_probe_config.process_service_inference.use_windows_service_name", true)

		require.True(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.use_windows_service_name"))
	})

	t.Run("Not enabled", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("service_monitoring_config.process_service_inference.use_windows_service_name", false)

		require.False(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.use_windows_service_name"))
	})
}
