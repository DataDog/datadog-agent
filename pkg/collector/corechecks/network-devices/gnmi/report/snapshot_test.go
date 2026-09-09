// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
)

func TestFirstStringValueIsDeterministic(t *testing.T) {
	path := "/openconfig/components/component/state/model-name"
	index := indexSnapshot([]client.CachedValue{
		{
			Key:   client.CacheKey{Path: path, Keys: map[string]string{"name": "FanTray3"}},
			Entry: client.CacheEntry{Value: "fan-model"},
		},
		{
			Key:   client.CacheKey{Path: path, Keys: map[string]string{"name": "Chassis"}},
			Entry: client.CacheEntry{Value: "chassis-model"},
		},
	})

	assert.Equal(t, "chassis-model", firstStringValue(index, path, map[string]string{"name": "Chassis"}))
}

func TestPathsShareParent(t *testing.T) {
	parent, ok := pathsShareParent(
		"/openconfig/components/component/state/memory/utilized",
		"/openconfig/components/component/state/memory/available",
	)
	assert.True(t, ok)
	assert.Equal(t, "/openconfig/components/component/state/memory", parent)

	_, ok = pathsShareParent(
		"/openconfig/components/component/state/memory/utilized",
		"/openconfig/components/component/state/cpu/utilized",
	)
	assert.False(t, ok)
}

func TestLookupCachedValueRequiresExactKeys(t *testing.T) {
	path := "/openconfig/components/component/state/memory/utilized"
	index := indexSnapshot([]client.CachedValue{
		{
			Key:   client.CacheKey{Path: path, Keys: map[string]string{"name": "ControlA"}},
			Entry: client.CacheEntry{Value: int64(25)},
		},
		{
			Key:   client.CacheKey{Path: path, Keys: map[string]string{"name": "FanTray3"}},
			Entry: client.CacheEntry{Value: int64(10)},
		},
	})

	cached, ok := lookupCachedValue(index, path, map[string]string{"name": "ControlA"})
	require.True(t, ok)
	value, ok := int64Value(cached.Entry.Value)
	require.True(t, ok)
	assert.Equal(t, int64(25), value)

	_, ok = lookupCachedValue(index, path, map[string]string{"name": "FanTray6"})
	assert.False(t, ok)
}

func TestResolveDeviceComponentNamePrefersChassis(t *testing.T) {
	metadata := config.DefaultOpenConfigMetadata()
	index := indexSnapshot([]client.CachedValue{
		{
			Key:   client.CacheKey{Path: metadata.Device.Platform, Keys: map[string]string{"name": "FanTray3"}},
			Entry: client.CacheEntry{Value: "fan-model"},
		},
		{
			Key:   client.CacheKey{Path: metadata.Device.Platform, Keys: map[string]string{"name": "Chassis"}},
			Entry: client.CacheEntry{Value: "chassis-model"},
		},
	})

	assert.Equal(t, "Chassis", resolveDeviceComponentName(index, metadata))
}

func TestInterfaceNamesIgnoresComponentMetricPaths(t *testing.T) {
	metadata := config.DefaultOpenConfigMetadata()
	index := indexSnapshot([]client.CachedValue{
		{
			Key: client.CacheKey{
				Path: "/openconfig/interfaces/interface/ethernet/state/port-speed",
				Keys: map[string]string{"name": "ethernet-1/1"},
			},
			Entry: client.CacheEntry{Value: "SPEED_1GB"},
		},
		{
			Key: client.CacheKey{
				Path: "/openconfig/components/component/cpu/utilization/state/instant",
				Keys: map[string]string{"name": "ControlA"},
			},
			Entry: client.CacheEntry{Value: 12.5},
		},
		{
			Key: client.CacheKey{
				Path: "/openconfig/components/component/state/memory/utilized",
				Keys: map[string]string{"name": "CPU-ControlA"},
			},
			Entry: client.CacheEntry{Value: int64(100)},
		},
	})

	profile := config.ProfileDefinition{
		Metrics: []config.MetricConfig{
			{
				Path: "/openconfig/interfaces/interface/ethernet/state/port-speed",
				Tags: map[string]string{"interface": "name"},
			},
			{
				Path: "/openconfig/components/component/cpu/utilization/state/instant",
				Keys:   map[string]string{"component": "name"},
				Tags:   map[string]string{"cpu": "name"},
			},
			{
				Path: "/openconfig/components/component/state/memory/utilized",
				Keys:   map[string]string{"component": "name"},
				Tags:   map[string]string{"memory": "name"},
			},
		},
	}

	names := interfaceNames(index, metadata, interfaceMetricPathsFromProfile(profile))
	assert.Equal(t, []string{"ethernet-1/1"}, names)

	snapshot := []client.CachedValue{
		{
			Key: client.CacheKey{
				Path: "/openconfig/interfaces/interface/ethernet/state/port-speed",
				Keys: map[string]string{"name": "ethernet-1/1"},
			},
			Entry: client.CacheEntry{Value: "SPEED_1GB"},
		},
		{
			Key: client.CacheKey{
				Path: "/openconfig/components/component/cpu/utilization/state/instant",
				Keys: map[string]string{"name": "ControlA"},
			},
			Entry: client.CacheEntry{Value: 12.5},
		},
	}
	interfaces := buildInterfaceMetadata("default:srl2", metadata, snapshot, interfaceMetricPathsFromProfile(profile))
	require.Len(t, interfaces, 1)
	assert.Equal(t, "ethernet-1/1", interfaces[0].Name)
}

func TestComponentNamesIgnoresInterfacePaths(t *testing.T) {
	metadata := config.DefaultOpenConfigMetadata()
	index := indexSnapshot([]client.CachedValue{
		{
			Key: client.CacheKey{
				Path: metadata.Device.Platform,
				Keys: map[string]string{"name": "Chassis"},
			},
			Entry: client.CacheEntry{Value: "7220 IXR"},
		},
		{
			Key: client.CacheKey{
				Path: "/openconfig/interfaces/interface/state/name",
				Keys: map[string]string{"name": "ethernet-1/1"},
			},
			Entry: client.CacheEntry{Value: "ethernet-1/1"},
		},
	})

	assert.Equal(t, []string{"Chassis"}, componentNames(index, metadata))
}
