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

func TestDefaultStalenessThreshold(t *testing.T) {
	assert.Equal(t, 30*time.Second, DefaultStalenessThreshold(15*time.Second))
}

func TestFilterStale(t *testing.T) {
	now := time.Unix(100, 0)
	threshold := 20 * time.Second

	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{Path: "/fresh"},
			Entry: client.CacheEntry{
				Value:     int64(1),
				Timestamp: now.Add(-10 * time.Second),
			},
		},
		{
			Key: client.CacheKey{Path: "/stale"},
			Entry: client.CacheEntry{
				Value:     int64(2),
				Timestamp: now.Add(-30 * time.Second),
			},
		},
		{
			Key: client.CacheKey{Path: "/zero-timestamp"},
			Entry: client.CacheEntry{
				Value: int64(3),
			},
		},
	}

	filtered := FilterStale(snapshot, threshold, now)
	require.Len(t, filtered, 1)
	assert.Equal(t, "/fresh", filtered[0].Key.Path)
}

func TestFilterStaleDisabled(t *testing.T) {
	now := time.Unix(100, 0)
	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{Path: "/stale"},
			Entry: client.CacheEntry{
				Value:     int64(1),
				Timestamp: now.Add(-1 * time.Hour),
			},
		},
	}

	filtered := FilterStale(snapshot, 0, now)
	assert.Equal(t, snapshot, filtered)
}

func TestOldestSampleAgeSeconds(t *testing.T) {
	now := time.Unix(100, 0)
	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{Path: "/newer"},
			Entry: client.CacheEntry{
				Timestamp: now.Add(-5 * time.Second),
			},
		},
		{
			Key: client.CacheKey{Path: "/older"},
			Entry: client.CacheEntry{
				Timestamp: now.Add(-25 * time.Second),
			},
		},
	}

	assert.Equal(t, 25.0, OldestSampleAgeSeconds(snapshot, now))
	assert.Equal(t, 0.0, OldestSampleAgeSeconds(nil, now))
}

func TestReportHealth(t *testing.T) {
	deviceAddress := "10.0.0.1"
	baseTags := []string{
		"device_ip:" + deviceAddress,
		"device_id:default:" + deviceAddress,
	}

	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{
			Address: deviceAddress,
		},
	}

	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	stats := HealthStats{
		StreamState:      client.StreamStateConnected,
		ReconnectCount:   2,
		ReceivedSamples:  4,
		SampleAgeSeconds: 12.5,
	}
	require.NoError(t, ReportHealth(mockSender, cfg, stats))

	mockSender.AssertMetric(t, "Gauge", metricStreamState, streamStateConnected, "", baseTags)
	mockSender.AssertMetric(t, "Gauge", metricReconnectCount, 2, "", baseTags)
	mockSender.AssertMetric(t, "Gauge", metricReceivedSamples, 4, "", baseTags)
	mockSender.AssertMetric(t, "Gauge", metricSampleAgeSeconds, 12.5, "", baseTags)
}

func TestReportHealthValidation(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.1"},
	}
	stats := HealthStats{StreamState: client.StreamStateNotReady}

	t.Run("nil sender", func(t *testing.T) {
		err := ReportHealth(nil, cfg, stats)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sender is nil")
	})

	t.Run("nil config", func(t *testing.T) {
		mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
		err := ReportHealth(mockSender, nil, stats)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "check config is nil")
	})
}

func TestReportMetricsSkipsStaleValues(t *testing.T) {
	now := time.Unix(100, 0)
	deviceAddress := "10.0.0.1"
	baseTags := []string{
		"device_ip:" + deviceAddress,
		"device_id:default:" + deviceAddress,
	}

	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{
			Address:               deviceAddress,
			MinCollectionInterval: 15,
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
	}

	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{
				Path: "/openconfig/interfaces/interface/state/counters/in-octets",
				Keys: map[string]string{"name": "fresh"},
			},
			Entry: client.CacheEntry{
				Value:     uint64(42),
				Timestamp: now.Add(-10 * time.Second),
			},
		},
		{
			Key: client.CacheKey{
				Path: "/openconfig/interfaces/interface/state/counters/in-octets",
				Keys: map[string]string{"name": "stale"},
			},
			Entry: client.CacheEntry{
				Value:     uint64(99),
				Timestamp: now.Add(-1 * time.Hour),
			},
		},
	}

	freshSnapshot := FilterStale(snapshot, DefaultStalenessThreshold(15*time.Second), now)
	mockSender := mocksender.NewMockSender(t, checkid.ID("gnmi"))
	mockSender.SetupAcceptAll()

	require.NoError(t, ReportMetrics(mockSender, cfg, freshSnapshot, freshSnapshot))
	mockSender.AssertMetric(t, "MonotonicCount", "snmp.ifHCInOctets", 42, "", append(baseTags, "interface:fresh"))
	mockSender.AssertNumberOfCalls(t, "MonotonicCount", 1)
}
