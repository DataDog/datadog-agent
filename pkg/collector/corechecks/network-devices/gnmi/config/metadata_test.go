// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultOpenConfigMetadata(t *testing.T) {
	metadata := DefaultOpenConfigMetadata()
	assert.Equal(t, "/openconfig/system/state/hostname", metadata.Device.Hostname)
	assert.Equal(t, "/openconfig/interfaces/interface/state/ifindex", metadata.Interface.IfIndex)
	assert.Equal(t, map[string]string{"interface": "name"}, metadata.Interface.Keys)
}

func TestMetadataConfigResolvedUsesDefaults(t *testing.T) {
	metadata := MetadataConfig{}.Resolved()
	assert.Equal(t, DefaultOpenConfigMetadata(), metadata)
}

func TestMetadataConfigSubscriptionPaths(t *testing.T) {
	paths := DefaultOpenConfigMetadata().SubscriptionPaths()
	require.NotEmpty(t, paths)

	seen := make(map[string]PathSubscriptionConfig)
	for _, path := range paths {
		seen[path.Path] = path
	}

	assert.Contains(t, seen, "/openconfig/system/state/hostname")
	assert.Equal(t, map[string]string(nil), seen["/openconfig/system/state/hostname"].Tags)
	assert.Equal(t, map[string]string{"component": "name"}, seen["/openconfig/components/component/state/mfg-name"].Tags)
	assert.Equal(t, map[string]string{"interface": "name"}, seen["/openconfig/interfaces/interface/state/ifindex"].Tags)
	ipPath := "/openconfig/interfaces/interface/subinterfaces/subinterface/ipv4/addresses/address/state/ip"
	assert.Contains(t, seen, ipPath)
	assert.Equal(t, map[string]string{"interface": "name", "subinterface": "index", "address": "ip"}, seen[ipPath].Tags)

	ipv6Path := "/openconfig/interfaces/interface/subinterfaces/subinterface/ipv6/addresses/address/state/ip"
	assert.Contains(t, seen, ipv6Path)
	assert.Equal(t, map[string]string{"interface": "name", "subinterface": "index", "address": "ip"}, seen[ipv6Path].Tags)
}

func TestInterfaceKeyValues(t *testing.T) {
	keys := DefaultOpenConfigMetadata().InterfaceKeyValues("Ethernet1")
	assert.Equal(t, map[string]string{"name": "Ethernet1"}, keys)
}

func TestValidateMetadataConfigRequiresIfIndex(t *testing.T) {
	err := validateMetadataConfig(MetadataConfig{
		Interface: InterfaceMetadataConfig{
			Keys: map[string]string{"interface": "name"},
			Name: "/openconfig/interfaces/interface/state/name",
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ifindex")
}
