// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"context"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *ActionStore {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return NewActionStore(ctx)
}

// TestClaim_SetsClaimedAtAndStatus checks that a successful claim sets the claimed at timestamp and status.
func TestClaim_SetsClaimedAtAndStatus(t *testing.T) {
	store := newTestStore(t)
	key := ActionKey{ID: "test-action", Version: 1}

	before := time.Now().Unix()
	ok := store.Claim(key)
	after := time.Now().Unix()

	if !ok {
		t.Fatal("expected Claim to return true")
	}
	record, exists := store.GetRecord(key)
	if !exists {
		t.Fatal("expected record to exist after Claim")
	}
	if record.Status != StatusClaimed {
		t.Errorf("got status %q, want %q", record.Status, StatusClaimed)
	}
	if record.ClaimedAt < before || record.ClaimedAt > after {
		t.Errorf("ClaimedAt %d not in range [%d, %d]", record.ClaimedAt, before, after)
	}
}

// TestClaim_DuplicateReturnsFalse checks that a duplicate claim returns false.
func TestClaim_DuplicateReturnsFalse(t *testing.T) {
	store := newTestStore(t)
	key := ActionKey{ID: "test-action", Version: 1}

	if !store.Claim(key) {
		t.Fatal("first Claim should succeed")
	}
	if store.Claim(key) {
		t.Error("second Claim on same key should return false")
	}
}

// TestCleanup_RemovesStaleClaimedRecord checks that a stale claimed record is removed by cleanup.
func TestCleanup_RemovesStaleClaimedRecord(t *testing.T) {
	store := newTestStore(t)
	key := ActionKey{ID: "stale", Version: 1}

	store.Claim(key)

	// Backdate ClaimedAt past retention cutoff
	store.mu.Lock()
	k := key.String()
	r := store.executed[k]
	r.ClaimedAt = time.Now().Add(-(RecordRetentionTTL + time.Second)).Unix()
	store.executed[k] = r
	store.mu.Unlock()

	store.cleanup()

	if _, exists := store.GetRecord(key); exists {
		t.Error("stale claimed record should have been removed by cleanup")
	}
}

// TestCleanup_KeepsRecentClaimedRecord checks that a recent claimed record is not removed by cleanup.
func TestCleanup_KeepsRecentClaimedRecord(t *testing.T) {
	store := newTestStore(t)
	key := ActionKey{ID: "recent", Version: 1}

	store.Claim(key)
	store.cleanup()

	if _, exists := store.GetRecord(key); !exists {
		t.Error("recent claimed record should not be removed by cleanup")
	}
}
