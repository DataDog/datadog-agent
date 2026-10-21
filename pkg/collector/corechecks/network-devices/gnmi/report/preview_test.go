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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
)

func TestCaptureSenderRecordsMetrics(t *testing.T) {
	capture := NewCaptureSender()
	capture.Gauge("snmp.ifInOctets", 42, "", []string{"interface:eth0", "device_id:default:10.0.0.5"})
	capture.Rate("snmp.ifInOctets.rate", 7, "", []string{"interface:eth0"})

	metrics := capture.Metrics()
	require.Len(t, metrics, 2)
	assert.Equal(t, "gauge", metrics[0].Type)
	assert.Equal(t, "snmp.ifInOctets", metrics[0].Name)
	assert.Equal(t, 42.0, metrics[0].Value)
}

func TestFormatCapturedMetric(t *testing.T) {
	formatted := FormatCapturedMetric(CapturedMetric{
		Type:  "gauge",
		Name:  "snmp.ifInOctets",
		Value: 42,
		Tags:  []string{"device_id:default:10.0.0.5", "interface:eth0"},
	})
	assert.Equal(t, "gauge snmp.ifInOctets 42 host=- tags=[device_id:default:10.0.0.5,interface:eth0]", formatted)
}

func TestCaptureSenderRecordsMetadataEvent(t *testing.T) {
	capture := NewCaptureSender()
	payload := []byte(`{"devices":[{"id":"default:srl1"}]}`)
	capture.EventPlatformEvent(payload, "network-devices-metadata")

	events := capture.MetadataEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "network-devices-metadata", events[0].EventType)
	assert.JSONEq(t, string(payload), string(events[0].Payload))
}

func TestFormatCapturedMetadataEvent(t *testing.T) {
	formatted := FormatCapturedMetadataEvent(CapturedMetadataEvent{
		EventType: "network-devices-metadata",
		Payload:   []byte(`{"devices":[{"id":"default:srl1"}]}`),
	})
	assert.Contains(t, formatted, "metadata event_type=network-devices-metadata")
	assert.Contains(t, formatted, `"id": "default:srl1"`)
}

func TestPreviewCollectionEmitsProfileMetrics(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{
			Address:               "10.0.0.5",
			MinCollectionInterval: 15,
		},
		Profile: config.ProfileDefinition{
			Name: "interface-stats",
			Metrics: []config.MetricConfig{
				{
					Path:   "/openconfig/interfaces/interface/state/counters/in-octets",
					Metric: "snmp.ifInOctets",
					Type:   config.MetricTypeGauge,
					Tags:   map[string]string{"interface": "name"},
				},
			},
		},
	}

	now := time.Unix(1000, 0)
	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{
				Path: "/openconfig/interfaces/interface/state/counters/in-octets",
				Keys: map[string]string{"name": "eth0"},
			},
			Entry: client.CacheEntry{Value: int64(1234), Timestamp: now},
		},
	}

	gnmiClient := newPreviewTestClient(snapshot)
	capture := NewCaptureSender()
	opts := PreviewOptions{
		IncludeHealth:          false,
		IncludeInterfaceStatus: false,
	}

	_, err := PreviewCollection(capture, cfg, gnmiClient, NewBandwidthState(), time.Time{}, now, opts)
	require.NoError(t, err)

	metrics := capture.Metrics()
	require.NotEmpty(t, metrics)
	assert.Equal(t, "snmp.ifInOctets", metrics[0].Name)
	assert.Contains(t, metrics[0].Tags, "interface:eth0")
}

