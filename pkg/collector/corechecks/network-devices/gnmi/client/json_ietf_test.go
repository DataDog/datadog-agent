// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"testing"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/stretchr/testify/require"
)

func TestNormalizeGNMIPathCanonicalizesModulePrefixes(t *testing.T) {
	path := &gnmipb.Path{
		Elem: []*gnmipb.PathElem{
			{Name: "openconfig-interfaces:interfaces"},
			{Name: "interface", Key: map[string]string{"name": "ethernet-1/1"}},
			{Name: "state"},
			{Name: "counters"},
		},
	}

	normalized, keys := normalizeGNMIPath(path)
	require.Equal(t, "/openconfig/interfaces/interface/state/counters", normalized)
	require.Equal(t, map[string]string{"name": "ethernet-1/1"}, keys)
}

func TestNormalizeGNMIPathCanonicalizesNestedModulePrefixes(t *testing.T) {
	path := &gnmipb.Path{
		Elem: []*gnmipb.PathElem{
			{Name: "openconfig-interfaces:interfaces"},
			{Name: "interface", Key: map[string]string{"name": "ethernet-1/1"}},
			{Name: "openconfig-if-ethernet:ethernet"},
			{Name: "state"},
		},
	}

	normalized, keys := normalizeGNMIPath(path)
	require.Equal(t, "/openconfig/interfaces/interface/ethernet/state", normalized)
	require.Equal(t, map[string]string{"name": "ethernet-1/1"}, keys)
}

func TestFlattenJSONLeavesExpandsContainerMaps(t *testing.T) {
	now := time.Unix(123, 0)
	keys := map[string]string{"name": "ethernet-1/1"}
	value := map[string]any{
		"hostname": "srl2",
	}
	leaves := flattenJSONLeaves("/openconfig/system/state", nil, value, now)
	require.Len(t, leaves, 1)
	require.Equal(t, "/openconfig/system/state/hostname", leaves[0].Key.Path)
	require.Equal(t, "srl2", leaves[0].Entry.Value)

	counterValue := map[string]any{
		"in-octets":  uint64(42),
		"out-octets": uint64(84),
	}
	leaves = flattenJSONLeaves("/openconfig/interfaces/interface/state/counters", keys, counterValue, now)
	require.Len(t, leaves, 2)

	seen := make(map[string]any)
	for _, leaf := range leaves {
		require.Equal(t, keys, leaf.Key.Keys)
		seen[leaf.Key.Path] = leaf.Entry.Value
	}
	require.Equal(t, uint64(42), seen["/openconfig/interfaces/interface/state/counters/in-octets"])
	require.Equal(t, uint64(84), seen["/openconfig/interfaces/interface/state/counters/out-octets"])
}
