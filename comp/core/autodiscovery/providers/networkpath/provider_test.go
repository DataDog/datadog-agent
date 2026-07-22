// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkpath

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	networkpathcheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/networkpath"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestProviderValidScheduledConfig(t *testing.T) {
	provider := NewProvider()
	changesCh := provider.Stream(context.Background())
	assert.Empty(t, <-changesCh)

	statuses := applyStatuses()
	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: []byte(`{
			"type": "scheduled",
			"test_config_id": "test-config-a",
			"unknown_root_field": true,
			"config": {
				"unknown_config_field": true,
				"tests": [
					{
						"hostname": "api.example.com",
						"port": 443,
						"protocol": "tcp",
						"interval_sec": 60,
						"timeout_ms": 1000,
						"max_ttl": 30,
						"tcp_method": "syn",
						"traceroute_queries": 3,
						"e2e_queries": 50,
						"source_service": "frontend",
						"destination_service": "api",
						"tags": ["env:prod"],
						"unknown_endpoint_field": "ignored"
					},
					{"hostname": "db.example.com"}
				]
			}
		}`)},
	}, statuses.callback)

	assert.Equal(t, state.ApplyStateAcknowledged, statuses.values["path/a"].State)
	assert.Empty(t, provider.GetConfigErrors())

	changes := <-changesCh
	assert.Empty(t, changes.Unschedule)
	require.Len(t, changes.Schedule, 2)

	first := changes.Schedule[0]
	assert.Equal(t, networkpathcheck.CheckName, first.Name)
	assert.Equal(t, configSource, first.Source)
	assert.Equal(t, names.NetworkPathRemoteConfig, provider.String())
	require.Len(t, first.Instances, 1)

	instance := unmarshalInstance(t, first.Instances[0])
	assert.Equal(t, "test-config-a", instance["test_config_id"])
	assert.Equal(t, "api.example.com", instance["hostname"])
	assert.Equal(t, 443, instance["port"])
	assert.Equal(t, "TCP", instance["protocol"])
	assert.Equal(t, 60, instance["min_collection_interval"])
	assert.Equal(t, 30, instance["timeout"])
	assert.Equal(t, 30, instance["max_ttl"])
	assert.Equal(t, "syn", instance["tcp_method"])
	assert.Equal(t, 3, instance["traceroute_queries"])
	assert.Equal(t, 50, instance["e2e_queries"])
	assert.Equal(t, "frontend", instance["source_service"])
	assert.Equal(t, "api", instance["destination_service"])
	assert.Equal(t, []interface{}{"env:prod"}, instance["tags"])

	second := unmarshalInstance(t, changes.Schedule[1].Instances[0])
	assert.Equal(t, "test-config-a", second["test_config_id"])
	assert.Equal(t, "db.example.com", second["hostname"])
}

func TestProviderNoOpSnapshotDoesNotRestartChecks(t *testing.T) {
	provider := NewProvider()
	changesCh := provider.Stream(context.Background())
	assert.Empty(t, <-changesCh)

	config := rawScheduledConfig("test-config-a", `{"hostname":"api.example.com"}`)
	provider.Update(map[string]state.RawConfig{"path/a": {Config: config}}, applyStatuses().callback)
	first := <-changesCh
	require.Len(t, first.Schedule, 1)

	provider.Update(map[string]state.RawConfig{"path/a": {Config: config}}, applyStatuses().callback)
	assertNoChanges(t, changesCh)
}

func TestProviderConvertsTotalTimeoutToPerHop(t *testing.T) {
	tests := []struct {
		name            string
		endpoint        string
		expectedTimeout int
	}{
		{
			name:            "exact milliseconds",
			endpoint:        `{"hostname":"api.example.com","timeout_ms":1000,"max_ttl":30}`,
			expectedTimeout: 30,
		},
		{
			name:            "fractional milliseconds round up",
			endpoint:        `{"hostname":"api.example.com","timeout_ms":1000,"max_ttl":32}`,
			expectedTimeout: 29,
		},
		{
			name:            "positive sub-millisecond timeout does not trigger the check default",
			endpoint:        `{"hostname":"api.example.com","timeout_ms":1,"max_ttl":30}`,
			expectedTimeout: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, err := parseConfig(rawScheduledConfig("test-config-a", tt.endpoint))
			require.NoError(t, err)
			require.Len(t, configs, 1)

			instance := unmarshalInstance(t, configs[0].Instances[0])
			assert.Equal(t, tt.expectedTimeout, instance["timeout"])
		})
	}
}