func TestPreviewCollectionEmitsMetadata(t *testing.T) {
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{
			Address:               "srl1",
			MinCollectionInterval: 15,
		},
		Profile: config.ProfileDefinition{
			Name:     "interface-stats",
			Metadata: config.DefaultOpenConfigMetadata(),
		},
	}

	now := time.Unix(1000, 0)
	snapshot := []client.CachedValue{
		{
			Key:   client.CacheKey{Path: config.DefaultOpenConfigMetadata().Device.Hostname},
			Entry: client.CacheEntry{Value: "router-1", Timestamp: now},
		},
		{
			Key:   client.CacheKey{Path: "/openconfig/interfaces/interface/state/name", Keys: map[string]string{"name": "eth0"}},
			Entry: client.CacheEntry{Value: "eth0", Timestamp: now},
		},
	}

	gnmiClient := newPreviewTestClient(snapshot)
	capture := NewCaptureSender()
	opts := PreviewOptions{
		IncludeHealth:          false,
		IncludeInterfaceStatus: false,
		IncludeMetadata:        true,
	}

	lastMetadataReport, err := PreviewCollection(capture, cfg, gnmiClient, NewBandwidthState(), time.Time{}, now, opts)
	require.NoError(t, err)

	events := capture.MetadataEvents()
	require.NotEmpty(t, events)
	assert.Equal(t, "network-devices-metadata", events[0].EventType)
	assert.Contains(t, string(events[0].Payload), `"id":"default:srl1"`)

	unchanged, err := PreviewCollection(capture, cfg, gnmiClient, NewBandwidthState(), lastMetadataReport, now.Add(time.Minute), opts)
	require.NoError(t, err)
	assert.Equal(t, lastMetadataReport, unchanged)
	assert.Empty(t, capture.MetadataEvents())

	advanced, err := PreviewCollection(capture, cfg, gnmiClient, NewBandwidthState(), unchanged, now.Add(11*time.Minute), opts)
	require.NoError(t, err)
	assert.Equal(t, now.Add(11*time.Minute), advanced)
	assert.NotEmpty(t, capture.MetadataEvents())
}

func TestPreviewCollectionMatchesCheckCollection(t *testing.T) {
	now := time.Unix(1000, 0)
	cfg := &config.CheckConfig{
		Instance: config.InstanceConfig{Address: "10.0.0.5", MinCollectionInterval: 15},
		Profile: config.ProfileDefinition{
			Metadata: config.DefaultOpenConfigMetadata(),
			Metrics: []config.MetricConfig{{
				Path: "/openconfig/interfaces/interface/state/ifindex", Metric: "snmp.interface.ifindex",
				Tags: map[string]string{"interface": "name"},
			}},
		},
	}
	snapshot := []client.CachedValue{
		{Key: client.CacheKey{Path: cfg.Profile.Metadata.Device.Hostname}, Entry: client.CacheEntry{Value: "router-1", Timestamp: now}},
		{Key: client.CacheKey{Path: cfg.Profile.Metadata.Interface.Name, Keys: map[string]string{"name": "eth0"}}, Entry: client.CacheEntry{Value: "eth0", Timestamp: now}},
		{Key: client.CacheKey{Path: cfg.Profile.Metadata.Interface.IfIndex, Keys: map[string]string{"name": "eth0"}}, Entry: client.CacheEntry{Value: int32(42), Timestamp: now}},
	}
	gnmiClient := newPreviewTestClient(snapshot)
	checkSender, previewSender := NewCaptureSender(), NewCaptureSender()
	checkTime, err := CollectSnapshot(checkSender, cfg, gnmiClient, snapshot, NewBandwidthState(), time.Time{}, now, DefaultPreviewOptions())
	require.NoError(t, err)
	previewTime, err := PreviewCollection(previewSender, cfg, gnmiClient, NewBandwidthState(), time.Time{}, now, DefaultPreviewOptions())
	require.NoError(t, err)
	assert.Equal(t, checkTime, previewTime)
	assert.Equal(t, checkSender.Metrics(), previewSender.Metrics())
	assert.Equal(t, checkSender.MetadataEvents(), previewSender.MetadataEvents())
	assert.NotEmpty(t, checkSender.Metrics())
	assert.NotEmpty(t, checkSender.MetadataEvents())
}

