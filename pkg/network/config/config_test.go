// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package config

import (
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
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

func TestMaxClosedConnectionsBuffered(t *testing.T) {
	maxTrackedConnections := New().MaxTrackedConnections

	t.Run("value set", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_CONFIG_MAX_CLOSED_CONNECTIONS_BUFFERED", strconv.FormatUint(uint64(maxTrackedConnections-1), 10))
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
		t.Setenv("DD_NETWORK_CONFIG_MAX_FAILED_CONNECTIONS_BUFFERED", strconv.FormatUint(uint64(maxTrackedConnections-1), 10))
		cfg := New()

		require.Equal(t, maxTrackedConnections-1, cfg.MaxFailedConnectionsBuffered)
	})

	t.Run("value not set", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		require.Equal(t, cfg.MaxTrackedConnections, cfg.MaxFailedConnectionsBuffered)
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
		mockSystemProbe.SetWithoutSource("system_probe_config.process_service_inference.use_windows_service_name", false)

		require.False(t, mockSystemProbe.GetBool("system_probe_config.process_service_inference.use_windows_service_name"))
	})
}

func TestExpectedTagsDuration(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Equal(t, 30*time.Minute, cfg.ExpectedTagsDuration)
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("system_probe_config.expected_tags_duration", 20*time.Second)
		cfg := New()

		assert.Equal(t, 20*time.Second, cfg.ExpectedTagsDuration)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_SYSTEM_PROBE_EXPECTED_TAGS_DURATION", "30s")
		cfg := New()

		assert.Equal(t, 30*time.Second, cfg.ExpectedTagsDuration)
	})
}

func TestEnableCertCollection(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Equal(t, false, cfg.EnableCertCollection)
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.enable_cert_collection", true)
		cfg := New()

		assert.Equal(t, true, cfg.EnableCertCollection)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_ENABLE_CERT_COLLECTION", "true")
		cfg := New()

		assert.Equal(t, true, cfg.EnableCertCollection)
	})
}

func TestEnableCertCollectionMapCleanerInterval(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Equal(t, 30*time.Second, cfg.CertCollectionMapCleanerInterval)
	})

	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.cert_collection_map_cleaner_interval", 60*time.Second)
		cfg := New()

		assert.Equal(t, 60*time.Second, cfg.CertCollectionMapCleanerInterval)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_NETWORK_CONFIG_CERT_COLLECTION_MAP_CLEANER_INTERVAL", "42s")
		cfg := New()

		assert.Equal(t, 42*time.Second, cfg.CertCollectionMapCleanerInterval)
	})
}

func TestEnableContainerStore(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Equal(t, true, cfg.EnableContainerStore)
	})
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("event_monitoring_config.network_process.container_store.enabled", true)
		cfg := New()

		assert.Equal(t, true, cfg.EnableContainerStore)
	})
	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_EVENT_MONITORING_CONFIG_NETWORK_PROCESS_CONTAINER_STORE_ENABLED", "true")
		cfg := New()

		assert.Equal(t, true, cfg.EnableContainerStore)
	})
}

func TestMaxContainersTracked(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()

		assert.Equal(t, 1024, cfg.MaxContainersTracked)
	})
	t.Run("via YAML", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("event_monitoring_config.network_process.container_store.max_containers_tracked", 42)
		cfg := New()

		assert.Equal(t, 42, cfg.MaxContainersTracked)
	})
	t.Run("via ENV variable", func(t *testing.T) {
		mock.NewSystemProbe(t)
		t.Setenv("DD_EVENT_MONITORING_CONFIG_NETWORK_PROCESS_CONTAINER_STORE_MAX_CONTAINERS_TRACKED", "42")
		cfg := New()

		assert.Equal(t, 42, cfg.MaxContainersTracked)
	})
}

func TestDNSMonitoringPorts(t *testing.T) {
	t.Run("default value", func(t *testing.T) {
		mock.NewSystemProbe(t)
		cfg := New()
		assert.Equal(t, []int{53}, cfg.DNSMonitoringPortList)
	})

	t.Run("via YAML - single port 53", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.dns_monitoring_ports", []int{53})
		cfg := New()
		assert.Equal(t, []int{53}, cfg.DNSMonitoringPortList)
	})

	t.Run("via YAML - single port non-53", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.dns_monitoring_ports", []int{5353})
		cfg := New()
		assert.Equal(t, []int{5353}, cfg.DNSMonitoringPortList)
	})

	t.Run("via YAML - multiple ports including 53", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.dns_monitoring_ports", []int{53, 5353})
		cfg := New()
		assert.Equal(t, []int{53, 5353}, cfg.DNSMonitoringPortList)
	})

	t.Run("via YAML - multiple ports excluding 53", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		mockSystemProbe.SetWithoutSource("network_config.dns_monitoring_ports", []int{8053, 5353})
		cfg := New()
		assert.Equal(t, []int{8053, 5353}, cfg.DNSMonitoringPortList)
	})

	t.Run("via YAML - http ports should be removed", func(t *testing.T) {
		mockSystemProbe := mock.NewSystemProbe(t)
		// HTTP ports would capture an enormous amount of traffic and cause issues.
		// network config prevents the user from accidentally enabling these ports
		mockSystemProbe.SetWithoutSource("network_config.dns_monitoring_ports", []int{53, 443, 5353, 80})
		cfg := New()
		assert.Equal(t, []int{53, 5353}, cfg.DNSMonitoringPortList)
	})
}
