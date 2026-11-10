// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
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

func TestPreviewExplicitOriginsIncludeMetadataAndTopology(t *testing.T) {
	for _, origin := range []string{"openconfig", "native"} {
		t.Run(origin, func(t *testing.T) {
			p := func(path string) string { return origin + ":" + path }
			metadata := config.MetadataConfig{
				Device: config.DeviceMetadataConfig{
					Keys:     map[string]string{"component": "name"},
					Hostname: p("/system/state/hostname"), ComponentType: p("/components/component/state/type"),
					Platform: p("/components/component/state/model-name"), SerialNumber: p("/components/component/state/serial-no"),
				},
				Interface: config.InterfaceMetadataConfig{
					Keys: map[string]string{"interface": "name"},
					Name: p("/interfaces/interface/state/name"), Description: p("/interfaces/interface/state/description"),
					AdminStatus: p("/interfaces/interface/state/admin-status"), OperStatus: p("/interfaces/interface/state/oper-status"),
				},
				IPAddress: config.IPAddressMetadataConfig{
					IP:           p("/interfaces/interface/subinterfaces/subinterface/ipv4/addresses/address/state/ip"),
					PrefixLength: p("/interfaces/interface/subinterfaces/subinterface/ipv4/addresses/address/state/prefix-length"),
				},
			}
			topology := config.TopologyConfig{LLDP: config.LLDPNeighborConfig{
				Keys:       map[string]string{"interface": "name", "neighbor": "id"},
				ChassisID:  p("/lldp/interfaces/interface/neighbors/neighbor/state/chassis-id"),
				SystemName: p("/lldp/interfaces/interface/neighbors/neighbor/state/system-name"),
				PortID:     p("/lldp/interfaces/interface/neighbors/neighbor/state/port-id"),
			}}
			cfg := &config.CheckConfig{Instance: config.InstanceConfig{Address: "router", MinCollectionInterval: 15, CollectTopology: true}, Profile: config.ProfileDefinition{
				Metadata: metadata, Topology: topology, Metrics: []config.MetricConfig{{
					Path: p("/interfaces/interface/state/counters/in-octets"), Metric: "snmp.ifHCInOctets", Type: config.MetricTypeMonotonicCount, Tags: map[string]string{"interface": "name"},
				}},
			}}
			now := time.Unix(1000, 0)
			var snapshot []client.CachedValue
			add := func(path string, keys map[string]string, value any) {
				snapshot = append(snapshot, client.CachedValue{Key: client.CacheKey{Path: path, Keys: keys}, Entry: client.CacheEntry{Value: value, Timestamp: now, ReceivedAt: now}})
				// Identical node and keys in another origin must not replace fields.
				foreign := "other:" + strings.TrimPrefix(path, origin+":")
				snapshot = append(snapshot, client.CachedValue{Key: client.CacheKey{Path: foreign, Keys: keys}, Entry: client.CacheEntry{Value: "wrong-origin", Timestamp: now, ReceivedAt: now}})
			}
			componentKeys := map[string]string{"name": "Chassis"}
			interfaceKeys := map[string]string{"name": "eth0"}
			neighborKeys := map[string]string{"name": "eth0", "id": "1"}
			add(metadata.Device.Hostname, nil, "router-host")
			add(metadata.Device.ComponentType, componentKeys, "CHASSIS")
			add(metadata.Device.Platform, componentKeys, "model")
			add(metadata.Device.SerialNumber, componentKeys, "serial")
			add(metadata.Interface.Name, interfaceKeys, "eth0")
			add(metadata.Interface.Description, interfaceKeys, "uplink")
			add(metadata.Interface.AdminStatus, interfaceKeys, "UP")
			add(metadata.Interface.OperStatus, interfaceKeys, "UP")
			add(metadata.IPAddress.IP, map[string]string{"name": "eth0", "index": "0", "ip": "192.0.2.1"}, "192.0.2.1")
			add(metadata.IPAddress.PrefixLength, map[string]string{"name": "eth0", "index": "0", "ip": "192.0.2.1"}, 24)
			add(topology.LLDP.ChassisID, neighborKeys, "remote-id")
			add(topology.LLDP.SystemName, neighborKeys, "remote-host")
			add(topology.LLDP.PortID, neighborKeys, "remote-port")
			add(cfg.Profile.Metrics[0].Path, interfaceKeys, 42)
			// Discovery must not invent inventory or neighbors from other origins.
			snapshot = append(snapshot,
				client.CachedValue{Key: client.CacheKey{Path: "other:/interfaces/interface/state/name", Keys: map[string]string{"name": "foreign"}}, Entry: client.CacheEntry{Value: "foreign"}},
				client.CachedValue{Key: client.CacheKey{Path: "other:/lldp/interfaces/interface/neighbors/neighbor/state/chassis-id", Keys: map[string]string{"name": "foreign", "id": "2"}}, Entry: client.CacheEntry{Value: "foreign"}},
			)
			capture := NewCaptureSender()
			_, err := PreviewCollection(capture, cfg, newPreviewTestClient(snapshot), NewBandwidthState(), time.Time{}, now, DefaultPreviewOptions())
			require.NoError(t, err)
			var profileMetric, status bool
			for _, metric := range capture.Metrics() {
				if metric.Name == "snmp.ifHCInOctets" {
					profileMetric = true
					require.Equal(t, 42.0, metric.Value)
					require.Contains(t, metric.Tags, "interface_alias:uplink")
					require.Contains(t, metric.Tags, "snmp_host:router-host")
				}
				if metric.Name == interfaceStatusMetric {
					status = true
					require.Contains(t, metric.Tags, "interface:eth0")
				}
			}
			require.True(t, profileMetric)
			require.True(t, status)
			require.Len(t, capture.MetadataEvents(), 1)
			var payload devicemetadata.NetworkDevicesMetadata
			require.NoError(t, json.Unmarshal(capture.MetadataEvents()[0].Payload, &payload))
			require.Len(t, payload.Devices, 1)
			require.Equal(t, "router-host", payload.Devices[0].OsHostname)
			require.Equal(t, "model", payload.Devices[0].ProductName)
			require.Equal(t, "serial", payload.Devices[0].SerialNumber)
			require.Len(t, payload.Interfaces, 1)
			require.Equal(t, "eth0", payload.Interfaces[0].Name)
			require.Len(t, payload.IPAddresses, 1)
			require.Equal(t, "192.0.2.1", payload.IPAddresses[0].IPAddress)
			require.EqualValues(t, 24, payload.IPAddresses[0].Prefixlen)
			require.Len(t, payload.Links, 1)
			require.Equal(t, "remote-host", payload.Links[0].Remote.Device.Name)
			require.Equal(t, "remote-port", payload.Links[0].Remote.Interface.ID)
		})
	}
}
