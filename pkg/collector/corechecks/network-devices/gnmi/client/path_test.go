// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"testing"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
)

func TestSubscribePathFromMetric(t *testing.T) {
	metric := config.MetricConfig{
		Path: "/openconfig/interfaces/interface/state/counters/in-octets",
		Tags: map[string]string{
			"interface": "name",
		},
	}

	path, err := subscribePathFromMetric(metric)
	require.NoError(t, err)
	require.Equal(t, "openconfig", path.GetOrigin())
	require.Len(t, path.GetElem(), 5)
	require.Equal(t, "interfaces", path.GetElem()[0].GetName())
	require.Equal(t, "interface", path.GetElem()[1].GetName())
	require.Equal(t, map[string]string{"name": "*"}, path.GetElem()[1].GetKey())
}

func TestCacheKeyFromGNMIPath(t *testing.T) {
	path := &gnmipb.Path{
		Elem: []*gnmipb.PathElem{
			{Name: "openconfig"},
			{Name: "interfaces"},
			{Name: "interface", Key: map[string]string{"name": "eth0"}},
			{Name: "state"},
			{Name: "counters"},
			{Name: "in-octets"},
		},
	}

	key := cacheKeyFromGNMIPath(path)
	require.Equal(t, "/openconfig/interfaces/interface/state/counters/in-octets", key.Path)
	require.Equal(t, map[string]string{"name": "eth0"}, key.Keys)
}

func TestCacheKeyEncodingDoesNotCollide(t *testing.T) {
	combinedValue := CacheKey{Path: "/interfaces/interface", Keys: map[string]string{"a": "b,c=d"}}
	separateValues := CacheKey{Path: "/interfaces/interface", Keys: map[string]string{"a": "b", "c": "d"}}
	require.NotEqual(t, combinedValue.cacheKey(), separateValues.cacheKey())
}

func TestNormalizedPathMatchesKeyedPrefix(t *testing.T) {
	prefix := normalizedPathFromGNMIPath(&gnmipb.Path{Elem: []*gnmipb.PathElem{
		{Name: "interfaces"},
		{Name: "interface", Key: map[string]string{"name": "eth0"}},
	}})
	matching := normalizedPathFromGNMIPath(&gnmipb.Path{Elem: []*gnmipb.PathElem{
		{Name: "interfaces"},
		{Name: "interface", Key: map[string]string{"name": "eth0"}},
		{Name: "state"},
	}})
	nonMatchingKey := normalizedPathFromGNMIPath(&gnmipb.Path{Elem: []*gnmipb.PathElem{
		{Name: "interfaces"},
		{Name: "interface", Key: map[string]string{"name": "eth1"}},
		{Name: "state"},
	}})
	nonMatchingSegment := normalizedPathFromGNMIPath(&gnmipb.Path{Elem: []*gnmipb.PathElem{
		{Name: "interfaces"},
		{Name: "interface-state"},
	}})

	require.True(t, matching.matchesPrefix(prefix))
	require.False(t, nonMatchingKey.matchesPrefix(prefix))
	require.False(t, nonMatchingSegment.matchesPrefix(prefix))
}

func TestNormalizedPathMatchesOriginAndTarget(t *testing.T) {
	path := normalizedPathFromGNMIPath(&gnmipb.Path{
		Origin: "openconfig",
		Target: "router-a",
		Elem:   []*gnmipb.PathElem{{Name: "interfaces"}},
	})

	require.True(t, path.matchesPrefix(normalizedPathFromGNMIPath(&gnmipb.Path{
		Origin: "openconfig",
		Target: "router-a",
		Elem:   []*gnmipb.PathElem{{Name: "interfaces"}},
	})))
	require.False(t, path.matchesPrefix(normalizedPathFromGNMIPath(&gnmipb.Path{
		Origin: "vendor",
		Elem:   []*gnmipb.PathElem{{Name: "interfaces"}},
	})))
	require.False(t, path.matchesPrefix(normalizedPathFromGNMIPath(&gnmipb.Path{
		Target: "router-b",
		Elem:   []*gnmipb.PathElem{{Name: "interfaces"}},
	})))
}

func TestCacheKeyFromGNMIPathModulePrefixedSegments(t *testing.T) {
	path := &gnmipb.Path{
		Elem: []*gnmipb.PathElem{
			{Name: "openconfig-system:system"},
			{Name: "state"},
		},
	}

	key := cacheKeyFromGNMIPath(path)
	require.Equal(t, "/openconfig-system:system/state", key.Path)
}

func TestScopedLookupKeysAndLegacySnapshots(t *testing.T) {
	actual, err := SelectorKey("native:/entry[name=A]/entry[name=B]/counter")
	require.NoError(t, err)
	parent, err := SelectorKey("native:/entry[name=A]/entry/counter")
	require.NoError(t, err)
	require.True(t, actual.MatchesLookupKeys(parent, map[string]string{"name": "B"}))
	require.False(t, actual.MatchesLookupKeys(parent, map[string]string{"name": "C"}))
	wildcard, err := SelectorKey("native:/entry[name=A]/entry[name=*]/counter")
	require.NoError(t, err)
	require.True(t, actual.MatchesSelectorKeys(wildcard))
	legacy := CacheKey{Path: actual.Path, Keys: actual.Keys}
	require.False(t, legacy.MatchesSelectorKeys(actual), "an old flat snapshot cannot establish both segment predicates")
	unique, err := SelectorKey("native:/vrf[vrf-id=blue]/interface[name=eth0]/counter")
	require.NoError(t, err)
	legacy = CacheKey{Path: unique.Path, Keys: unique.Keys}
	require.True(t, legacy.MatchesLookupKeys(unique, nil), "unambiguous old flat snapshots still work")
}