func TestCalculatePerHopTimeoutMS(t *testing.T) {
	tests := []struct {
		name            string
		totalTimeoutMS  int64
		maxTTL          int
		expectedTimeout int64
	}{
		{
			name:            "reserves ten percent",
			totalTimeoutMS:  1000,
			maxTTL:          1,
			expectedTimeout: 900,
		},
		{
			name:            "divides budget evenly across hops",
			totalTimeoutMS:  1000,
			maxTTL:          30,
			expectedTimeout: 30,
		},
		{
			name:            "rounds fractional milliseconds up",
			totalTimeoutMS:  1000,
			maxTTL:          32,
			expectedTimeout: 29,
		},
		{
			name:            "minimum contract values remain positive",
			totalTimeoutMS:  1,
			maxTTL:          1,
			expectedTimeout: 1,
		},
		{
			name:            "minimum timeout at maximum TTL remains positive",
			totalTimeoutMS:  1,
			maxTTL:          255,
			expectedTimeout: 1,
		},
		{
			name:            "maximum timeout and TTL",
			totalTimeoutMS:  120000,
			maxTTL:          255,
			expectedTimeout: 424,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedTimeout, calculatePerHopTimeoutMS(tt.totalTimeoutMS, tt.maxTTL))
		})
	}
}

func TestProviderTimeoutUsesEffectiveDefaultMaxTTL(t *testing.T) {
	configs, err := parseConfig(rawScheduledConfig("test-config-a", `{"hostname":"api.example.com","timeout_ms":1000}`))
	require.NoError(t, err)
	require.Len(t, configs, 1)

	instance := unmarshalInstance(t, configs[0].Instances[0])
	assert.Equal(t, 30, instance["timeout"])
	assert.NotContains(t, instance, "max_ttl")

	checkConfig, err := networkpathcheck.NewCheckConfig(configs[0].Instances[0], nil)
	require.NoError(t, err)
	assert.Equal(t, uint8(30), checkConfig.MaxTTL)
	assert.Equal(t, 30*time.Millisecond, checkConfig.Timeout)
}

func TestProviderOmittedTimeoutKeepsExistingCheckDefault(t *testing.T) {
	configs, err := parseConfig(rawScheduledConfig("test-config-a", `{"hostname":"api.example.com","max_ttl":30}`))
	require.NoError(t, err)
	require.Len(t, configs, 1)

	instance := unmarshalInstance(t, configs[0].Instances[0])
	assert.NotContains(t, instance, "timeout")

	checkConfig, err := networkpathcheck.NewCheckConfig(configs[0].Instances[0], nil)
	require.NoError(t, err)
	assert.Equal(t, time.Second, checkConfig.Timeout)
}

func TestLocalYAMLTimeoutRemainsPerHop(t *testing.T) {
	config, err := networkpathcheck.NewCheckConfig(
		integration.Data("hostname: api.example.com\ntimeout: 1000\nmax_ttl: 30\n"),
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, time.Second, config.Timeout)
}

func TestProviderSendChangesReturnsOnShutdownWhenChannelFull(t *testing.T) {
	provider := NewProvider()
	ctx, cancel := context.WithCancel(context.Background())
	provider.Stream(ctx)

	for len(provider.configChanges) < cap(provider.configChanges) {
		provider.configChanges <- integration.ConfigChanges{Schedule: []integration.Config{{Name: networkpathcheck.CheckName}}}
	}

	done := make(chan struct{})
	go func() {
		provider.sendChanges(integration.ConfigChanges{Schedule: []integration.Config{{Name: networkpathcheck.CheckName}}})
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sendChanges blocked after stream shutdown")
	}
}

func TestProviderValidUpdateReplacesWholePath(t *testing.T) {
	provider := NewProvider()
	changesCh := provider.Stream(context.Background())
	assert.Empty(t, <-changesCh)

	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: rawScheduledConfig("test-config-a", `{"hostname":"api.example.com"}`)},
	}, applyStatuses().callback)
	first := <-changesCh
	require.Len(t, first.Schedule, 1)

	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: rawScheduledConfig("test-config-a", `{"hostname":"db.example.com"}`)},
	}, applyStatuses().callback)
	second := <-changesCh
	require.Len(t, second.Unschedule, 1)
	require.Len(t, second.Schedule, 1)

	oldInstance := unmarshalInstance(t, second.Unschedule[0].Instances[0])
	newInstance := unmarshalInstance(t, second.Schedule[0].Instances[0])
	assert.Equal(t, "api.example.com", oldInstance["hostname"])
	assert.Equal(t, "db.example.com", newInstance["hostname"])
}

