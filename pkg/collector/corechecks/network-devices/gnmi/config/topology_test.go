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

func TestDefaultOpenConfigLLDP(t *testing.T) {
	topology := DefaultOpenConfigLLDP()
	assert.Equal(t, "/openconfig/lldp/interfaces/interface/neighbors/neighbor/state/chassis-id", topology.LLDP.ChassisID)
	assert.Equal(t, map[string]string{"interface": "name", "neighbor": "id"}, topology.LLDP.Keys)
}

func TestTopologyConfigSubscriptionPaths(t *testing.T) {
	paths := DefaultOpenConfigLLDP().SubscriptionPaths()
	require.NotEmpty(t, paths)
	assert.Equal(t, map[string]string{"interface": "name", "neighbor": "id"}, paths[0].Tags)
}

func TestTopologyConfigIsZero(t *testing.T) {
	assert.True(t, TopologyConfig{}.IsZero())
	assert.False(t, DefaultOpenConfigLLDP().IsZero())
}
