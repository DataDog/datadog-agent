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
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	aconfig "github.com/DataDog/datadog-agent/pkg/config"
)

func TestDisablingDNSInspection(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-DisableDNS.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.DNSInspection)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_DISABLE_DNS_INSPECTION", "true")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.DNSInspection)
	})
}

func TestDisablingProtocolClassification(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-NoPRTCLClassifying.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.ProtocolClassificationEnabled)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_ENABLE_PROTOCOL_CLASSIFICATION", "false")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.ProtocolClassificationEnabled)
	})
}

func TestEnableHTTPStatsByStatusCode(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDSystemProbeConfig-EnableHTTPStatusCodeAggr.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPStatsByStatusCode)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP_STATS_BY_STATUS_CODE", "true")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPStatsByStatusCode)
	})
}

func TestEnableHTTPMonitoring(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-DeprecatedEnableHTTP.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING", "true")

		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableHTTP.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP_MONITORING", "true")

		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP_MONITORING", "false")

		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING", "false")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP_MONITORING", "true")

		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("Both enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP_MONITORING", "true")

		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("Not enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()

		assert.False(t, cfg.EnableHTTPMonitoring)
	})
}

func TestEnableDataStreams(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableDataStreams.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.DataStreamsEnabled)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_DATA_STREAMS_ENABLED", "true")

		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.DataStreamsEnabled)
	})
}

func TestEnableJavaTLSSupport(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
service_monitoring_config:
  tls:
    java:
      enabled: true
`)
		require.True(t, cfg.EnableJavaTLSSupport)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_JAVA_ENABLED", "true")

		cfg := New()

		require.True(t, cfg.EnableJavaTLSSupport)
	})

}

func TestEnableHTTP2Monitoring(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableHTTP2.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTP2Monitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP2_MONITORING", "true")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTP2Monitoring)
	})
}

func TestDefaultDisabledJavaTLSSupport(t *testing.T) {
	aconfig.ResetSystemProbeConfig(t)

	_, err := sysconfig.New("")
	require.NoError(t, err)
	cfg := New()

	assert.False(t, cfg.EnableJavaTLSSupport)
}

func TestDefaultDisabledHTTP2Support(t *testing.T) {
	aconfig.ResetSystemProbeConfig(t)

	_, err := sysconfig.New("")
	require.NoError(t, err)
	cfg := New()

	assert.False(t, cfg.EnableHTTP2Monitoring)
}

func TestDisableGatewayLookup(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		// default config
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableGatewayLookup)

		aconfig.ResetSystemProbeConfig(t)
		_, err = sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-DisableGwLookup.yaml")
		require.NoError(t, err)
		cfg = New()

		assert.False(t, cfg.EnableGatewayLookup)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_GATEWAY_LOOKUP", "false")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.EnableGatewayLookup)
	})
}

func TestIgnoreConntrackInitFailure(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-IgnoreCTInitFailure.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.IgnoreConntrackInitFailure)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_IGNORE_CONNTRACK_INIT_FAILURE", "true")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.Nil(t, err)
		assert.True(t, cfg.IgnoreConntrackInitFailure)
	})
}

func TestEnablingDNSStatsCollection(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableDNSStats.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.CollectDNSStats)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_COLLECT_DNS_STATS", "false")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.CollectDNSStats)

		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_COLLECT_DNS_STATS", "true")
		_, err = sysconfig.New("")
		require.NoError(t, err)
		cfg = New()

		assert.True(t, cfg.CollectDNSStats)
	})
}

func TestDisablingDNSDomainCollection(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-DisableDNSDomains.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.CollectDNSDomains)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_COLLECT_DNS_DOMAINS", "false")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.CollectDNSDomains)

		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_COLLECT_DNS_DOMAINS", "true")
		_, err = sysconfig.New("")
		require.NoError(t, err)
		cfg = New()

		assert.True(t, cfg.CollectDNSDomains)
	})
}

func TestSettingMaxDNSStats(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-DisableDNSDomains.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.Equal(t, 100, cfg.MaxDNSStats)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		os.Unsetenv("DD_SYSTEM_PROBE_CONFIG_MAX_DNS_STATS")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.Equal(t, 20000, cfg.MaxDNSStats) // default value

		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_MAX_DNS_STATS", "10000")
		_, err = sysconfig.New("")
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
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDSystemProbeConfig-HTTPReplaceRulesDeprecated.yaml")
		require.NoError(t, err)
		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_HTTP_REPLACE_RULES", envContent)

		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDSystemProbeConfig-HTTPReplaceRules.yaml")
		require.NoError(t, err)
		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_REPLACE_RULES", envContent)

		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_HTTP_REPLACE_RULES", envContent)

		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_REPLACE_RULES", envContent)

		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("Both enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_REPLACE_RULES", envContent)
		// Setting a different value for the old value, as we should override.
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_HTTP_REPLACE_RULES", `
        [
          {
            "pattern": "payment_id"
          }
        ]
        `)

		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("Not enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()

		assert.Empty(t, cfg.HTTPReplaceRules)
	})
}

func TestMaxTrackedHTTPConnections(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDSystemProbeConfig-MaxTrackedHTTPConnectionsDeprecated.yaml")
		require.NoError(t, err)
		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_NETWORK_CONFIG_MAX_TRACKED_HTTP_CONNECTIONS", "1025")

		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDSystemProbeConfig-MaxTrackedHTTPConnections.yaml")
		require.NoError(t, err)
		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_TRACKED_HTTP_CONNECTIONS", "1025")

		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_NETWORK_CONFIG_MAX_TRACKED_HTTP_CONNECTIONS", "1025")

		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_TRACKED_HTTP_CONNECTIONS", "1025")

		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("Both enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		// Setting a different value
		t.Setenv("DD_NETWORK_CONFIG_MAX_TRACKED_HTTP_CONNECTIONS", "1026")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_TRACKED_HTTP_CONNECTIONS", "1025")

		cfg := New()

		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1025))
	})

	t.Run("Not enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()
		// Default value.
		require.Equal(t, cfg.MaxTrackedHTTPConnections, int64(1024))
	})
}

func TestHTTPMapCleanerInterval(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
system_probe_config:
  http_map_cleaner_interval_in_s: 1025
`)

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_HTTP_MAP_CLEANER_INTERVAL_IN_S", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
service_monitoring_config:
  http_map_cleaner_interval_in_s: 1025