func TestProviderInvalidUpdateKeepsLastValidConfig(t *testing.T) {
	provider := NewProvider()
	changesCh := provider.Stream(context.Background())
	assert.Empty(t, <-changesCh)

	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: rawScheduledConfig("test-config-a", `{"hostname":"api.example.com"}`)},
	}, applyStatuses().callback)
	first := <-changesCh
	require.Len(t, first.Schedule, 1)

	statuses := applyStatuses()
	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: rawScheduledConfig("test-config-a", `{"port":443}`)},
	}, statuses.callback)

	assert.Equal(t, state.ApplyStateError, statuses.values["path/a"].State)
	assert.Contains(t, statuses.values["path/a"].Error, "tests[0]")
	assertNoChanges(t, changesCh)
	assert.NotEmpty(t, provider.GetConfigErrors()["path/a"])

	statuses = applyStatuses()
	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: rawScheduledConfig("test-config-a", `{"hostname":"api.example.com"}`)},
	}, statuses.callback)
	assert.Equal(t, state.ApplyStateAcknowledged, statuses.values["path/a"].State)
	assert.Empty(t, provider.GetConfigErrors())
	assertNoChanges(t, changesCh)
}

func TestProviderMissingPathUnschedulesActiveConfigs(t *testing.T) {
	provider := NewProvider()
	changesCh := provider.Stream(context.Background())
	assert.Empty(t, <-changesCh)

	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: rawScheduledConfig("test-config-a", `{"hostname":"api.example.com"}`)},
		"path/b": {Config: rawScheduledConfig("test-config-b", `{"hostname":"db.example.com"}`)},
	}, applyStatuses().callback)
	first := <-changesCh
	require.Len(t, first.Schedule, 2)

	provider.Update(map[string]state.RawConfig{
		"path/b": {Config: rawScheduledConfig("test-config-b", `{"hostname":"db.example.com"}`)},
	}, applyStatuses().callback)

	second := <-changesCh
	require.Len(t, second.Unschedule, 1)
	assert.Empty(t, second.Schedule)
	instance := unmarshalInstance(t, second.Unschedule[0].Instances[0])
	assert.Equal(t, "test-config-a", instance["test_config_id"])
	assert.Equal(t, "api.example.com", instance["hostname"])
}

func TestProviderTreatsRCPathsAsOpaqueConfigIdentities(t *testing.T) {
	provider := NewProvider()
	changesCh := provider.Stream(context.Background())
	assert.Empty(t, <-changesCh)

	provider.Update(map[string]state.RawConfig{
		"datadog/2/NETWORK_PATH/test-config-aaa-bbb-ccc/config":           {Config: rawScheduledConfig("aaa-bbb-ccc", `{"hostname":"plain.example.com"}`)},
		"datadog/2/NETWORK_PATH/test-config-scheduled-def-ghi-jkl/config": {Config: rawScheduledConfig("def-ghi-jkl", `{"hostname":"typed.example.com"}`)},
	}, applyStatuses().callback)

	first := <-changesCh
	assert.Empty(t, first.Unschedule)
	require.Len(t, first.Schedule, 2)
	assert.ElementsMatch(t, []string{"plain.example.com", "typed.example.com"}, scheduledHostnames(t, first.Schedule))

	provider.Update(map[string]state.RawConfig{
		"datadog/2/NETWORK_PATH/test-config-aaa-bbb-ccc/config": {Config: rawScheduledConfig("aaa-bbb-ccc", `{"hostname":"plain.example.com"}`)},
	}, applyStatuses().callback)

	second := <-changesCh
	require.Len(t, second.Unschedule, 1)
	assert.Empty(t, second.Schedule)
	assert.Equal(t, []string{"typed.example.com"}, scheduledHostnames(t, second.Unschedule))
}

func TestProviderMissingPathClearsStaleConfigError(t *testing.T) {
	provider := NewProvider()
	changesCh := provider.Stream(context.Background())
	assert.Empty(t, <-changesCh)

	statuses := applyStatuses()
	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: rawScheduledConfig("test-config-a", `{"port":443}`)},
	}, statuses.callback)

	assert.Equal(t, state.ApplyStateError, statuses.values["path/a"].State)
	assert.NotEmpty(t, provider.GetConfigErrors()["path/a"])
	assertNoChanges(t, changesCh)

	provider.Update(map[string]state.RawConfig{}, applyStatuses().callback)
	assert.Empty(t, provider.GetConfigErrors())
	assertNoChanges(t, changesCh)
}

