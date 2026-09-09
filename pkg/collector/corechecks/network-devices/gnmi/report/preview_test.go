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

	_, err := PreviewCollection(capture, cfg, gnmiClient, NewBandwidthState(), time.Time{}, now, opts)
	require.NoError(t, err)

	events := capture.MetadataEvents()
	require.NotEmpty(t, events)
	assert.Equal(t, "network-devices-metadata", events[0].EventType)
	assert.Contains(t, string(events[0].Payload), `"id":"default:srl1"`)
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