`)

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAP_CLEANER_INTERVAL_IN_S", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_HTTP_MAP_CLEANER_INTERVAL_IN_S", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAP_CLEANER_INTERVAL_IN_S", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("Both enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		// Setting a different value
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_HTTP_MAP_CLEANER_INTERVAL_IN_S", "1026")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAP_CLEANER_INTERVAL_IN_S", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPMapCleanerInterval, 1025*time.Second)
	})

	t.Run("Not enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()
		// Default value.
		require.Equal(t, cfg.HTTPMapCleanerInterval, 300*time.Second)
	})
}

func TestHTTPIdleConnectionTTL(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
system_probe_config:
  http_idle_connection_ttl_in_s: 1025
`)

		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_HTTP_IDLE_CONNECTION_TTL_IN_S", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
service_monitoring_config:
  http_idle_connection_ttl_in_s: 1025
`)
		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_IDLE_CONNECTION_TTL_IN_S", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_HTTP_IDLE_CONNECTION_TTL_IN_S", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_IDLE_CONNECTION_TTL_IN_S", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("Both enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		// Setting a different value
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_HTTP_IDLE_CONNECTION_TTL_IN_S", "1026")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_IDLE_CONNECTION_TTL_IN_S", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPIdleConnectionTTL, 1025*time.Second)
	})

	t.Run("Not enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()
		// Default value.
		require.Equal(t, cfg.HTTPIdleConnectionTTL, 30*time.Second)
	})
}

func TestHTTPNotificationThreshold(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
network_config:
  http_notification_threshold: 100
`)
		require.Equal(t, cfg.HTTPNotificationThreshold, int64(100))
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_NOTIFICATION_THRESHOLD", "100")

		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(100))
	})

	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
service_monitoring_config:
  http_notification_threshold: 100
`)
		require.Equal(t, cfg.HTTPNotificationThreshold, int64(100))
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_NOTIFICATION_THRESHOLD", "100")

		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(100))
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_NOTIFICATION_THRESHOLD", "100")

		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(100))
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_NOTIFICATION_THRESHOLD", "100")

		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(100))
	})

	t.Run("Both enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		// Setting a different value
		t.Setenv("DD_NETWORK_CONFIG_HTTP_NOTIFICATION_THRESHOLD", "101")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_NOTIFICATION_THRESHOLD", "100")

		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(100))
	})

	t.Run("Not enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()
		// Default value.
		require.Equal(t, cfg.HTTPNotificationThreshold, int64(512))
	})
}

// Testing we're not exceeding the limit for http_notification_threshold.
func TestHTTPNotificationThresholdOverLimit(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
network_config:
  http_notification_threshold: 1025
`)
		require.Equal(t, cfg.HTTPNotificationThreshold, int64(512))
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_NOTIFICATION_THRESHOLD", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(512))
	})

	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
