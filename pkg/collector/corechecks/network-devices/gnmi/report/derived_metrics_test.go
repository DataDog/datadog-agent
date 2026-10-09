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
	deviceID := buildDeviceIDFromConfigAddress(deviceAddress)
	baseTags := []string{
		deviceNamespaceTag,
		"device_ip:" + deviceAddress,
		"device_id:" + deviceID,
		"snmp_device:" + deviceAddress,
		integrationSourceGNMITag,
		internalDeviceResourceTag(deviceID),
	}
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: deviceAddress},
		Profile: config.ProfileDefinition{
			Metrics: []config.MetricConfig{
				{
					Path:   pathMemoryUsed,
					Metric: "snmp.memory.used",
					Type:   config.MetricTypeGauge,
					Keys:   map[string]string{"component": "name"},
					Tags:   map[string]string{"memory": "name"},
				},
				{
					Path:   pathMemoryAvail,
					Metric: "snmp.memory.free",
					Type:   config.MetricTypeGauge,
					Keys:   map[string]string{"component": "name"},
					Tags:   map[string]string{"memory": "name"},
				},
			},
		},
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

func TestReportDerivedMetricsMemoryUsageRequiresBothPaths(t *testing.T) {
	deviceAddress := "10.0.0.5"
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: deviceAddress},
		Profile: config.ProfileDefinition{
			Metrics: []config.MetricConfig{
				{
					Path:   pathMemoryUsed,
					Metric: "snmp.memory.used",
					Type:   config.MetricTypeGauge,
					Keys:   map[string]string{"component": "name"},
					Tags:   map[string]string{"memory": "name"},
				},
				{
					Path:   pathMemoryAvail,
					Metric: "snmp.memory.free",
					Type:   config.MetricTypeGauge,
					Keys:   map[string]string{"component": "name"},
					Tags:   map[string]string{"memory": "name"},
				},
			},
		},
	}
	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{
				Path: pathMemoryUsed,
				Keys: map[string]string{"name": "FanTray3"},
			},
			Entry: client.CacheEntry{Value: int64(10)},
		},
		{
			Key: client.CacheKey{
				Path: pathMemoryAvail,
				Keys: map[string]string{"name": "ControlA"},
			},
			Entry: client.CacheEntry{Value: int64(90)},
		},
	}

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	err := ReportDerivedMetrics(mockSender, cfg, snapshot, NewBandwidthState())
	require.NoError(t, err)

	mockSender.AssertNotCalled(t, "Gauge", metricMemoryUsage)
}

func TestReportDerivedMetricsMemoryUsageMatchesProfileComponentTags(t *testing.T) {
	deviceAddress := "10.0.0.5"
	deviceID := buildDeviceIDFromConfigAddress(deviceAddress)
	controlTags := []string{
		deviceNamespaceTag,
		"device_ip:" + deviceAddress,
		"device_id:" + deviceID,
		"snmp_device:" + deviceAddress,
		"integration_source:gnmi",
		"memory:ControlA",
	}
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: deviceAddress},
		Profile: config.ProfileDefinition{
			Metrics: []config.MetricConfig{
				{
					Path:   pathMemoryUsed,
					Metric: "snmp.memory.used",
					Type:   config.MetricTypeGauge,
					Keys:   map[string]string{"component": "name"},
					Tags:   map[string]string{"memory": "name"},
				},
				{
					Path:   pathMemoryAvail,
					Metric: "snmp.memory.free",
					Type:   config.MetricTypeGauge,
					Keys:   map[string]string{"component": "name"},
					Tags:   map[string]string{"memory": "name"},
				},
			},
		},
	}
	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{
				Path: pathMemoryUsed,
				Keys: map[string]string{"name": "ControlA"},
			},
			Entry: client.CacheEntry{Value: int64(25)},
		},
		{
			Key: client.CacheKey{
				Path: pathMemoryAvail,
				Keys: map[string]string{"name": "ControlA"},
			},
			Entry: client.CacheEntry{Value: int64(75)},
		},
	}

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	err := ReportDerivedMetrics(mockSender, cfg, snapshot, NewBandwidthState())
	require.NoError(t, err)

	mockSender.AssertMetric(t, "Gauge", metricMemoryUsage, 25.0, "", controlTags)
}

