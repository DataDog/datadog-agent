// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// memCursorStore is an in-memory cursorStore for the tests in this package.
// It is not yet exercised by a test in this file: the sweeper that drives a
// discovery cycle (a later component, in this same package) is what will
// use it as a fake cursorStore.
var _ cursorStore = newMemCursorStore()

type memCursorStore struct {
	states map[string]cursorState
	saves  int
}

func newMemCursorStore() *memCursorStore {
	return &memCursorStore{states: map[string]cursorState{}}
}

func (m *memCursorStore) Load(id string) (cursorState, bool) {
	s, ok := m.states[id]
	return s, ok
}

func (m *memCursorStore) Save(id string, s cursorState) error {
	m.saves++
	m.states[id] = s
	return nil
}

func (m *memCursorStore) Clear(id string) error {
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
	cfg := rangeConfig{
		CIDR:               "10.0.0.0/24",
		IgnoredIPAddresses: []string{"10.0.0.2", "10.0.0.1"},
	}
	creds := []connectivity.SNMPCredential{
		{ID: "cred-b", Version: "2c", Community: "public"},
		{ID: "cred-a", Version: "2c", Community: "public"},
	}

	a := rangeDigest(cfg, creds)

	// Order must not matter: the same set yields the same digest.
	cfg2 := rangeConfig{
		CIDR:               "10.0.0.0/24",
		IgnoredIPAddresses: []string{"10.0.0.1", "10.0.0.2"},
	}
	creds2 := []connectivity.SNMPCredential{
		{ID: "cred-a", Version: "2c", Community: "public"},
		{ID: "cred-b", Version: "2c", Community: "public"},
	}
	assert.Equal(t, a, rangeDigest(cfg2, creds2))
	assert.NotEmpty(t, a)
}

func TestRangeDigestChangesWithRangeAndCredentials(t *testing.T) {
	base := rangeConfig{CIDR: "10.0.0.0/24", IgnoredIPAddresses: []string{"10.0.0.1"}}
	creds := []connectivity.SNMPCredential{{ID: "cred-a", Version: "2c", Community: "public"}}
	a := rangeDigest(base, creds)

	changedCIDR := base
	changedCIDR.CIDR = "10.0.1.0/24"
	assert.NotEqual(t, a, rangeDigest(changedCIDR, creds))

	changedIgnored := base
	changedIgnored.IgnoredIPAddresses = []string{"10.0.0.9"}
	assert.NotEqual(t, a, rangeDigest(changedIgnored, creds))

	changedSecret := []connectivity.SNMPCredential{{ID: "cred-a", Version: "2c", Community: "rotated"}}
	assert.NotEqual(t, a, rangeDigest(base, changedSecret))
}

func TestRangeDigestIgnoresInterval(t *testing.T) {
	creds := []connectivity.SNMPCredential{{ID: "cred-a", Version: "2c"}}
	a := rangeConfig{CIDR: "10.0.0.0/24", IntervalSec: 3600}
	b := rangeConfig{CIDR: "10.0.0.0/24", IntervalSec: 900}
	assert.Equal(t, rangeDigest(a, creds), rangeDigest(b, creds),
		"an interval-only change must not discard scan progress")
}
