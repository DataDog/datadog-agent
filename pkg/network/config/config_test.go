// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
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

func TestDisablingDNSInspection(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-DisableDNS.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.DNSInspection)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)
		t.Setenv("DD_DISABLE_DNS_INSPECTION", "true")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.DNSInspection)
	})
}

func TestDisablingProtocolClassification(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-NoPRTCLClassifying.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.ProtocolClassificationEnabled)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)
		t.Setenv("DD_ENABLE_PROTOCOL_CLASSIFICATION", "false")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.ProtocolClassificationEnabled)
	})
}

func TestEnableGoTLSSupport(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableGoTLS.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableGoTLSSupport)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_GO_TLS_SUPPORT", "true")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableGoTLSSupport)
	})
}

func TestEnableHTTPStatsByStatusCode(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig(t)
		_, err := sysconfig.New("./testdata/TestDDSystemProbeConfig-EnableHTTPStatusCodeAggr.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPStatsByStatusCode)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP_STATS_BY_STATUS_CODE", "true")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPStatsByStatusCode)
	})
}

func TestEnableHTTPMonitoring(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableHTTP.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING", "true")

		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPMonitoring)
	})
}

func TestEnableDataStreams(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableDataStreams.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.DataStreamsEnabled)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_DATA_STREAMS_ENABLED", "true")

		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.DataStreamsEnabled)
	})
}

func TestEnableJavaTLSSupport(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig(t)

		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableJavaTLS.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableJavaTLSSupport)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)

		t.Setenv("DD_SERVICE_MONITORING_CONFIG_JAVA_TLS_ENABLED", "true")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableJavaTLSSupport)
	})
}

func TestEnableHTTP2Monitoring(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig(t)

		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableHTTP2.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTP2Monitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)

		t.Setenv("DD_SERVICE_MONITORING_CONFIG_ENABLE_HTTP2_MONITORING", "true")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTP2Monitoring)
	})
}

func TestDefaultDisabledJavaTLSSupport(t *testing.T) {
	newConfig(t)

	_, err := sysconfig.New("")
	require.NoError(t, err)
	cfg := New()

	assert.False(t, cfg.EnableJavaTLSSupport)
}

func TestDefaultDisabledHTTP2Support(t *testing.T) {
	newConfig(t)

	_, err := sysconfig.New("")
	require.NoError(t, err)
	cfg := New()

	assert.False(t, cfg.EnableHTTP2Monitoring)
}

func TestDisableGatewayLookup(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig(t)
		// default config
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableGatewayLookup)

		newConfig(t)
		_, err = sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-DisableGwLookup.yaml")
		require.NoError(t, err)
		cfg = New()

		assert.False(t, cfg.EnableGatewayLookup)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_GATEWAY_LOOKUP", "false")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.EnableGatewayLookup)
	})
}

func TestIgnoreConntrackInitFailure(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-IgnoreCTInitFailure.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.IgnoreConntrackInitFailure)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)
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
		newConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableDNSStats.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.CollectDNSStats)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)
		t.Setenv("DD_COLLECT_DNS_STATS", "false")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.CollectDNSStats)

		newConfig(t)
		t.Setenv("DD_COLLECT_DNS_STATS", "true")
		_, err = sysconfig.New("")
		require.NoError(t, err)
		cfg = New()

		assert.True(t, cfg.CollectDNSStats)
	})
}

func TestDisablingDNSDomainCollection(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-DisableDNSDomains.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.CollectDNSDomains)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)
		t.Setenv("DD_COLLECT_DNS_DOMAINS", "false")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.CollectDNSDomains)

		newConfig(t)
		t.Setenv("DD_COLLECT_DNS_DOMAINS", "true")
		_, err = sysconfig.New("")
		require.NoError(t, err)
		cfg = New()

		assert.True(t, cfg.CollectDNSDomains)
	})
}

func TestSettingMaxDNSStats(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-DisableDNSDomains.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.Equal(t, 100, cfg.MaxDNSStats)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)
		os.Unsetenv("DD_SYSTEM_PROBE_CONFIG_MAX_DNS_STATS")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.Equal(t, 20000, cfg.MaxDNSStats) // default value

		newConfig(t)
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

	t.Run("via YAML", func(t *testing.T) {
		newConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-HTTPReplaceRules.yaml")
		require.NoError(t, err)
		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_HTTP_REPLACE_RULES", `
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
        `)

		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		require.Len(t, cfg.HTTPReplaceRules, 3)
		for i, r := range expected {
			assert.Equal(t, r, cfg.HTTPReplaceRules[i])
		}
	})
}

func TestMaxClosedConnectionsBuffered(t *testing.T) {
	maxTrackedConnections := New().MaxTrackedConnections

	t.Run("value set", func(t *testing.T) {
		newConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_MAX_CLOSED_CONNECTIONS_BUFFERED", fmt.Sprintf("%d", maxTrackedConnections-1))

		cfg := New()
		require.Equal(t, int(maxTrackedConnections-1), cfg.MaxClosedConnectionsBuffered)
	})

	t.Run("value not set", func(t *testing.T) {
		newConfig(t)
		cfg := New()
		require.Equal(t, int(cfg.MaxTrackedConnections), cfg.MaxClosedConnectionsBuffered)
	})
}

func TestMaxHTTPStatsBuffered(t *testing.T) {
	t.Run("value set through env var", func(t *testing.T) {
		newConfig(t)
		t.Setenv("DD_SYSTEM_PROBE_NETWORK_MAX_HTTP_STATS_BUFFERED", "50000")

		cfg := New()
		assert.Equal(t, 50000, cfg.MaxHTTPStatsBuffered)
	})

	t.Run("value set through yaml", func(t *testing.T) {
		newConfig(t)
		cfg := configurationFromYAML(t, `
network_config:
  max_http_stats_buffered: 30000
`)

		assert.Equal(t, 30000, cfg.MaxHTTPStatsBuffered)
	})
}

func TestMaxKafkaStatsBuffered(t *testing.T) {
	t.Run("value set through env var", func(t *testing.T) {
		newConfig(t)
		t.Setenv("DD_SERVICE_MONITORING_CONFIG_MAX_KAFKA_STATS_BUFFERED", "50000")

		cfg := New()
		assert.Equal(t, 50000, cfg.MaxKafkaStatsBuffered)
	})

	t.Run("value set through yaml", func(t *testing.T) {
		newConfig(t)
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

			newConfig(t)
			_, err = sysconfig.New(f.Name())
			require.NoError(t, err)
			cfg := New()
			assert.Equal(t, tc.npmEnabled, cfg.NPMEnabled, "npm state")
			assert.Equal(t, tc.usmEnabled, cfg.ServiceMonitoringEnabled, "usm state")
			assert.Equal(t, tc.dsmEnabled, cfg.DataStreamsEnabled, "dsm state")
		})
	}
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