func TestCollectInterfaceBandwidthDataUsesProfilePathsAndKeys(t *testing.T) {
	profile := config.ProfileDefinition{Metrics: []config.MetricConfig{
		{Path: "/vendor/ports/port/state/in-bytes", Metric: "snmp.ifHCInOctets", Tags: map[string]string{"interface": "port_id"}},
		{Path: "/vendor/ports/port/state/out-bytes", Metric: "snmp.ifHCOutOctets", Tags: map[string]string{"interface": "port_id"}},
		{Path: "/vendor/ports/port/state/speed", Metric: "snmp.ifInSpeed", Tags: map[string]string{"interface": "port_id"}},
	}}
	keys := map[string]string{"port_id": "ethernet-1/9"}
	snapshot := []client.CachedValue{
		{Key: client.CacheKey{Path: "/vendor/ports/port/state/in-bytes", Keys: keys}, Entry: client.CacheEntry{Value: uint64(100)}},
		{Key: client.CacheKey{Path: "/vendor/ports/port/state/out-bytes", Keys: keys}, Entry: client.CacheEntry{Value: uint64(200)}},
		{Key: client.CacheKey{Path: "/vendor/ports/port/state/speed", Keys: keys}, Entry: client.CacheEntry{Value: uint64(1_000)}},
	}

	interfaces := collectInterfaceBandwidthData(nil, "default:router", profile, snapshot, nil)
	require.Len(t, interfaces, 1)
	entry := interfaces[cacheKeysID(keys)]
	require.NotNil(t, entry)
	assert.Equal(t, "ethernet-1/9", entry.name)
	assert.Equal(t, float64(100), entry.inOctets)
	assert.Equal(t, float64(200), entry.outOctets)
	assert.Equal(t, uint64(1_000), entry.speed)
}

