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
		Path: "/interfaces/interface/state/counters/in-octets",
		Tags: map[string]string{
			"interface": "name",
		},
	}

	path, err := subscribePathFromMetric(metric)
	require.NoError(t, err)
	require.Len(t, path.GetElem(), 5)
	require.Equal(t, "interface", path.GetElem()[1].GetName())
	require.Equal(t, map[string]string{"name": "*"}, path.GetElem()[1].GetKey())
}

func TestCacheKeyFromGNMIPath(t *testing.T) {
	path := &gnmipb.Path{
		Elem: []*gnmipb.PathElem{
			{Name: "interfaces"},
			{Name: "interface", Key: map[string]string{"name": "eth0"}},
			{Name: "state"},
			{Name: "counters"},
			{Name: "in-octets"},
		},
	}

	key := cacheKeyFromGNMIPath(path)
	require.Equal(t, "/interfaces/interface/state/counters/in-octets", key.Path)
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
	require.Equal(t, "/openconfig/system/state", key.Path)
}