func TestPreviewCollectionMetricPrefix(t *testing.T) {
	for _, prefix := range []string{"", "snmp", "gnmi"} {
		t.Run(prefix, func(t *testing.T) {
			cfg := throughputTestConfig("10.0.0.5")
			cfg.Instance.MetricPrefix = prefix
			cfg.Instance.MinCollectionInterval = 15
			cfg.Profile.Metadata = config.DefaultOpenConfigMetadata()
			cfg.Profile.Metrics = append(cfg.Profile.Metrics,
				config.MetricConfig{Path: pathPortSpeed, Metric: "snmp.ifInSpeed", Type: config.MetricTypeGauge, Tags: map[string]string{"interface": "name"}},
				config.MetricConfig{Path: pathMemoryUsed, Metric: "snmp.memory.used", Type: config.MetricTypeGauge},
				config.MetricConfig{Path: pathMemoryAvail, Metric: "snmp.memory.free", Type: config.MetricTypeGauge},
				config.MetricConfig{Path: "/custom", Metric: "custom.metric", Type: config.MetricTypeGauge},
			)
			start := time.Unix(1000, 0)
			snapshot := func(value uint64, ts time.Time) []client.CachedValue {
				return append(inOctetsSample(value, ts),
					client.CachedValue{Key: client.CacheKey{Path: pathPortSpeed, Keys: map[string]string{"name": "eth0"}}, Entry: client.CacheEntry{Value: uint64(1_000_000_000), Timestamp: ts}},
					client.CachedValue{Key: client.CacheKey{Path: pathMemoryUsed, Keys: map[string]string{"name": "Chassis"}}, Entry: client.CacheEntry{Value: 75, Timestamp: ts}},
					client.CachedValue{Key: client.CacheKey{Path: pathMemoryAvail, Keys: map[string]string{"name": "Chassis"}}, Entry: client.CacheEntry{Value: 25, Timestamp: ts}},
					client.CachedValue{Key: client.CacheKey{Path: "/custom"}, Entry: client.CacheEntry{Value: 42, Timestamp: ts}},
				)
			}
			capture := NewCaptureSender()
			state := NewBandwidthState()
			_, err := PreviewCollection(capture, cfg, newPreviewTestClient(snapshot(100, start)), state, time.Time{}, start, DefaultPreviewOptions())
			require.NoError(t, err)
			now := start.Add(15 * time.Second)
			_, err = PreviewCollection(capture, cfg, newPreviewTestClient(snapshot(200, now)), state, time.Time{}, now, DefaultPreviewOptions())
			require.NoError(t, err)

			expectedPrefix := prefix
			if expectedPrefix == "" {
				expectedPrefix = "snmp"
			}
			names := make([]string, 0)
			for _, metric := range capture.Metrics() {
				names = append(names, metric.Name)
				assert.Contains(t, metric.Tags, "integration_source:gnmi")
				if prefix == "gnmi" {
					assert.NotContains(t, metric.Name, "snmp.")
				}
			}
			for _, suffix := range []string{"ifHCInOctets", "ifHCInOctets.rate", "ifInSpeed", "ifBandwidthInUsage.rate", "memory.used", "memory.free", "memory.usage", "interface.status"} {
				assert.Contains(t, names, expectedPrefix+"."+suffix)
			}
			assert.Contains(t, names, "datadog.gnmi.stream_state")
			assert.Contains(t, names, "custom.metric")
			assert.Equal(t, "snmp.ifHCInOctets", cfg.Profile.Metrics[0].Metric)
		})
	}
}

func TestPreviewCollectionPrunesStateWithNoFreshSamples(t *testing.T) {
	now := time.Now()
	cfg := &config.CheckConfig{Instance: config.InstanceConfig{Address: "10.0.0.5", MinCollectionInterval: 15}}
	state := NewBandwidthState()
	state["disappeared"] = &bandwidthUsage{lastSeen: now.Add(-46 * time.Second)}
	_, err := PreviewCollection(NewCaptureSender(), cfg, newPreviewTestClient(nil), state, time.Time{}, now,
		PreviewOptions{IncludeHealth: true})
	require.NoError(t, err)
	assert.NotContains(t, state, "disappeared")
}

type previewTestClient struct {
	snapshot []client.CachedValue
}

func newPreviewTestClient(snapshot []client.CachedValue) *previewTestClient {
	return &previewTestClient{snapshot: snapshot}
}

func (c *previewTestClient) Snapshot() []client.CachedValue {
	return c.snapshot
}

func (c *previewTestClient) StreamState() client.StreamState {
	return client.StreamStateConnected
}

func (c *previewTestClient) Synchronized() bool {
	return true
}

func (c *previewTestClient) ReconnectAttempts() int {
	return 0
}

func (c *previewTestClient) ReceivedSamples() int {
	return len(c.snapshot)
}
