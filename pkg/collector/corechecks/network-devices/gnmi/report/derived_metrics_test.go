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

func TestReportDerivedMetricsMemoryUsage(t *testing.T) {
	deviceAddress := "10.0.0.5"
	baseTags := []string{
		"device_ip:" + deviceAddress,
		"device_id:default:" + deviceAddress,
	}
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: deviceAddress},
		Profile:  config.ProfileDefinition{},
	}
	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{
				Path: pathMemoryUsed,
				Keys: map[string]string{"name": "Chassis"},
			},
			Entry: client.CacheEntry{Value: int64(25)},
		},
		{
			Key: client.CacheKey{
				Path: pathMemoryAvail,
				Keys: map[string]string{"name": "Chassis"},
			},
			Entry: client.CacheEntry{Value: int64(75)},
		},
	}

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	err := ReportDerivedMetrics(mockSender, cfg, snapshot, NewBandwidthState())
	require.NoError(t, err)

	mockSender.AssertMetric(t, "Gauge", metricMemoryUsage, 25.0, "", append(baseTags, "memory:Chassis"))
}

func TestReportDerivedMetricsInterfaceBandwidthUsage(t *testing.T) {
	deviceAddress := "10.0.0.5"
	ifSpeed := uint64(1_000_000_000)
	interfaceTags := []string{
		"device_ip:" + deviceAddress,
		"device_id:default:" + deviceAddress,
		"interface:eth0",
		"interface_index:42",
		"dd.internal.resource:ndm_interface:default:" + deviceAddress + ":42",
	}
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: deviceAddress},
		Profile: config.ProfileDefinition{
			Metrics: []config.MetricConfig{
				{
					Path: pathPortSpeed,
					ValueMap: map[string]int{
						"SPEED_1GB": 1_000_000_000,
					},
				},
			},
			Metadata: config.MetadataConfig{
				Interface: config.InterfaceMetadataConfig{
					Keys: map[string]string{
						"interface": "name",
					},
					IfIndex: "/openconfig/interfaces/interface/state/ifindex",
				},
			},
		},
	}

	snapshot := func(inOctets, outOctets float64) []client.CachedValue {
		return []client.CachedValue{
			{
				Key: client.CacheKey{
					Path: pathInOctets,
					Keys: map[string]string{"name": "eth0"},
				},
				Entry: client.CacheEntry{Value: inOctets},
			},
			{
				Key: client.CacheKey{
					Path: pathOutOctets,
					Keys: map[string]string{"name": "eth0"},
				},
				Entry: client.CacheEntry{Value: outOctets},
			},
			{
				Key: client.CacheKey{
					Path: pathPortSpeed,
					Keys: map[string]string{"name": "eth0"},
				},
				Entry: client.CacheEntry{Value: "SPEED_1GB"},
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
		}
	}

	bandwidthState := NewBandwidthState()
	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	start := time.Unix(100, 0)
	bandwidthTimeNow = func() time.Time { return start }
	err := ReportDerivedMetrics(mockSender, cfg, snapshot(1_500_000, 750_000), bandwidthState)
	require.NoError(t, err)
	mockSender.AssertNotCalled(t, "Gauge", metricBandwidthInUsage)

	bandwidthTimeNow = func() time.Time { return start.Add(15 * time.Second) }
	mockSender = mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()
	err = ReportDerivedMetrics(mockSender, cfg, snapshot(3_000_000, 1_500_000), bandwidthState)
	require.NoError(t, err)

	inUsage := ((3_000_000 * 8) / float64(ifSpeed)) * 100
	prevInUsage := ((1_500_000 * 8) / float64(ifSpeed)) * 100
	expectedInRate := (inUsage - prevInUsage) / 15.0
	outUsage := ((1_500_000 * 8) / float64(ifSpeed)) * 100
	prevOutUsage := ((750_000 * 8) / float64(ifSpeed)) * 100
	expectedOutRate := (outUsage - prevOutUsage) / 15.0

	mockSender.AssertMetric(t, "Gauge", metricBandwidthInUsage, expectedInRate, "", interfaceTags)
	mockSender.AssertMetric(t, "Gauge", metricBandwidthOutUsage, expectedOutRate, "", interfaceTags)
}

func TestReportMetricsEmitsThroughputRates(t *testing.T) {
	deviceAddress := "10.0.0.1"
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: deviceAddress},
		Profile: config.ProfileDefinition{
			Metrics: []config.MetricConfig{
				{
					Path:   pathInOctets,
					Metric: "snmp.ifHCInOctets",
					Type:   config.MetricTypeMonotonicCount,
					Tags: map[string]string{
						"interface": "name",
					},
				},
			},
		},
	}
	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{
				Path: pathInOctets,
				Keys: map[string]string{"name": "eth0"},
			},
			Entry: client.CacheEntry{Value: uint64(2_000_000)},
		},
	}

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	err := ReportMetrics(mockSender, cfg, snapshot, snapshot)
	require.NoError(t, err)

	mockSender.AssertMetric(t, "MonotonicCount", "snmp.ifHCInOctets", 2_000_000, "", []string{
		"device_ip:" + deviceAddress,
		"device_id:default:" + deviceAddress,
		"interface:eth0",
	})
	mockSender.AssertMetric(t, "Rate", "snmp.ifHCInOctets.rate", 2_000_000, "", []string{
		"device_ip:" + deviceAddress,
		"device_id:default:" + deviceAddress,
		"interface:eth0",
	})
}

func TestEvaluateMemoryUsage(t *testing.T) {
	usage, err := evaluateMemoryUsage(25, 100)
	require.NoError(t, err)
	assert.Equal(t, 25.0, usage)
}