func TestProviderMixedSnapshotSchedulesValidAndKeepsInvalidError(t *testing.T) {
	provider := NewProvider()
	changesCh := provider.Stream(context.Background())
	assert.Empty(t, <-changesCh)

	statuses := applyStatuses()
	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: rawScheduledConfig("test-config-a", `{"port":443}`)},
		"path/b": {Config: rawScheduledConfig("test-config-b", `{"hostname":"db.example.com"}`)},
	}, statuses.callback)

	assert.Equal(t, state.ApplyStateError, statuses.values["path/a"].State)
	assert.Equal(t, state.ApplyStateAcknowledged, statuses.values["path/b"].State)
	assert.NotEmpty(t, provider.GetConfigErrors()["path/a"])

	changes := <-changesCh
	assert.Empty(t, changes.Unschedule)
	require.Len(t, changes.Schedule, 1)
	instance := unmarshalInstance(t, changes.Schedule[0].Instances[0])
	assert.Equal(t, "test-config-b", instance["test_config_id"])
	assert.Equal(t, "db.example.com", instance["hostname"])
}

func TestProviderEmptyTestsFailsClosed(t *testing.T) {
	provider := NewProvider()
	changesCh := provider.Stream(context.Background())
	assert.Empty(t, <-changesCh)

	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: rawScheduledConfig("test-config-a", `{"hostname":"api.example.com"}`)},
	}, applyStatuses().callback)
	first := <-changesCh
	require.Len(t, first.Schedule, 1)

	statuses := applyStatuses()
	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: []byte(`{"type":"scheduled","test_config_id":"test-config-a","config":{"tests":[]}}`)},
	}, statuses.callback)

	assert.Equal(t, state.ApplyStateError, statuses.values["path/a"].State)
	assert.Contains(t, statuses.values["path/a"].Error, "config.tests must contain at least one item")
	assertNoChanges(t, changesCh)
	assert.NotEmpty(t, provider.GetConfigErrors()["path/a"])
}

func TestProviderIgnoresDynamicType(t *testing.T) {
	provider := NewProvider()
	changesCh := provider.Stream(context.Background())
	assert.Empty(t, <-changesCh)

	statuses := applyStatuses()
	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: []byte(rawDynamicConfigString("test-config-a"))},
	}, statuses.callback)

	assert.NotContains(t, statuses.values, "path/a")
	assert.NotContains(t, provider.GetConfigErrors(), "path/a")
	assertNoChanges(t, changesCh)
}

func TestProviderDynamicTypeDefensivelyClearsScheduledStateAtSamePath(t *testing.T) {
	provider := NewProvider()
	changesCh := provider.Stream(context.Background())
	assert.Empty(t, <-changesCh)

	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: rawScheduledConfig("test-config-a", `{"hostname":"api.example.com"}`)},
	}, applyStatuses().callback)
	first := <-changesCh
	require.Len(t, first.Schedule, 1)

	statuses := applyStatuses()
	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: []byte(rawDynamicConfigString("dynamic-sentinel"))},
	}, statuses.callback)

	assert.NotContains(t, statuses.values, "path/a")
	second := <-changesCh
	require.Len(t, second.Unschedule, 1)
	assert.Empty(t, second.Schedule)
	assert.NotContains(t, provider.activeByPath, "path/a")
	assert.NotContains(t, provider.GetConfigErrors(), "path/a")
}

func TestProviderDynamicSnapshotUnschedulesMissingScheduledPath(t *testing.T) {
	provider := NewProvider()
	changesCh := provider.Stream(context.Background())
	assert.Empty(t, <-changesCh)

	provider.Update(map[string]state.RawConfig{
		"path/scheduled": {Config: rawScheduledConfig("test-config-a", `{"hostname":"api.example.com"}`)},
	}, applyStatuses().callback)
	first := <-changesCh
	require.Len(t, first.Schedule, 1)

	statuses := applyStatuses()
	provider.Update(map[string]state.RawConfig{
		"path/dynamic": {Config: []byte(rawDynamicConfigString("dynamic-sentinel"))},
	}, statuses.callback)

	assert.NotContains(t, statuses.values, "path/dynamic")
	second := <-changesCh
	require.Len(t, second.Unschedule, 1)
	assert.Empty(t, second.Schedule)
	instance := unmarshalInstance(t, second.Unschedule[0].Instances[0])
	assert.Equal(t, "test-config-a", instance["test_config_id"])
	assert.Equal(t, "api.example.com", instance["hostname"])
	assert.NotContains(t, provider.GetConfigErrors(), "path/dynamic")
}

