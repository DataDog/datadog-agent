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
				Keys: map[string]string{"component": "name"},
				Tags: map[string]string{"cpu": "name"},
			},
			{
				Path: "/openconfig/components/component/state/memory/utilized",
				Keys: map[string]string{"component": "name"},
				Tags: map[string]string{"memory": "name"},
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

func TestInterfaceNamesUsesEachMetricKeyMapping(t *testing.T) {
	metadata := config.DefaultOpenConfigMetadata()
	profile := config.ProfileDefinition{Metrics: []config.MetricConfig{{
		Path: "/vendor/ports/port/state/counter",
		Tags: map[string]string{"interface": "port_id"},
	}}}
	index := indexSnapshot([]client.CachedValue{{
		Key: client.CacheKey{
			Path: "/vendor/ports/port/state/counter",
			Keys: map[string]string{"port_id": "ethernet-1/9"},
		},
		Entry: client.CacheEntry{Value: uint64(42)},
	}})

	names := interfaceNames(index, metadata, interfaceMetricPathsFromProfile(profile))
	assert.Equal(t, []string{"ethernet-1/9"}, names)
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

func TestHostnameFallbackDoesNotGuessNativeSelectors(t *testing.T) {
	snapshot := []client.CachedValue{
		{Key: client.CacheKey{Path: "native:/system/config/hostname"}, Entry: client.CacheEntry{Value: "native-config"}},
		{Key: client.CacheKey{Path: "other:/system/state/hostname"}, Entry: client.CacheEntry{Value: "other-host"}},
	}
	metadata := config.MetadataConfig{Device: config.DeviceMetadataConfig{Hostname: "native:/system/state/hostname"}}
	require.Empty(t, ResolveDeviceHostname(snapshot, metadata))
	metadata.Device.Hostname = "openconfig:/system/state/hostname"
	require.Empty(t, ResolveDeviceHostname(snapshot, metadata), "native hostnames cannot fill missing OpenConfig metadata")
	snapshot = append(snapshot, client.CachedValue{Key: client.CacheKey{Path: "/system/config/hostname"}, Entry: client.CacheEntry{Value: "legacy"}})
	require.Equal(t, "legacy", ResolveDeviceHostname(snapshot, metadata))
}

func TestOpenConfigAliasesAreFiniteAndNativeModulesStayExact(t *testing.T) {
	for _, path := range []string{
		"native:/system/state/hostname",
		"native:/openconfig-system:system/state/hostname",
		"/vendor:system/state/hostname",
		"/unknown-root/state/hostname",
	} {
		require.Equal(t, []string{path}, snapshotPathAliases(path))
		require.False(t, snapshotPathMatches(path, "openconfig:/system/state/hostname"))
	}
}

func selectorCachedValue(t *testing.T, selector string, value any) client.CachedValue {
	t.Helper()
	key, err := client.SelectorKey(selector)
	require.NoError(t, err)
	return client.CachedValue{Key: key, Entry: client.CacheEntry{Value: value}}
}

func TestRepeatedSegmentPredicatesDoNotMatchWrongParent(t *testing.T) {
	selector := "native:/entry[name=A]/entry[name=B]/counter"
	snapshot := []client.CachedValue{
		selectorCachedValue(t, selector, 1),
		selectorCachedValue(t, "native:/entry[name=C]/entry[name=B]/counter", 2),
	}
	values := selectSnapshotValues(indexSnapshot(snapshot), selector)
	require.Len(t, values, 1)
	require.Equal(t, 1, values[0].Entry.Value)
	capture := NewCaptureSender()
	cfg := &config.CheckConfig{Profile: config.ProfileDefinition{Metrics: []config.MetricConfig{{Path: selector, Metric: "snmp.counter", Type: config.MetricTypeGauge}}}}
	require.NoError(t, ReportMetrics(capture, cfg, snapshot, nil))
	require.Len(t, capture.Metrics(), 1)
	require.Equal(t, 1.0, capture.Metrics()[0].Value)
}

func TestLookupMergesLiteralParentKeysAndRejectsConflicts(t *testing.T) {
	path := "native:/vrfs/vrf[vrf-id=blue]/interfaces/interface/state/description"
	cached := selectorCachedValue(t, "native:/vrfs/vrf[vrf-id=blue]/interfaces/interface[name=eth0]/state/description", "uplink")
	index := indexSnapshot([]client.CachedValue{cached})
	value, ok := lookupCachedValue(index, path, map[string]string{"name": "eth0"})
	require.True(t, ok)
	require.Equal(t, "uplink", value.Entry.Value)
	_, ok = lookupCachedValue(index, path, map[string]string{"vrf-id": "red", "name": "eth0"})
	require.False(t, ok)
}

func TestOpenConfigSelectorAliasesPreserveFullScopedIdentity(t *testing.T) {
	snapshot := []client.CachedValue{
		selectorCachedValue(t, "openconfig:/interfaces/entry[name=B]/entry/counter", 1),
		selectorCachedValue(t, "openconfig:/interfaces/entry/entry[name=B]/counter", 2),
		selectorCachedValue(t, "native:/interfaces/entry[name=B]/entry/counter", 99),
	}
	values := selectSnapshotValues(indexSnapshot(snapshot), "openconfig:/openconfig-interfaces:interfaces/entry/entry/counter")
	require.Len(t, values, 2, "one result per full scoped identity, not per flat key map or alias bucket")
	require.ElementsMatch(t, []any{1, 2}, []any{values[0].Entry.Value, values[1].Entry.Value})
}

func TestOpenConfigSelectorAliasesPreferExactPath(t *testing.T) {
	qualified := "openconfig:/openconfig-system:system/state/hostname"
	unqualified := "openconfig:/system/state/hostname"
	for _, selector := range []string{qualified, unqualified} {
		exact := selectorCachedValue(t, selector, "exact")
		aliasPath := unqualified
		if selector == unqualified {
			aliasPath = qualified
		}
		index := indexSnapshot([]client.CachedValue{selectorCachedValue(t, aliasPath, "alias"), exact})
		require.Equal(t, "exact", firstStringValue(index, selector, nil))
		value, ok := lookupCachedValue(index, selector, nil)
		require.True(t, ok)
		require.Equal(t, "exact", value.Entry.Value)
	}
}

func TestSnapshotPathMatchesAliasesOnBothSides(t *testing.T) {
	qualified := "openconfig:/openconfig-interfaces:interfaces/interface/state/name"
	unqualified := "openconfig:/interfaces/interface/state/name"
	require.True(t, snapshotPathMatches(unqualified, qualified))
	require.True(t, snapshotPathMatches(qualified, unqualified))
	require.False(t, snapshotPathMatches("native:/interfaces/interface/state/name", qualified))
	require.False(t, snapshotPathMatches("native:/interfaces/interface/state/name", "native:/openconfig-interfaces:interfaces/interface/state/name"))
}
