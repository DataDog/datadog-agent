// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package providers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestNetworkPathRemoteConfigProviderIgnoresUnmarkedConfigs(t *testing.T) {
	provider := NewNetworkPathRemoteConfigProvider()
	statuses := map[string]state.ApplyStatus{}

	provider.ScheduleCallback(map[string]state.RawConfig{
		"datadog/2/DEBUG/other/config": {
			Config: []byte(`{"hello":"world"}`),
		},
	}, recordApplyStatuses(statuses))

	require.Empty(t, statuses)
	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Empty(t, configs)
}

func TestNetworkPathRemoteConfigProviderSchedulesMarkedConfigs(t *testing.T) {
	provider := NewNetworkPathRemoteConfigProvider()
	statuses := map[string]state.ApplyStatus{}
	configPath := "datadog/2/DEBUG/abc/network_path.json"

	provider.ScheduleCallback(map[string]state.RawConfig{
		configPath: {
			Config: []byte(`{
				"poc": "network_path_scheduled_tests",
				"type": "scheduled",
				"configs": [{
					"test_id": "test-123",
					"hostname": "api.example.com",
					"port": 443,
					"protocol": "tcp",
					"max_ttl": 30,
					"timeout_ms": 1000,
					"interval_sec": 60,
					"source_service": "frontend",
					"destination_service": "api",
					"tcp_method": "SYN",
					"traceroute_queries": 3,
					"e2e_queries": 50,
					"tags": ["env:prod"]
				}]
			}`),
			Metadata: state.Metadata{
				ID:      "abc",
				Name:    "network_path.json",
				Version: 7,
			},
		},
	}, recordApplyStatuses(statuses))

	require.Equal(t, state.ApplyStateAcknowledged, statuses[configPath].State)

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 1)

	cfg := configs[0]
	require.Equal(t, "network_path", cfg.Name)
	require.Equal(t, "remote_config_debug/network_path/abc/network_path.json", cfg.Source)
	require.Len(t, cfg.Instances, 1)

	instance := unmarshalNetworkPathInstance(t, cfg.Instances[0])
	require.Equal(t, "api.example.com", instance["hostname"])
	require.Equal(t, 443, instance["port"])
	require.Equal(t, "TCP", instance["protocol"])
	require.Equal(t, 30, instance["max_ttl"])
	require.Equal(t, 1000, instance["timeout"])
	require.Equal(t, 60, instance["min_collection_interval"])
	require.Equal(t, "frontend", instance["source_service"])
	require.Equal(t, "api", instance["destination_service"])
	require.Equal(t, "syn", instance["tcp_method"])
	require.Equal(t, 3, instance["traceroute_queries"])
	require.Equal(t, 50, instance["e2e_queries"])
	require.ElementsMatch(t, []interface{}{
		"env:prod",
		"network_path.test_id:test-123",
		"network_path.config_source:remote_config",
		"network_path.rc_product:debug",
		"network_path.rc_config_id:abc",
		"network_path.rc_config_version:7",
	}, instance["tags"])
}

func TestNetworkPathRemoteConfigProviderEmptyConfigsUnschedules(t *testing.T) {
	provider := NewNetworkPathRemoteConfigProvider()
	statuses := map[string]state.ApplyStatus{}
	configPath := "datadog/2/DEBUG/abc/network_path.json"

	provider.ScheduleCallback(map[string]state.RawConfig{
		configPath: {
			Config: []byte(`{
				"poc": "network_path_scheduled_tests",
				"type": "scheduled",
				"configs": [{"hostname": "api.example.com"}]
			}`),
			Metadata: state.Metadata{ID: "abc"},
		},
	}, recordApplyStatuses(statuses))

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 1)

	provider.ScheduleCallback(map[string]state.RawConfig{
		configPath: {
			Config: []byte(`{
				"poc": "network_path_scheduled_tests",
				"type": "scheduled",
				"configs": []
			}`),
			Metadata: state.Metadata{ID: "abc"},
		},
	}, recordApplyStatuses(statuses))

	require.Equal(t, state.ApplyStateAcknowledged, statuses[configPath].State)
	configs, err = provider.Collect(context.Background())
	require.NoError(t, err)
	require.Empty(t, configs)
}

