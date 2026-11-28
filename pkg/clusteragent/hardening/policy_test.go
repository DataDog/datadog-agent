// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const testCluster = "6f2c9a1e-0000-4000-8000-000000000001"

var testAddedAt = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

// testPolicy returns a capabilities policy that lists testCluster as added at addedAt.
func testPolicy(state State, addedAt time.Time) []byte {
	return []byte(fmt.Sprintf(`{
		"type": "policy", "id": "hp-caps", "control": "capabilities", "state": %q,
		"clusters": [{"id": %q, "added_at": %q}, {"id": "other", "added_at": "2026-10-01T00:00:00Z"}],
		"limits": {"first_batch": 2, "hourly_cap": 10, "pause_threshold": 2},
		"min_exceptions_generation": 1
	}`, state, testCluster, addedAt.Format(time.RFC3339)))
}

// testExceptions returns the only shard of exceptions generation 1, with fields
// a JSON fragment of extra fields.
func testExceptions(fields string) []byte {
	if fields != "" {
		fields = ", " + fields
	}
	return []byte(`{"type": "exceptions", "control": "capabilities", "generation": 1, "shard": 0, "shards": 1` + fields + `}`)
}

func setRC(t *testing.T, s *Store, raws ...[]byte) {
	t.Helper()
	updates := map[string]state.RawConfig{}
	for i, raw := range raws {
		updates[fmt.Sprintf("datadog/2/CWS_SECURITY_PROFILES/c%d/config", i)] = state.RawConfig{Config: raw}
	}
	s.OnRCUpdate(updates, func(path string, status state.ApplyStatus) {
		require.Equal(t, state.ApplyStateAcknowledged, status.State, "%s: %s", path, status.Error)
	})
}

func TestParsePolicy(t *testing.T) {
	p, _, err := parseConfig(testPolicy(StateEnforce, testAddedAt), testCluster)
	require.NoError(t, err)
	assert.True(t, p.Active())
	assert.Equal(t, testAddedAt, p.cluster.AddedAt)

	p, _, err = parseConfig(testPolicy(StateEnforce, testAddedAt), "unlisted")
	require.NoError(t, err)
	assert.False(t, p.Active())

	for name, raw := range map[string]string{
		"unknown field":   `{"type": "policy", "id": "hp-1", "control": "capabilities", "state": "enforce", "limits": {"first_batch": 1, "hourly_cap": 1, "pause_threshold": 1}, "extra": 1}`,
		"unknown control": `{"type": "policy", "id": "hp-1", "control": "read_only_root_fs", "state": "enforce", "limits": {"first_batch": 1, "hourly_cap": 1, "pause_threshold": 1}}`,
		"no limits":       `{"type": "policy", "id": "hp-1", "control": "capabilities", "state": "enforce"}`,
		"no added_at":     `{"type": "policy", "id": "hp-1", "control": "capabilities", "state": "enforce", "limits": {"first_batch": 1, "hourly_cap": 1, "pause_threshold": 1}, "clusters": [{"id": "c"}]}`,
		"unknown type":    `{"type": "request"}`,
	} {
		_, _, err := parseConfig([]byte(raw), testCluster)
		assert.Error(t, err, name)
	}
}

func TestExceptionsMatch(t *testing.T) {
	_, e, err := parseConfig(testExceptions(`
		"namespaces": ["kube-system"],
		"image_repositories": ["nginx"],
		"workloads": [{"kind": "Deployment", "namespace": "shop", "name": "web", "container": "app", "reason": "evidence"}],
		"emergency_stops": [{"clusters": ["`+testCluster+`"], "namespaces": ["pay"]}, {"clusters": ["other"]}]`), testCluster)
	require.NoError(t, err)
	set := mergeShards([]*Exceptions{e})

	assert.True(t, set.Exempts("kube-system", "dns", "c", "coredns:1"))
	assert.True(t, set.Exempts("shop", "front", "c", "docker.io/library/nginx:1.25"), "repositories are normalized")
	assert.True(t, set.Exempts("shop", "web", "app", "web:1"))
	assert.False(t, set.Exempts("shop", "web", "sidecar", "web:1"))

	assert.True(t, set.Stops(testCluster, "pay", "api", "c", "api:1"))
	assert.False(t, set.Stops(testCluster, "shop", "api", "c", "api:1"))
	assert.True(t, set.Stops("other", "shop", "api", "c", "api:1"), "a stop with no matcher covers its whole cluster")
}

func TestStoreExceptionsNeedEveryShard(t *testing.T) {
	shard := func(gen, i, n int) []byte {
		return []byte(fmt.Sprintf(`{"type": "exceptions", "control": "capabilities", "generation": %d, "shard": %d, "shards": %d, "namespaces": ["g%d"]}`, gen, i, n, gen))
	}
	s := NewStore(testCluster)
	setRC(t, s, testPolicy(StateEnforce, testAddedAt), shard(1, 0, 1), shard(2, 0, 2))

	set, ok := s.Exceptions(ControlCapabilities, 1)
	require.True(t, ok)
	assert.Equal(t, int64(1), set.Generation, "generation 2 misses a shard")

	_, ok = s.Exceptions(ControlCapabilities, 2)
	assert.False(t, ok, "no complete generation is new enough")

	setRC(t, s, testPolicy(StateEnforce, testAddedAt), shard(1, 0, 1), shard(2, 0, 2), shard(2, 1, 2))
	set, ok = s.Exceptions(ControlCapabilities, 2)
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"g2", "g2"}, set.Namespaces)
}

func TestStoreRefusesASecondPolicyForAControl(t *testing.T) {
	s := NewStore(testCluster)
	var errs int
	s.OnRCUpdate(map[string]state.RawConfig{
		"a": {Config: testPolicy(StateEnforce, testAddedAt)},
		"b": {Config: testPolicy(StateStopped, testAddedAt)},
	}, func(_ string, status state.ApplyStatus) {
		if status.State == state.ApplyStateError {
			errs++
		}
	})
	assert.Equal(t, 1, errs)
	assert.Len(t, s.Policies(), 1)
}

func TestStoreRemoteConfigWinsOverFile(t *testing.T) {
	s := NewStore(testCluster)
	require.NoError(t, s.setFileConfigs([]byte(`[`+string(testPolicy(StateStopped, testAddedAt))+`]`)))
	setRC(t, s, testPolicy(StateEnforce, testAddedAt))
	p, ok := s.Policy("hp-caps")
	require.True(t, ok)
	assert.Equal(t, StateEnforce, p.State)
}
