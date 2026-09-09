// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
)

func TestReportMetrics(t *testing.T) {
	deviceAddress := "10.0.0.1"
	deviceID := buildDeviceIDFromConfigAddress(deviceAddress)
	baseTags := []string{
		deviceNamespaceTag,
		"device_ip:" + deviceAddress,
		"device_id:" + deviceID,
		"snmp_device:" + deviceAddress,
		integrationSourceGNMITag,
		internalDeviceResourceTag(deviceID),
	}

	tests := []struct {
		name        string
		cfg         *config.CheckConfig
		snapshot    []client.CachedValue
		wantMetrics []expectedMetric
		errContains string
	}{
		{
			name: "gauge metric",
			cfg: &config.CheckConfig{
				Instance: config.InstanceConfig{
					Address: deviceAddress,
				},
				Profile: config.ProfileDefinition{
					Metrics: []config.MetricConfig{
						{
							Path:   "/openconfig/components/component/state/temperature",
							Metric: "snmp.temp",
							Type:   config.MetricTypeGauge,
						},
					},
				},
			},
			snapshot: []client.CachedValue{
				{
					Key: client.CacheKey{Path: "/openconfig/components/component/state/temperature"},
					Entry: client.CacheEntry{
						Value:     float32(42.5),
						Timestamp: time.Unix(1, 0),
					},
				},
			},
			wantMetrics: []expectedMetric{
				{method: "Gauge", name: "snmp.temp", value: 42.5, tags: baseTags},
			},
		},
		{
			name: "monotonic_count metric",
			cfg: &config.CheckConfig{
				Instance: config.InstanceConfig{
					Address: deviceAddress,
				},
				Profile: config.ProfileDefinition{
					Metrics: []config.MetricConfig{
						{
							Path:   "/openconfig/interfaces/interface/state/counters/in-octets",
							Metric: "snmp.ifHCInOctets",
							Type:   config.MetricTypeMonotonicCount,
							Tags: map[string]string{
								"interface": "name",
							},
						},
					},
				},
			},
			snapshot: []client.CachedValue{
				{
					Key: client.CacheKey{
						Path: "/openconfig/interfaces/interface/state/counters/in-octets",
						Keys: map[string]string{"name": "eth0"},
					},
					Entry: client.CacheEntry{
						Value:     uint64(2_000_000),
						Timestamp: time.Unix(1, 0),
					},
				},
				{
					Key: client.CacheKey{
						Path: "/openconfig/interfaces/interface/state/ifindex",
						Keys: map[string]string{"name": "eth0"},
					},
					Entry: client.CacheEntry{Value: int32(42)},
				},
				{
					Key: client.CacheKey{
						Path: "/openconfig/interfaces/interface/state/description",
						Keys: map[string]string{"name": "eth0"},
					},
					Entry: client.CacheEntry{Value: "uplink"},
				},
			},
			wantMetrics: []expectedMetric{
				{
					method: "MonotonicCount",
					name:   "snmp.ifHCInOctets",
					value:  2_000_000,
					tags: []string{
						deviceNamespaceTag,
						"device_ip:" + deviceAddress,
						"device_id:" + deviceID,
						"snmp_device:" + deviceAddress,
						integrationSourceGNMITag,
						internalDeviceResourceTag(deviceID),
						"interface:eth0",
						"interface_alias:uplink",
						"dd.internal.resource:ndm_interface:" + deviceID + ":eth0",
					},
				},
				{
					method: "Rate",
					name:   "snmp.ifHCInOctets.rate",
					value:  2_000_000,
					tags: []string{
						deviceNamespaceTag,
						"device_ip:" + deviceAddress,
						"device_id:" + deviceID,
						"snmp_device:" + deviceAddress,
						integrationSourceGNMITag,
						internalDeviceResourceTag(deviceID),
						"interface:eth0",
						"interface_alias:uplink",
						"dd.internal.resource:ndm_interface:" + deviceID + ":eth0",
					},
				},
			},
		},
		{
			name: "value_map for string enumeration",
			cfg: &config.CheckConfig{
				Instance: config.InstanceConfig{
					Address: deviceAddress,
				},
				Profile: config.ProfileDefinition{
					Metrics: []config.MetricConfig{
						{
							Path:   "/openconfig/interfaces/interface/state/admin-status",
							Metric: "snmp.ifAdminStatus",
							Type:   config.MetricTypeGauge,
							Tags: map[string]string{
								"interface": "name",
							},
							ValueMap: map[string]int{
								"UP":   1,
								"DOWN": 2,
							},
						},
					},
				},
			},
			snapshot: []client.CachedValue{
				{
					Key: client.CacheKey{
						Path: "/openconfig/interfaces/interface/state/admin-status",
						Keys: map[string]string{"name": "eth0"},
					},
					Entry: client.CacheEntry{
						Value:     "UP",
						Timestamp: time.Unix(1, 0),
					},
				},
				{
					Key: client.CacheKey{
						Path: "/openconfig/interfaces/interface/state/admin-status",
						Keys: map[string]string{"name": "eth1"},
					},
					Entry: client.CacheEntry{
						Value:     "DOWN",
						Timestamp: time.Unix(1, 0),
					},
				},
			},
			wantMetrics: []expectedMetric{
				{
					method: "Gauge",
					name:   "snmp.ifAdminStatus",
					value:  1,
					tags:   append(baseTags, "interface:eth0"),
				},
				{
					method: "Gauge",
					name:   "snmp.ifAdminStatus",
					value:  2,
					tags:   append(baseTags, "interface:eth1"),
				},
			},
		},
		{
			name: "instance tags are included",
			cfg: &config.CheckConfig{
				Instance: config.InstanceConfig{
					Address: deviceAddress,
					Tags:    []string{"env:prod", "site:dc1"},
				},
				Profile: config.ProfileDefinition{
					Metrics: []config.MetricConfig{
						{
							Path:   "/openconfig/system/state/uptime",
							Metric: "snmp.sysUpTime",
							Type:   config.MetricTypeGauge,
						},
					},
				},
			},
			snapshot: []client.CachedValue{
				{
					Key:   client.CacheKey{Path: "/openconfig/system/state/uptime"},
					Entry: client.CacheEntry{Value: int64(12345)},
				},
			},
			wantMetrics: []expectedMetric{
				{
					method: "Gauge",
					name:   "snmp.sysUpTime",
					value:  12345,
					tags:   append(baseTags, "env:prod", "site:dc1"),
				},
			},
		},
		{
			name: "skip unsupported values without failing snapshot",
			cfg: &config.CheckConfig{
				Instance: config.InstanceConfig{
					Address: deviceAddress,
				},
				Profile: config.ProfileDefinition{
					Metrics: []config.MetricConfig{
						{
							Path:   "/openconfig/interfaces/interface/state/counters/in-octets",
							Metric: "snmp.ifHCInOctets",
							Type:   config.MetricTypeMonotonicCount,
						},
						{
							Path:   "/openconfig/interfaces/interface/state/admin-status",
							Metric: "snmp.ifAdminStatus",
							Type:   config.MetricTypeGauge,
							ValueMap: map[string]int{
								"UP": 1,
							},
						},
					},
				},
			},
			snapshot: []client.CachedValue{
				{
					Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/counters/in-octets"},
					Entry: client.CacheEntry{Value: []byte{1, 2, 3}},
				},
				{
					Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/admin-status"},
					Entry: client.CacheEntry{Value: "UNKNOWN"},
				},
				{
					Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/counters/out-octets"},
					Entry: client.CacheEntry{Value: int64(99)},
				},
			},
			wantMetrics: nil,
		},
		{
			name: "profile path without leading slash",
			cfg: &config.CheckConfig{
				Instance: config.InstanceConfig{
					Address: deviceAddress,
				},
				Profile: config.ProfileDefinition{
					Metrics: []config.MetricConfig{
						{
							Path:   "openconfig/system/state/uptime",
							Metric: "snmp.sysUpTime",
							Type:   config.MetricTypeGauge,
						},
					},
				},
			},
			snapshot: []client.CachedValue{
				{
					Key:   client.CacheKey{Path: "/openconfig/system/state/uptime"},
					Entry: client.CacheEntry{Value: int64(7)},
				},
			},
			wantMetrics: []expectedMetric{
				{method: "Gauge", name: "snmp.sysUpTime", value: 7, tags: baseTags},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
			mockSender.SetupAcceptAll()

			err := ReportMetrics(mockSender, tt.cfg, tt.snapshot, tt.snapshot)
			if tt.errContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)

			gaugeCalls := 0
			monotonicCalls := 0
			rateCalls := 0
			for _, want := range tt.wantMetrics {
				mockSender.AssertMetric(t, want.method, want.name, want.value, "", want.tags)
				switch want.method {
				case "Gauge":
					gaugeCalls++
				case "MonotonicCount":
					monotonicCalls++
				case "Rate":
					rateCalls++
				}
			}
			mockSender.AssertNumberOfCalls(t, "Gauge", gaugeCalls)
			mockSender.AssertNumberOfCalls(t, "MonotonicCount", monotonicCalls)
			mockSender.AssertNumberOfCalls(t, "Rate", rateCalls)
		})
	}
}