func TestNetworkPathRemoteConfigProviderRejectsInvalidConfigAndKeepsLastValid(t *testing.T) {
	provider := NewNetworkPathRemoteConfigProvider()
	statuses := map[string]state.ApplyStatus{}
	configPath := "datadog/2/DEBUG/abc/network_path.json"

	provider.ScheduleCallback(map[string]state.RawConfig{
		configPath: {
			Config: []byte(`{
				"poc": "network_path_scheduled_tests",
				"type": "scheduled",
				"configs": [{"hostname": "api.example.com", "port": 443}]
			}`),
			Metadata: state.Metadata{ID: "abc"},
		},
	}, recordApplyStatuses(statuses))

	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 1)
	previous := configs[0]

	provider.ScheduleCallback(map[string]state.RawConfig{
		configPath: {
			Config: []byte(`{
				"poc": "network_path_scheduled_tests",
				"type": "scheduled",
				"configs": [{"hostname": "api.example.com", "port": 70000}]
			}`),
			Metadata: state.Metadata{ID: "abc"},
		},
	}, recordApplyStatuses(statuses))

	require.Equal(t, state.ApplyStateError, statuses[configPath].State)
	require.Contains(t, statuses[configPath].Error, "port must be between 1 and 65535")

	configs, err = provider.Collect(context.Background())
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.Equal(t, previous.FastDigest(), configs[0].FastDigest())
	require.NotEmpty(t, provider.GetConfigErrors()[configPath])
}

func TestNetworkPathRemoteConfigProviderRejectsUnsupportedType(t *testing.T) {
	provider := NewNetworkPathRemoteConfigProvider()
	statuses := map[string]state.ApplyStatus{}
	configPath := "datadog/2/DEBUG/abc/network_path.json"

	provider.ScheduleCallback(map[string]state.RawConfig{
		configPath: {
			Config: []byte(`{
				"poc": "network_path_scheduled_tests",
				"type": "dynamic",
				"configs": []
			}`),
			Metadata: state.Metadata{ID: "abc"},
		},
	}, recordApplyStatuses(statuses))

	require.Equal(t, state.ApplyStateError, statuses[configPath].State)
	require.Contains(t, statuses[configPath].Error, `unsupported Network Path DEBUG config type "dynamic"`)
	configs, err := provider.Collect(context.Background())
	require.NoError(t, err)
	require.Empty(t, configs)
}

func TestNetworkPathRemoteConfigProviderRejectsReservedTags(t *testing.T) {
	provider := NewNetworkPathRemoteConfigProvider()
	statuses := map[string]state.ApplyStatus{}
	configPath := "datadog/2/DEBUG/abc/network_path.json"

	provider.ScheduleCallback(map[string]state.RawConfig{
		configPath: {
			Config: []byte(`{
				"poc": "network_path_scheduled_tests",
				"type": "scheduled",
				"configs": [{
					"hostname": "api.example.com",
					"tags": ["network_path.rc_product:debug"]
				}]
			}`),
			Metadata: state.Metadata{ID: "abc"},
		},
	}, recordApplyStatuses(statuses))

	require.Equal(t, state.ApplyStateError, statuses[configPath].State)
	require.Contains(t, statuses[configPath].Error, "reserved Network Path RC prefix")
}

func recordApplyStatuses(statuses map[string]state.ApplyStatus) func(string, state.ApplyStatus) {
	return func(path string, status state.ApplyStatus) {
		statuses[path] = status
	}
}

func unmarshalNetworkPathInstance(t *testing.T, data integration.Data) map[string]interface{} {
	t.Helper()

	var instance map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &instance))
	return instance
}