func TestReportDerivedMetricsInterfaceBandwidthUsage(t *testing.T) {
	deviceAddress := "10.0.0.5"
	deviceID := buildDeviceIDFromConfigAddress(deviceAddress)
	ifSpeed := uint64(1_000_000_000)
	interfaceTags := []string{
		deviceNamespaceTag,
		"device_ip:" + deviceAddress,
		"device_id:" + deviceID,
		"snmp_device:" + deviceAddress,
		"integration_source:gnmi",
		"interface:eth0",
		"dd.internal.resource:ndm_interface:" + deviceID + ":eth0",
	}
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: deviceAddress},
		Profile: config.ProfileDefinition{
			Metrics: []config.MetricConfig{
				{
					Path:   pathInOctets,
					Metric: "snmp.ifHCInOctets",
					Tags:   map[string]string{"interface": "name"},
				},
				{
					Path:   pathOutOctets,
					Metric: "snmp.ifHCOutOctets",
					Tags:   map[string]string{"interface": "name"},
				},
				{
					Path:   pathPortSpeed,
					Metric: "snmp.ifInSpeed",
					Tags:   map[string]string{"interface": "name"},
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
	originalTimeNow := bandwidthTimeNow
	t.Cleanup(func() { bandwidthTimeNow = originalTimeNow })
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

func throughputTestConfig(deviceAddress string) *config.CheckConfig {
	return &config.CheckConfig{
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
}

func inOctetsSample(value uint64, ts time.Time) []client.CachedValue {
	return []client.CachedValue{
		{
			Key:   client.CacheKey{Path: pathInOctets, Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: value, Timestamp: ts},
		},
	}
}

// gaugeValues returns the values submitted as Gauge for metricName.
func gaugeValues(mockSender *mocksender.MockSender, metricName string) []float64 {
	var values []float64
	for _, call := range mockSender.Calls {
		if call.Method == "Gauge" && call.Arguments.String(0) == metricName {
			values = append(values, call.Arguments.Get(1).(float64))
		}
	}
	return values
}

func TestReportMetricsDoesNotEmitThroughputRates(t *testing.T) {
	deviceAddress := "10.0.0.1"
	deviceID := buildDeviceIDFromConfigAddress(deviceAddress)
	snapshot := inOctetsSample(2_000_000, time.Unix(100, 0))

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	require.NoError(t, ReportMetrics(mockSender, throughputTestConfig(deviceAddress), snapshot, snapshot))

	mockSender.AssertMetric(t, "MonotonicCount", "snmp.ifHCInOctets", 2_000_000, "", []string{
		deviceNamespaceTag,
		"device_ip:" + deviceAddress,
		"device_id:" + deviceID,
		"snmp_device:" + deviceAddress,
		"integration_source:gnmi",
		"interface:eth0",
	})
	mockSender.AssertNumberOfCalls(t, "Rate", 0)
	assert.Empty(t, gaugeValues(mockSender, "snmp.ifHCInOctets.rate"))
}

// Throughput is timed by the device sample timestamps, not by when the check
// runs, and an unchanged sample repeats the last rate instead of reporting 0
// followed by a doubled rate.
func TestReportDerivedMetricsThroughputRateUsesSampleTimestamps(t *testing.T) {
	deviceAddress := "10.0.0.1"
	deviceID := buildDeviceIDFromConfigAddress(deviceAddress)
	cfg := throughputTestConfig(deviceAddress)
	tags := []string{
		deviceNamespaceTag,
		"device_ip:" + deviceAddress,
		"device_id:" + deviceID,
		"snmp_device:" + deviceAddress,
		"integration_source:gnmi",
		"interface:eth0",
	}
	// The check's clock must not influence the result.
	originalTimeNow := bandwidthTimeNow
	t.Cleanup(func() { bandwidthTimeNow = originalTimeNow })
	bandwidthTimeNow = func() time.Time { return time.Unix(999_999, 0) }

	state := NewBandwidthState()
	base := time.Unix(100, 0)
	runs := []struct {
		name     string
		value    uint64
		ts       time.Time
		expected []float64
	}{
		{name: "first sample: no rate yet", value: 1_000, ts: base, expected: nil},
		{name: "new sample 10s later", value: 3_000, ts: base.Add(10 * time.Second), expected: []float64{200}},
		{name: "same sample read again", value: 3_000, ts: base.Add(10 * time.Second), expected: []float64{200}},
		{name: "next sample", value: 7_000, ts: base.Add(30 * time.Second), expected: []float64{200}},
	}
	for _, run := range runs {
		mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
		mockSender.SetupAcceptAll()
		require.NoError(t, ReportDerivedMetrics(mockSender, cfg, inOctetsSample(run.value, run.ts), state), run.name)
		assert.Equal(t, run.expected, gaugeValues(mockSender, "snmp.ifHCInOctets.rate"), run.name)
		if run.expected != nil {
			mockSender.AssertMetric(t, "Gauge", "snmp.ifHCInOctets.rate", run.expected[0], "", tags)
		}
	}
}

func TestReportDerivedMetricsBandwidthUsageUsesSampleTimestamps(t *testing.T) {
	ifSpeed := uint64(1_000_000_000)
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.5"},
		Profile: config.ProfileDefinition{Metrics: []config.MetricConfig{
			{Path: pathInOctets, Metric: "snmp.ifHCInOctets", Tags: map[string]string{"interface": "name"}},
			{Path: pathPortSpeed, Metric: "snmp.ifInSpeed", Tags: map[string]string{"interface": "name"},
				ValueMap: map[string]int{"SPEED_1GB": int(ifSpeed)}},
		}},
	}
	snapshot := func(inOctets uint64, ts time.Time) []client.CachedValue {
		return []client.CachedValue{
			{Key: client.CacheKey{Path: pathInOctets, Keys: map[string]string{"name": "eth0"}},
				Entry: client.CacheEntry{Value: inOctets, Timestamp: ts}},
			{Key: client.CacheKey{Path: pathPortSpeed, Keys: map[string]string{"name": "eth0"}},
				Entry: client.CacheEntry{Value: "SPEED_1GB", Timestamp: ts}},
		}
	}
	originalTimeNow := bandwidthTimeNow
	t.Cleanup(func() { bandwidthTimeNow = originalTimeNow })
	bandwidthTimeNow = func() time.Time { return time.Unix(999_999, 0) }

	usage := func(octets float64) float64 { return ((octets * 8) / float64(ifSpeed)) * 100 }
	expected := (usage(3_000_000) - usage(1_500_000)) / 10.0

	state := NewBandwidthState()
	base := time.Unix(100, 0)
	for _, run := range []struct {
		octets uint64
		ts     time.Time
		want   []float64
	}{
		{octets: 1_500_000, ts: base},
		{octets: 3_000_000, ts: base.Add(10 * time.Second), want: []float64{expected}},
		{octets: 3_000_000, ts: base.Add(10 * time.Second), want: []float64{expected}}, // same sample read twice
	} {
		mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
		mockSender.SetupAcceptAll()
		require.NoError(t, ReportDerivedMetrics(mockSender, cfg, snapshot(run.octets, run.ts), state))
		assert.InDeltaSlice(t, run.want, gaugeValues(mockSender, metricBandwidthInUsage), 1e-9)
	}
}

func TestCounterRate(t *testing.T) {
	base := time.Unix(100, 0)

	t.Run("counter reset discards the rate", func(t *testing.T) {
		state := NewBandwidthState()
		_, err := state.counterRate("s", 0, 100, base)
		require.Error(t, err)
		_, err = state.counterRate("s", 0, 50, base.Add(10*time.Second))
		require.ErrorContains(t, err, "negative")
		// resumes from the post-reset sample
		rate, err := state.counterRate("s", 0, 150, base.Add(20*time.Second))
		require.NoError(t, err)
		assert.Equal(t, 10.0, rate)
	})

	t.Run("unchanged sample before any rate is skipped", func(t *testing.T) {
		state := NewBandwidthState()
		_, err := state.counterRate("s", 0, 100, base)
		require.Error(t, err)
		_, err = state.counterRate("s", 0, 100, base)
		require.ErrorIs(t, err, errNoNewSample)
	})

	t.Run("timestamp going backwards resets the series", func(t *testing.T) {
		state := NewBandwidthState()
		_, _ = state.counterRate("s", 0, 100, base)
		_, err := state.counterRate("s", 0, 200, base.Add(-time.Second))
		require.ErrorContains(t, err, "backwards")
		rate, err := state.counterRate("s", 0, 300, base.Add(9*time.Second))
		require.NoError(t, err)
		assert.Equal(t, 10.0, rate)
	})

	t.Run("scale change resets the series", func(t *testing.T) {
		state := NewBandwidthState()
		_, _ = state.counterRate("s", 1000, 100, base)
		_, err := state.counterRate("s", 2000, 200, base.Add(10*time.Second))
		require.ErrorContains(t, err, "ifSpeed changed")
	})

	t.Run("missing device timestamp falls back to the clock", func(t *testing.T) {
		originalTimeNow := bandwidthTimeNow
		t.Cleanup(func() { bandwidthTimeNow = originalTimeNow })
		state := NewBandwidthState()
		bandwidthTimeNow = func() time.Time { return base }
		_, _ = state.counterRate("s", 0, 100, time.Time{})
		bandwidthTimeNow = func() time.Time { return base.Add(5 * time.Second) }
		rate, err := state.counterRate("s", 0, 150, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, 10.0, rate)
	})
}

func TestEvaluateMemoryUsage(t *testing.T) {
	usage, err := evaluateMemoryUsage(25, 100)
	require.NoError(t, err)
	assert.Equal(t, 25.0, usage)
}
