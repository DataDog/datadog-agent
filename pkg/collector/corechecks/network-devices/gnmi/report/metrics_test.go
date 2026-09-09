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
	baseTags := []string{
		"device_ip:" + deviceAddress,
		"device_id:default:" + deviceAddress,
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
							Path:   "/components/component/state/temperature",
							Metric: "snmp.temp",
							Type:   config.MetricTypeGauge,
						},
					},
				},
			},
			snapshot: []client.CachedValue{
				{
					Key: client.CacheKey{Path: "/components/component/state/temperature"},
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
							Path:   "/interfaces/interface/state/counters/in-octets",
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
						Path: "/interfaces/interface/state/counters/in-octets",
						Keys: map[string]string{"name": "eth0"},
					},
					Entry: client.CacheEntry{
						Value:     uint64(2_000_000),
						Timestamp: time.Unix(1, 0),
					},
				},
				{
					Key: client.CacheKey{
						Path: "/interfaces/interface/state/ifindex",
						Keys: map[string]string{"name": "eth0"},
					},
					Entry: client.CacheEntry{Value: int32(42)},
				},
				{
					Key: client.CacheKey{
						Path: "/interfaces/interface/state/description",
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
						"device_ip:" + deviceAddress,
						"device_id:default:" + deviceAddress,
						"interface:eth0",
						"interface_index:42",
						"interface_alias:uplink",
						"dd.internal.resource:ndm_interface:default:" + deviceAddress + ":42",
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
							Path:   "/interfaces/interface/state/admin-status",
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
						Path: "/interfaces/interface/state/admin-status",
						Keys: map[string]string{"name": "eth0"},
					},
					Entry: client.CacheEntry{
						Value:     "UP",
						Timestamp: time.Unix(1, 0),
					},
				},
				{
					Key: client.CacheKey{
						Path: "/interfaces/interface/state/admin-status",
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
							Path:   "/system/state/uptime",
							Metric: "snmp.sysUpTime",
							Type:   config.MetricTypeGauge,
						},
					},
				},
			},
			snapshot: []client.CachedValue{
				{
					Key:   client.CacheKey{Path: "/system/state/uptime"},
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
							Path:   "/interfaces/interface/state/counters/in-octets",
							Metric: "snmp.ifHCInOctets",
							Type:   config.MetricTypeMonotonicCount,
						},
						{
							Path:   "/interfaces/interface/state/admin-status",
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
					Key:   client.CacheKey{Path: "/interfaces/interface/state/counters/in-octets"},
					Entry: client.CacheEntry{Value: []byte{1, 2, 3}},
				},
				{
					Key:   client.CacheKey{Path: "/interfaces/interface/state/admin-status"},
					Entry: client.CacheEntry{Value: "UNKNOWN"},
				},
				{
					Key:   client.CacheKey{Path: "/interfaces/interface/state/counters/out-octets"},
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
							Path:   "system/state/uptime",
							Metric: "snmp.sysUpTime",
							Type:   config.MetricTypeGauge,
						},
					},
				},
			},
			snapshot: []client.CachedValue{
				{
					Key:   client.CacheKey{Path: "/system/state/uptime"},
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
			for _, want := range tt.wantMetrics {
				mockSender.AssertMetric(t, want.method, want.name, want.value, "", want.tags)
				switch want.method {
				case "Gauge":
					gaugeCalls++
				case "MonotonicCount":
					monotonicCalls++
				}
			}
			mockSender.AssertNumberOfCalls(t, "Gauge", gaugeCalls)
			mockSender.AssertNumberOfCalls(t, "MonotonicCount", monotonicCalls)
		})
	}
}

func TestReportMetricsValidation(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.1"},
		Profile: config.ProfileDefinition{
			Metrics: []config.MetricConfig{
				{Path: "/system/state/uptime", Metric: "snmp.sysUpTime", Type: config.MetricTypeGauge},
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