func TestBuildDeviceID(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "127.0.0.1", Port: 57401},
		Profile:  config.ProfileDefinition{},
	}

	assert.Equal(t, "default:127.0.0.1", buildDeviceID(cfg))

	cfg.Instance.Address = "srl1"
	assert.Equal(t, "default:srl1", buildDeviceID(cfg))
}

func TestBuildDeviceIDIgnoresTelemetryHostname(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "127.0.0.1"},
		Profile:  config.ProfileDefinition{},
	}
	snapshot := []client.CachedValue{
		{
			Key:   client.CacheKey{Path: "/openconfig/system/state/hostname"},
			Entry: client.CacheEntry{Value: "srl2"},
		},
	}

	assert.Equal(t, "default:127.0.0.1", buildDeviceID(cfg))

	tags := buildBaseTags(cfg, snapshot)
	assert.Contains(t, tags, "device_id:default:127.0.0.1")
	assert.Contains(t, tags, internalDeviceResourceTag("default:127.0.0.1"))
	assert.Contains(t, tags, "snmp_device:127.0.0.1")
	assert.Contains(t, tags, "device_ip:127.0.0.1")
	assert.Contains(t, tags, "snmp_host:srl2")
}

func TestBuildBaseTagsIncludesDeviceResourceTag(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.1"},
	}

	tags := buildBaseTags(cfg, nil)
	assert.Contains(t, tags, "device_id:default:10.0.0.1")
	assert.Contains(t, tags, internalDeviceResourceTag("default:10.0.0.1"))
	assert.NotContains(t, tags, "dd.internal.resource:ndm_interface:")
}