func TestProviderRejectsUnsupportedType(t *testing.T) {
	provider := NewProvider()
	changesCh := provider.Stream(context.Background())
	assert.Empty(t, <-changesCh)

	statuses := applyStatuses()
	provider.Update(map[string]state.RawConfig{
		"path/a": {Config: []byte(`{"type":"triggered","test_config_id":"test-config-a","config":{"tests":[{"hostname":"api.example.com"}]}}`)},
	}, statuses.callback)

	assert.Equal(t, state.ApplyStateError, statuses.values["path/a"].State)
	assert.Contains(t, statuses.values["path/a"].Error, `unsupported Network Path config type "triggered"`)
	assertNoChanges(t, changesCh)
}

func TestParseConfigValidBoundariesAndNormalization(t *testing.T) {
	configs, err := parseConfig(rawScheduledConfig(
		" test-config-a ",
		`{"hostname":" api.example.com ","port":1,"protocol":" udp ","max_ttl":1,"timeout_ms":1,"interval_sec":1}`,
		`{"hostname":"edge.example.com","port":65535,"protocol":" tCp ","max_ttl":255,"tcp_method":" SYN_SOCKET "}`,
		`{"hostname":"router.example.com","protocol":" iCmP "}`,
	))
	require.NoError(t, err)
	require.Len(t, configs, 3)

	first := unmarshalInstance(t, configs[0].Instances[0])
	assert.Equal(t, "test-config-a", first["test_config_id"])
	assert.Equal(t, "api.example.com", first["hostname"])
	assert.Equal(t, 1, first["port"])
	assert.Equal(t, "UDP", first["protocol"])
	assert.Equal(t, 1, first["max_ttl"])
	assert.Equal(t, 1, first["timeout"])
	assert.Equal(t, 1, first["min_collection_interval"])

	second := unmarshalInstance(t, configs[1].Instances[0])
	assert.Equal(t, 65535, second["port"])
	assert.Equal(t, "TCP", second["protocol"])
	assert.Equal(t, 255, second["max_ttl"])
	assert.Equal(t, "syn_socket", second["tcp_method"])

	third := unmarshalInstance(t, configs[2].Instances[0])
	assert.Equal(t, "ICMP", third["protocol"])
}

func TestParseConfigOutputIsAcceptedByNetworkPathCheck(t *testing.T) {
	configs, err := parseConfig(rawScheduledConfig(
		"test-config-a",
		`{"hostname":"api.example.com","port":443,"protocol":"tcp","max_ttl":30,"timeout_ms":1000,"interval_sec":60,"source_service":"frontend","destination_service":"api","tcp_method":"syn","traceroute_queries":3,"e2e_queries":50,"tags":["env:prod"]}`,
	))
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.Len(t, configs[0].Instances, 1)

	checkConfig, err := networkpathcheck.NewCheckConfig(configs[0].Instances[0], nil)
	require.NoError(t, err)
	assert.Equal(t, "test-config-a", checkConfig.TestConfigID)
	assert.Equal(t, "api.example.com", checkConfig.DestHostname)
	assert.Equal(t, uint16(443), checkConfig.DestPort)
	assert.Equal(t, payload.ProtocolTCP, checkConfig.Protocol)
	assert.Equal(t, uint8(30), checkConfig.MaxTTL)
	assert.Equal(t, 30*time.Millisecond, checkConfig.Timeout)
	assert.Equal(t, time.Minute, checkConfig.MinCollectionInterval)
	assert.Equal(t, "frontend", checkConfig.SourceService)
	assert.Equal(t, "api", checkConfig.DestinationService)
	assert.Equal(t, payload.TCPConfigSYN, checkConfig.TCPMethod)
	assert.Equal(t, 3, checkConfig.TracerouteQueries)
	assert.Equal(t, 50, checkConfig.E2eQueries)
	assert.Equal(t, []string{"env:prod"}, checkConfig.Tags)
}

func TestParseConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		expectedErr string
	}{
		{
			name:        "malformed json",
			raw:         `{"type":"scheduled",`,
			expectedErr: "invalid Network Path config",
		},
		{
			name:        "missing type",
			raw:         `{"test_config_id":"test-config-a","config":{"tests":[{"hostname":"api.example.com"}]}}`,
			expectedErr: "type is required",
		},
		{
			name:        "missing test config id",
			raw:         `{"type":"scheduled","config":{"tests":[{"hostname":"api.example.com"}]}}`,
			expectedErr: "test_config_id is required",
		},
		{
			name:        "missing tests",
			raw:         `{"type":"scheduled","test_config_id":"test-config-a","config":{}}`,
			expectedErr: "config.tests must be provided",
		},
		{
			name:        "missing config",
			raw:         `{"type":"scheduled","test_config_id":"test-config-a"}`,
			expectedErr: "config must be provided",
		},
		{
			name:        "missing hostname",
			raw:         rawScheduledConfigString("test-config-a", `{"port":443}`),
			expectedErr: "hostname is required",
		},
		{
			name:        "invalid port",
			raw:         rawScheduledConfigString("test-config-a", `{"hostname":"api.example.com","port":70000}`),
			expectedErr: "port must be between 1 and 65535",
		},
		{
			name:        "invalid max ttl",
			raw:         rawScheduledConfigString("test-config-a", `{"hostname":"api.example.com","max_ttl":256}`),
			expectedErr: "max_ttl must be between 1 and 255",
		},
		{
			name:        "invalid interval",
			raw:         rawScheduledConfigString("test-config-a", `{"hostname":"api.example.com","interval_sec":0}`),
			expectedErr: "interval_sec must be > 0",
		},
		{
			name:        "invalid timeout",
			raw:         rawScheduledConfigString("test-config-a", `{"hostname":"api.example.com","timeout_ms":0}`),
			expectedErr: "timeout_ms must be > 0",
		},
		{
			name:        "invalid protocol",
			raw:         rawScheduledConfigString("test-config-a", `{"hostname":"api.example.com","protocol":"sctp"}`),
			expectedErr: "unsupported protocol",
		},
		{
			name:        "invalid tcp method",
			raw:         rawScheduledConfigString("test-config-a", `{"hostname":"api.example.com","tcp_method":"bad"}`),
			expectedErr: "unsupported tcp_method",
		},
		{
			name:        "invalid traceroute queries",
			raw:         rawScheduledConfigString("test-config-a", `{"hostname":"api.example.com","traceroute_queries":0}`),
			expectedErr: "traceroute_queries must be > 0",
		},
		{
			name:        "invalid e2e queries",
			raw:         rawScheduledConfigString("test-config-a", `{"hostname":"api.example.com","e2e_queries":0}`),
			expectedErr: "e2e_queries must be > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseConfig([]byte(tt.raw))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

type statusRecorder struct {
	values map[string]state.ApplyStatus
}

func applyStatuses() *statusRecorder {
	return &statusRecorder{values: make(map[string]state.ApplyStatus)}
}

func (s *statusRecorder) callback(path string, status state.ApplyStatus) {
	s.values[path] = status
}

func rawScheduledConfig(testConfigID string, endpoints ...string) []byte {
	return []byte(rawScheduledConfigString(testConfigID, endpoints...))
}

func rawScheduledConfigString(testConfigID string, endpoints ...string) string {
	tests := strings.Join(endpoints, ",")
	return `{"type":"scheduled","test_config_id":"` + testConfigID + `","config":{"tests":[` + tests + `]}}`
}

func rawDynamicConfigString(testConfigID string) string {
	return `{"type":"dynamic","test_config_id":"` + testConfigID + `","config":{"filters":[{"type":"exclude","match_domain":"*.example.com","match_domain_strategy":"wildcard"}]}}`
}

func unmarshalInstance(t *testing.T, instanceData integration.Data) map[string]interface{} {
	t.Helper()
	var instance map[string]interface{}
	require.NoError(t, yaml.Unmarshal(instanceData, &instance))
	return instance
}

func scheduledHostnames(t *testing.T, configs []integration.Config) []string {
	t.Helper()
	hostnames := make([]string, 0, len(configs))
	for _, config := range configs {
		require.Len(t, config.Instances, 1)
		hostnames = append(hostnames, unmarshalInstance(t, config.Instances[0])["hostname"].(string))
	}
	return hostnames
}

func assertNoChanges(t *testing.T, changesCh <-chan integration.ConfigChanges) {
	t.Helper()
	select {
	case changes := <-changesCh:
		require.True(t, changes.IsEmpty(), "unexpected config changes: %#v", changes)
	default:
	}
}