service_monitoring_config:
  http_notification_threshold: 1025
`)
		require.Equal(t, cfg.HTTPNotificationThreshold, int64(512))
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_NOTIFICATION_THRESHOLD", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(512))
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_NOTIFICATION_THRESHOLD", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(512))
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_NOTIFICATION_THRESHOLD", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(512))
	})

	t.Run("Both enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		// Setting a different value
		t.Setenv("DD_NETWORK_CONFIG_HTTP_NOTIFICATION_THRESHOLD", "1026")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_NOTIFICATION_THRESHOLD", "1025")

		cfg := New()

		require.Equal(t, cfg.HTTPNotificationThreshold, int64(512))
	})

	t.Run("Not enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()
		// Default value.
		require.Equal(t, cfg.HTTPNotificationThreshold, int64(512))
	})
}

func TestHTTPMaxRequestFragment(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
network_config:
  http_max_request_fragment: 155
`)
		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "155")

		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
service_monitoring_config:
  http_max_request_fragment: 155
`)
		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "155")

		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "155")

		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "155")

		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("Both enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		// Setting a different value
		t.Setenv("DD_NETWORK_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "151")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "155")

		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(155))
	})

	t.Run("Not enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()
		// Default value.
		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(160))
	})
}

// Testing we're not exceeding the hard coded limit of http_max_request_fragment.
func TestHTTPMaxRequestFragmentLimit(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
network_config:
  http_max_request_fragment: 175
`)
		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(160))
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "175")

		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(160))
	})

	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
service_monitoring_config:
  http_max_request_fragment: 175
`)
		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(160))
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "175")

		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(160))
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_NETWORK_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "175")

		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(160))
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "175")

		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(160))
	})

	t.Run("Both enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		// Setting a different value
		t.Setenv("DD_NETWORK_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "176")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_HTTP_MAX_REQUEST_FRAGMENT", "175")

		cfg := New()

		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(160))
	})

	t.Run("Not enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()
		// Default value.
		require.Equal(t, cfg.HTTPMaxRequestFragment, int64(160))
	})
}

func TestMaxClosedConnectionsBuffered(t *testing.T) {
	maxTrackedConnections := New().MaxTrackedConnections

	t.Run("value set", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_MAX_CLOSED_CONNECTIONS_BUFFERED", fmt.Sprintf("%d", maxTrackedConnections-1))
		cfg := New()
		require.Equal(t, maxTrackedConnections-1, cfg.MaxClosedConnectionsBuffered)
	})

	t.Run("value not set", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()
		require.Equal(t, cfg.MaxTrackedConnections, cfg.MaxClosedConnectionsBuffered)
	})
}

func TestMaxHTTPStatsBuffered(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDSystemProbeConfig-MaxHTTPStatsBufferedDeprecated.yaml")
		require.NoError(t, err)
		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_MAX_HTTP_STATS_BUFFERED", "513")

		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDSystemProbeConfig-MaxHTTPStatsBuffered.yaml")
		require.NoError(t, err)
		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_HTTP_STATS_BUFFERED", "513")

		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_MAX_HTTP_STATS_BUFFERED", "513")

		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_HTTP_STATS_BUFFERED", "513")

		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("Both enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_MAX_HTTP_STATS_BUFFERED", "514")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_HTTP_STATS_BUFFERED", "513")

		cfg := New()

		require.Equal(t, cfg.MaxHTTPStatsBuffered, 513)
	})

	t.Run("Not enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()
		// Default value.
		require.Equal(t, cfg.MaxHTTPStatsBuffered, 100000)
	})
}

func TestMaxKafkaStatsBuffered(t *testing.T) {
	t.Run("value set through env var", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_KAFKA_STATS_BUFFERED", "50000")

		cfg := New()
		assert.Equal(t, 50000, cfg.MaxKafkaStatsBuffered)
	})

	t.Run("value set through yaml", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
service_monitoring_config:
  max_kafka_stats_buffered: 30000
`)

		assert.Equal(t, 30000, cfg.MaxKafkaStatsBuffered)
	})
}

