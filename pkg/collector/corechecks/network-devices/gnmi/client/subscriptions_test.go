// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
)

func TestStreamStateString(t *testing.T) {
	assert.Equal(t, "not_ready", StreamStateNotReady.String())
	assert.Equal(t, "connected", StreamStateConnected.String())
	assert.Equal(t, "reconnecting", StreamStateReconnecting.String())
	assert.Equal(t, "unknown", StreamState(99).String())
}

func TestSubscriptionSpecString(t *testing.T) {
	assert.Equal(t, "/system/state/hostname", SubscriptionSpec{Path: "/system/state/hostname"}.String())
	assert.Equal(t, "/interfaces/interface/state/admin-status{interface=name}", SubscriptionSpec{
		Path: "/interfaces/interface/state/admin-status",
		Keys: map[string]string{"interface": "name"},
	}.String())
}

func TestSubscriptionPaths(t *testing.T) {
	specs := SubscriptionPaths(Config{
		Profile: config.ProfileDefinition{
			Metrics: []config.MetricConfig{
				{
					Path:   "/system/state/uptime",
					Metric: "snmp.sysUpTime",
					Type:   config.MetricTypeGauge,
				},
			},
		},
	})
	require.NotEmpty(t, specs)
	assert.Equal(t, "/system/state/uptime", specs[0].Path)
}

func TestBuildSubscriptionSpecsDedupesPaths(t *testing.T) {
	cfg := Config{
		Profile: config.ProfileDefinition{
			Metrics: []config.MetricConfig{
				{
					Path:   "/interfaces/interface/state/admin-status",
					Metric: "snmp.ifAdminStatus",
					Type:   config.MetricTypeGauge,
					Tags:   map[string]string{"interface": "name"},
				},
			},
		},
	}

	specs := buildSubscriptionSpecs(cfg)
	paths := make(map[string]struct{})
	for _, spec := range specs {
		paths[spec.Path] = struct{}{}
	}

	_, hasAdminStatus := paths["/interfaces/interface/state/admin-status"]
	assert.True(t, hasAdminStatus)
	_, hasHostname := paths["/system/state/hostname"]
	assert.True(t, hasHostname)
}

func TestBuildSubscriptionSpecsIncludesTopologyWhenEnabled(t *testing.T) {
	cfg := Config{
		Profile: config.ProfileDefinition{
			Metrics: []config.MetricConfig{
				{
					Path:   "/system/state/uptime",
					Metric: "snmp.sysUpTime",
					Type:   config.MetricTypeGauge,
				},
			},
		},
		CollectTopology: true,
	}

	specs := buildSubscriptionSpecs(cfg)
	found := false
	for _, spec := range specs {
		if spec.Path == "/lldp/interfaces/interface/neighbors/neighbor/state/chassis-id" {
			found = true
			require.Equal(t, map[string]string{"interface": "name", "neighbor": "id"}, spec.Keys)
		}
	}
	assert.True(t, found)
}

func TestBuildSubscriptionSpecsExcludesTopologyByDefault(t *testing.T) {
	cfg := Config{
		Profile: config.ProfileDefinition{
			Metrics: []config.MetricConfig{
				{
					Path:   "/system/state/uptime",
					Metric: "snmp.sysUpTime",
					Type:   config.MetricTypeGauge,
				},
			},
		},
	}

	specs := buildSubscriptionSpecs(cfg)
	for _, spec := range specs {
		assert.NotContains(t, spec.Path, "/lldp/")
	}
}