func TestBuildInterfaceMetadataIncludesMetricPaths(t *testing.T) {
	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{
				Path: "/openconfig/interfaces/interface/ethernet/state/port-speed",
				Keys: map[string]string{"name": "ethernet-1/9"},
			},
			Entry: client.CacheEntry{Value: "SPEED_1GB"},
		},
	}
	metricPaths := []string{"/openconfig/interfaces/interface/ethernet/state/port-speed"}

	interfaces := buildInterfaceMetadata("default:srl2", config.DefaultOpenConfigMetadata(), snapshot, metricPaths)
	require.Len(t, interfaces, 1)
	assert.Equal(t, "ethernet-1/9", interfaces[0].Name)
}

func TestReportMetricsValidation(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.1"},
		Profile: config.ProfileDefinition{
			Metrics: []config.MetricConfig{
				{Path: "/openconfig/system/state/uptime", Metric: "snmp.sysUpTime", Type: config.MetricTypeGauge},
			},
		},
	}

	t.Run("nil sender", func(t *testing.T) {
		err := ReportMetrics(nil, cfg, nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sender is nil")
	})

	t.Run("nil config", func(t *testing.T) {
		mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
		err := ReportMetrics(mockSender, nil, nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "check config is nil")
	})
}

type expectedMetric struct {
	method string
	name   string
	value  float64
	tags   []string
}
