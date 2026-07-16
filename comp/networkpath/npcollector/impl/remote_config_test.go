// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/connfilter"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestDynamicRemoteConfigLifecycle(t *testing.T) {
	collector := newRemoteConfigTestCollector(t, []connfilter.Config{{
		Type:        connfilter.FilterTypeExclude,
		MatchDomain: "*.local.example",
	}})
	statuses := make(map[string]state.ApplyStatus)
	callback := func(path string, status state.ApplyStatus) { statuses[path] = status }

	collector.UpdateRemoteConfig(map[string]state.RawConfig{
		"scheduled": {Config: []byte(`{"type":"scheduled","test_config_id":"scheduled","config":{"tests":[]}}`)},
		"dynamic":   {Config: dynamicConfig("dynamic", `[{"type":"include","match_domain":"api.local.example"}]`)},
	}, callback)

	assert.NotContains(t, statuses, "scheduled")
	assert.Equal(t, state.ApplyStateAcknowledged, statuses["dynamic"].State)
	assert.False(t, collector.filter.IsIncluded("other.local.example", netip.Addr{}))
	assert.True(t, collector.filter.IsIncluded("api.local.example", netip.Addr{}), "RC must override a matching local filter")

	// An invalid replacement is rejected atomically and preserves the last valid
	// config for the same RC path.
	collector.UpdateRemoteConfig(map[string]state.RawConfig{
		"dynamic": {Config: dynamicConfig("dynamic", `[{"type":"exclude","match_domain":"[","match_domain_strategy":"regex"}]`)},
	}, callback)
	assert.Equal(t, state.ApplyStateError, statuses["dynamic"].State)
	assert.True(t, collector.filter.IsIncluded("api.local.example", netip.Addr{}))

	// Deletion removes the RC layer while preserving local filters.
	collector.UpdateRemoteConfig(map[string]state.RawConfig{}, callback)
	assert.False(t, collector.filter.IsIncluded("api.local.example", netip.Addr{}))
}

func TestDynamicRemoteConfigConflictFallsBackToLocal(t *testing.T) {
	collector := newRemoteConfigTestCollector(t, nil)
	statuses := make(map[string]state.ApplyStatus)
	callback := func(path string, status state.ApplyStatus) { statuses[path] = status }

	collector.UpdateRemoteConfig(map[string]state.RawConfig{
		"a": {Config: dynamicConfig("a", `[{"type":"exclude","match_domain":"api.example.com"}]`)},
		"b": {Config: dynamicConfig("b", `[{"type":"include","match_domain":"api.example.com"}]`)},
	}, callback)

	assert.Equal(t, state.ApplyStateError, statuses["a"].State)
	assert.Equal(t, state.ApplyStateError, statuses["b"].State)
	assert.Contains(t, statuses["a"].Error, "multiple dynamic NETWORK_PATH configs")
	assert.True(t, collector.filter.IsIncluded("api.example.com", netip.Addr{}), "conflicts must remove the entire RC layer")

	collector.UpdateRemoteConfig(map[string]state.RawConfig{
		"a": {Config: dynamicConfig("a", `[{"type":"exclude","match_domain":"api.example.com"}]`)},
	}, callback)
	assert.Equal(t, state.ApplyStateAcknowledged, statuses["a"].State)
	assert.False(t, collector.filter.IsIncluded("api.example.com", netip.Addr{}))
}

func TestDynamicRemoteConfigAcknowledgedWhenCollectorDisabled(t *testing.T) {
	collector := newNoopNpCollectorImpl()
	statuses := make(map[string]state.ApplyStatus)
	collector.UpdateRemoteConfig(map[string]state.RawConfig{
		"dynamic": {Config: dynamicConfig("dynamic", `[{"type":"exclude","match_ip":"10.0.0.0/8"}]`)},
	}, func(path string, status state.ApplyStatus) { statuses[path] = status })

	assert.Equal(t, state.ApplyStateAcknowledged, statuses["dynamic"].State)
	assert.Len(t, collector.remoteConfigState, 1)
}

func TestParseRemoteDynamicConfigValidation(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		err  string
	}{
		{name: "missing id", raw: dynamicConfig("", `[{"type":"exclude","match_ip":"10.0.0.1"}]`), err: "test_config_id is required"},
		{name: "empty filters", raw: dynamicConfig("id", `[]`), err: "must contain at least one item"},
		{name: "empty matcher", raw: dynamicConfig("id", `[{"type":"exclude"}]`), err: "match_domain or match_ip is required"},
		{name: "invalid type", raw: dynamicConfig("id", `[{"type":"drop","match_ip":"10.0.0.1"}]`), err: "invalid filter type"},
		{name: "invalid cidr", raw: dynamicConfig("id", `[{"type":"exclude","match_ip":"10.0.0.0/99"}]`), err: "failed to parsing match_ip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, dynamic, err := parseRemoteDynamicConfig(tt.raw)
			assert.True(t, dynamic)
			require.ErrorContains(t, err, tt.err)
		})
	}
}

func newRemoteConfigTestCollector(t *testing.T, local []connfilter.Config) *npCollectorImpl {
	t.Helper()
	filter, errs := connfilter.NewConnFilter(local, "", false)
	require.Empty(t, errs)
	return &npCollectorImpl{
		collectorConfigs: &collectorConfigs{filterConfig: local},
		filter:           filter,
	}
}

func dynamicConfig(id, filters string) []byte {
	return []byte(`{"type":"dynamic","test_config_id":"` + id + `","config":{"filters":` + filters + `}}`)
}
