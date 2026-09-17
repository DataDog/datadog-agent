// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

var _ cursorStore = newMemCursorStore()

// memCursorStore is an in-memory cursorStore for the tests in this package.
type memCursorStore struct {
	mu     sync.Mutex
	states map[string]cursorState
	saves  int
}

func newMemCursorStore() *memCursorStore {
	return &memCursorStore{states: map[string]cursorState{}}
}

func (m *memCursorStore) Load(id string) (cursorState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.states[id]
	return s, ok
}

func (m *memCursorStore) Save(id string, s cursorState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saves++
	m.states[id] = s
	return nil
}

func (m *memCursorStore) Clear(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.states, id)
	return nil
}

func TestPersistentCursorStoreRoundTrip(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("run_path", t.TempDir())
	store := newPersistentCursorStore()

	_, ok := store.Load("ad-1")
	assert.False(t, ok, "an unknown range has no cursor")

	want := cursorState{
		RunID:        "run-1",
		NextChunk:    12,
		Scanned:      3072,
		StartedAtMs:  1700000000000,
		ConfigDigest: "abc123",
	}
	require.NoError(t, store.Save("ad-1", want))

	got, ok := store.Load("ad-1")
	require.True(t, ok)
	assert.Equal(t, want, got)

	require.NoError(t, store.Clear("ad-1"))
	_, ok = store.Load("ad-1")
	assert.False(t, ok)
}

func TestPersistentCursorStoreClearIsIdempotent(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("run_path", t.TempDir())
	store := newPersistentCursorStore()
	assert.NoError(t, store.Clear("never-written"))
}

func TestRangeDigestIsStable(t *testing.T) {
	cfg := rangeConfig{CIDR: "10.0.0.0/24", IgnoredIPAddresses: []string{"10.0.0.2", "10.0.0.1"}}
	a := rangeDigest(cfg, []string{"snmp:b", "ping:a"})

	cfg2 := rangeConfig{CIDR: "10.0.0.0/24", IgnoredIPAddresses: []string{"10.0.0.1", "10.0.0.2"}}
	assert.Equal(t, a, rangeDigest(cfg2, []string{"ping:a", "snmp:b"}))
	assert.NotEmpty(t, a)
}

func TestRangeDigestChangesWithTheRangeAndTheProbes(t *testing.T) {
	base := rangeConfig{CIDR: "10.0.0.0/24", IgnoredIPAddresses: []string{"10.0.0.1"}}
	fingerprints := []string{"snmp:one"}
	a := rangeDigest(base, fingerprints)

	changedCIDR := base
	changedCIDR.CIDR = "10.0.1.0/24"
	assert.NotEqual(t, a, rangeDigest(changedCIDR, fingerprints))

	changedIgnored := base
	changedIgnored.IgnoredIPAddresses = []string{"10.0.0.9"}
	assert.NotEqual(t, a, rangeDigest(changedIgnored, fingerprints))

	assert.NotEqual(t, a, rangeDigest(base, []string{"snmp:two"}),
		"a probe option or credential change invalidates a partial cycle")
	assert.NotEqual(t, a, rangeDigest(base, []string{"snmp:one", "ping:one"}),
		"adding a probe invalidates a partial cycle")
}

func TestRangeDigestIgnoresInterval(t *testing.T) {
	a := rangeConfig{CIDR: "10.0.0.0/24", IntervalSec: 3600}
	b := rangeConfig{CIDR: "10.0.0.0/24", IntervalSec: 900}
	assert.Equal(t, rangeDigest(a, []string{"snmp:one"}), rangeDigest(b, []string{"snmp:one"}),
		"an interval-only change must not discard scan progress")
}
