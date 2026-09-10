// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	testQTypeA    = uint16(1)
	testQTypeAAAA = uint16(28)
)

func newTestDNSRequestTracker(t *testing.T) *dnsRequestTracker {
	t.Helper()
	tracker, err := newDNSRequestTracker(dnsRequestTrackerSize, dnsRequestTrackerTTL)
	require.NoError(t, err)
	return tracker
}

func TestDNSRequestTrackerMatchesRecordedRequest(t *testing.T) {
	tracker := newTestDNSRequestTracker(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now)

	assert.Same(t, entry, tracker.matchResponse(0x1234, "one.one.one.one", testQTypeA, now))
	assert.Equal(t, uint64(1), tracker.hits.Load())
}

func TestDNSRequestTrackerMissesUnknownKey(t *testing.T) {
	tracker := newTestDNSRequestTracker(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now)

	// wrong transaction ID
	assert.Nil(t, tracker.matchResponse(0x9999, "one.one.one.one", testQTypeA, now))
	// wrong name
	assert.Nil(t, tracker.matchResponse(0x1234, "example.com", testQTypeA, now))
	assert.Equal(t, uint64(2), tracker.misses.Load())
}

func TestDNSRequestTrackerExpiresAfterTTL(t *testing.T) {
	tracker := newTestDNSRequestTracker(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now)

	assert.Nil(t, tracker.matchResponse(0x1234, "one.one.one.one", testQTypeA, now.Add(dnsRequestTrackerTTL)))
	assert.Equal(t, uint64(1), tracker.misses.Load())
}

// A colliding transaction ID must lose the answer, never attribute it to the wrong process.
func TestDNSRequestTrackerCollisionPoisonsTheKey(t *testing.T) {
	tracker := newTestDNSRequestTracker(t)
	first := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	second := model.NewPlaceholderProcessCacheEntry(43, 43, false)
	now := time.Now()

	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, first, now)
	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, second, now)

	assert.Nil(t, tracker.matchResponse(0x1234, "one.one.one.one", testQTypeA, now))
	assert.Equal(t, uint64(1), tracker.collisions.Load())
}

// A tombstone must not be extended indefinitely by further colliding requests.
func TestDNSRequestTrackerPoisonDoesNotOutliveOriginalDeadline(t *testing.T) {
	tracker := newTestDNSRequestTracker(t)
	first := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	second := model.NewPlaceholderProcessCacheEntry(43, 43, false)
	third := model.NewPlaceholderProcessCacheEntry(44, 44, false)
	now := time.Now()

	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, first, now)
	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, second, now.Add(time.Second))
	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, third, now.Add(2*time.Second))

	// the tombstone still expires on the first request's deadline
	later := now.Add(dnsRequestTrackerTTL)
	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, third, later)
	assert.Same(t, third, tracker.matchResponse(0x1234, "one.one.one.one", testQTypeA, later))
}

// Some stub resolvers retransmit with the same transaction ID; that is not a collision.
func TestDNSRequestTrackerRetransmitRefreshesDeadline(t *testing.T) {
	tracker := newTestDNSRequestTracker(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now)
	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now.Add(4*time.Second))

	assert.Same(t, entry, tracker.matchResponse(0x1234, "one.one.one.one", testQTypeA, now.Add(6*time.Second)))
	assert.Equal(t, uint64(0), tracker.collisions.Load())
}

// nslookup issues A and AAAA for the same name; they must correlate independently.
func TestDNSRequestTrackerSeparatesQueryTypes(t *testing.T) {
	tracker := newTestDNSRequestTracker(t)
	a := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	aaaa := model.NewPlaceholderProcessCacheEntry(43, 43, false)
	now := time.Now()

	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, a, now)
	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeAAAA, aaaa, now)

	assert.Same(t, a, tracker.matchResponse(0x1234, "one.one.one.one", testQTypeA, now))
	assert.Same(t, aaaa, tracker.matchResponse(0x1234, "one.one.one.one", testQTypeAAAA, now))
	assert.Equal(t, uint64(0), tracker.collisions.Load())
}

// Resolvers may apply 0x20 randomisation and echo the question with randomised capitalisation.
func TestDNSRequestTrackerMatchesCaseInsensitively(t *testing.T) {
	tracker := newTestDNSRequestTracker(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now)

	assert.Same(t, entry, tracker.matchResponse(0x1234, "OnE.oNe.ONE.one", testQTypeA, now))
}

// The entry is deliberately not consumed, so a duplicated response merges idempotently.
func TestDNSRequestTrackerDoesNotConsumeOnMatch(t *testing.T) {
	tracker := newTestDNSRequestTracker(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now)

	assert.Same(t, entry, tracker.matchResponse(0x1234, "one.one.one.one", testQTypeA, now))
	assert.Same(t, entry, tracker.matchResponse(0x1234, "one.one.one.one", testQTypeA, now))
	assert.Equal(t, uint64(2), tracker.hits.Load())
}

func TestDNSRequestTrackerEvictsWhenFull(t *testing.T) {
	tracker, err := newDNSRequestTracker(2, dnsRequestTrackerTTL)
	require.NoError(t, err)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.recordRequest(1, "a.example.com", testQTypeA, entry, now)
	tracker.recordRequest(2, "b.example.com", testQTypeA, entry, now)
	tracker.recordRequest(3, "c.example.com", testQTypeA, entry, now)

	assert.Nil(t, tracker.matchResponse(1, "a.example.com", testQTypeA, now))
	assert.Same(t, entry, tracker.matchResponse(3, "c.example.com", testQTypeA, now))
}

func TestDNSRequestTrackerIgnoresNilProcessCacheEntry(t *testing.T) {
	tracker := newTestDNSRequestTracker(t)
	now := time.Now()

	tracker.recordRequest(0x1234, "one.one.one.one", testQTypeA, nil, now)

	assert.Nil(t, tracker.matchResponse(0x1234, "one.one.one.one", testQTypeA, now))
}