func TestNetworkConfigEnabled(t *testing.T) {
	ys := true

	for i, tc := range []struct {
		sysIn, npmIn, usmIn, dsmIn         *bool
		npmEnabled, usmEnabled, dsmEnabled bool
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
		{sysIn: nil, npmIn: nil, usmIn: nil, dsmIn: &ys, npmEnabled: false, usmEnabled: true, dsmEnabled: true},
		{sysIn: nil, npmIn: nil, usmIn: &ys, dsmIn: &ys, npmEnabled: false, usmEnabled: true, dsmEnabled: true},
		{sysIn: nil, npmIn: &ys, usmIn: &ys, dsmIn: &ys, npmEnabled: true, usmEnabled: true, dsmEnabled: true},
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
			if tc.dsmIn != nil {
				t.Setenv("DD_SYSTEM_PROBE_DATA_STREAMS_ENABLED", strconv.FormatBool(*tc.dsmIn))
			}

			aconfig.ResetSystemProbeConfig(t)
			_, err = sysconfig.New(f.Name())
			require.NoError(t, err)
			cfg := New()
			assert.Equal(t, tc.npmEnabled, cfg.NPMEnabled, "npm state")
			assert.Equal(t, tc.usmEnabled, cfg.ServiceMonitoringEnabled, "usm state")
			assert.Equal(t, tc.dsmEnabled, cfg.DataStreamsEnabled, "dsm state")
		})
	}
}

func TestIstioMonitoring(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()
		assert.False(t, cfg.EnableIstioMonitoring)
	})

	t.Run("via yaml", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
service_monitoring_config:
  tls:
    istio:
      enabled: true
`)
		assert.True(t, cfg.EnableIstioMonitoring)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_ISTIO_ENABLED", "true")

		cfg := New()
		assert.True(t, cfg.EnableIstioMonitoring)
	})
}

func TestUSMTLSNativeEnabled(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
network_config:
  enable_https_monitoring: true
`)

		require.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING", "true")

		cfg := New()

		require.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
service_monitoring_config:
  tls:
    native:
      enabled: true
`)
		require.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_NATIVE_ENABLED", "true")

		cfg := New()

		require.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_NATIVE_ENABLED", "false")

		cfg := New()

		require.False(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING", "false")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_NATIVE_ENABLED", "true")

		cfg := New()

		require.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("Both enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		// Setting a different value
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_NATIVE_ENABLED", "true")
		cfg := New()

		require.True(t, cfg.EnableNativeTLSMonitoring)
	})

	t.Run("Not enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()
		// Default value.
		require.False(t, cfg.EnableNativeTLSMonitoring)
	})
}

func TestUSMTLSGoEnabled(t *testing.T) {
	t.Run("via deprecated YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
service_monitoring_config:
  enable_go_tls_support: true
`)

		require.True(t, cfg.EnableGoTLSSupport)
	})

	t.Run("via deprecated ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_GO_TLS_SUPPORT", "true")

		cfg := New()

		require.True(t, cfg.EnableGoTLSSupport)
	})

	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := configurationFromYAML(t, `
service_monitoring_config:
  tls:
    go:
      enabled: true
`)
		require.True(t, cfg.EnableGoTLSSupport)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_GO_ENABLED", "true")

		cfg := New()

		require.True(t, cfg.EnableGoTLSSupport)
	})

	t.Run("Deprecated is enabled, new is disabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_GO_TLS_SUPPORT", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_GO_ENABLED", "false")

		cfg := New()

		require.False(t, cfg.EnableGoTLSSupport)
	})

	t.Run("Deprecated is disabled, new is enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_GO_TLS_SUPPORT", "false")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_GO_ENABLED", "true")

		cfg := New()

		require.True(t, cfg.EnableGoTLSSupport)
	})

	t.Run("Both enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		// Setting a different value
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_GO_TLS_SUPPORT", "true")
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_TLS_GO_ENABLED", "true")
		cfg := New()

		require.True(t, cfg.EnableGoTLSSupport)
	})

	t.Run("Not enabled", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		cfg := New()
		// Default value.
		require.False(t, cfg.EnableGoTLSSupport)
	})
}

func configurationFromYAML(t *testing.T, yaml string) *Config {
	f, err := os.CreateTemp("", "system-probe.*.yaml")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	b := []byte(yaml)
	n, err := f.Write(b)
	require.NoError(t, err)
	require.Equal(t, len(b), n)
	f.Sync()

	_, err = sysconfig.New(f.Name())
	require.NoError(t, err)
	return New()
}
