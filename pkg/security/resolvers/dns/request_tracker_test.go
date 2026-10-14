// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package dns

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

func newTestRequestTracker(t *testing.T) *RequestTracker {
	t.Helper()
	tracker, err := NewRequestTracker()
	require.NoError(t, err)
	return tracker
}

// matchEntry returns only the process MatchResponse attributes the response to
func matchEntry(tracker *RequestTracker, id uint16, name string, qtype uint16, now time.Time) *model.ProcessCacheEntry {
	entry, _ := tracker.MatchResponse(id, name, qtype, now)
	return entry
}

func TestRequestTrackerMatchesRecordedRequest(t *testing.T) {
	tracker := newTestRequestTracker(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now)

	assert.Same(t, entry, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeA, now))
	assert.Equal(t, uint64(1), tracker.hits.Load())
}

func TestRequestTrackerMissesUnknownKey(t *testing.T) {
	tracker := newTestRequestTracker(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now)

	// wrong transaction ID
	assert.Nil(t, matchEntry(tracker, 0x9999, "one.one.one.one", testQTypeA, now))
	// wrong name
	assert.Nil(t, matchEntry(tracker, 0x1234, "example.com", testQTypeA, now))
	assert.Equal(t, uint64(2), tracker.misses.Load())
}

func TestRequestTrackerExpiresAfterTTL(t *testing.T) {
	tracker := newTestRequestTracker(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now)

	assert.Nil(t, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeA, now.Add(requestTrackerTTL)))
	assert.Equal(t, uint64(1), tracker.misses.Load())
}

// A colliding transaction ID must lose the answer, never attribute it to the wrong process.
func TestRequestTrackerCollisionPoisonsTheKey(t *testing.T) {
	tracker := newTestRequestTracker(t)
	first := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	second := model.NewPlaceholderProcessCacheEntry(43, 43, false)
	now := time.Now()

	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, first, now)
	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, second, now)

	assert.Nil(t, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeA, now))
	assert.Equal(t, uint64(1), tracker.collisions.Load())
}

// The tombstone must cover the matching window of the latest colliding request, or a later request
// reopens the key while an earlier one is still in flight and receives its answers.
func TestRequestTrackerPoisonCoversLatestCollidingRequest(t *testing.T) {
	tracker := newTestRequestTracker(t)
	a := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	b := model.NewPlaceholderProcessCacheEntry(43, 43, false)
	c := model.NewPlaceholderProcessCacheEntry(44, 44, false)
	now := time.Now()

	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, a, now)
	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, b, now.Add(4*time.Second))
	// past a's deadline, while b is still in flight
	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, c, now.Add(5100*time.Millisecond))

	// b's response must not be attributed to c
	assert.Nil(t, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeA, now.Add(5200*time.Millisecond)))
	assert.Equal(t, uint64(2), tracker.collisions.Load())

	// once the latest colliding request, c, is stale, the key can be claimed again
	later := now.Add(5100*time.Millisecond + requestTrackerTTL)
	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, c, later)
	assert.Same(t, c, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeA, later))
}

// Some stub resolvers retransmit with the same transaction ID; that is not a collision.
func TestRequestTrackerRetransmitRefreshesDeadline(t *testing.T) {
	tracker := newTestRequestTracker(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now)
	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now.Add(4*time.Second))

	assert.Same(t, entry, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeA, now.Add(6*time.Second)))
	assert.Equal(t, uint64(0), tracker.collisions.Load())
}

// nslookup issues A and AAAA for the same name; they must correlate independently.
func TestRequestTrackerSeparatesQueryTypes(t *testing.T) {
	tracker := newTestRequestTracker(t)
	a := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	aaaa := model.NewPlaceholderProcessCacheEntry(43, 43, false)
	now := time.Now()

	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, a, now)
	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeAAAA, aaaa, now)

	assert.Same(t, a, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeA, now))
	assert.Same(t, aaaa, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeAAAA, now))
	assert.Equal(t, uint64(0), tracker.collisions.Load())
}

// Resolvers may apply 0x20 randomisation and echo the question with randomised capitalisation.
func TestRequestTrackerMatchesCaseInsensitively(t *testing.T) {
	tracker := newTestRequestTracker(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now)

	matched, name := tracker.MatchResponse(0x1234, "OnE.oNe.ONE.one", testQTypeA, now)
	assert.Same(t, entry, matched)
	// the response is filed under the request's spelling, so that it lands on the same tree node
	assert.Equal(t, "one.one.one.one", name)
}

// The entry is deliberately not consumed, so a duplicated response merges idempotently.
func TestRequestTrackerDoesNotConsumeOnMatch(t *testing.T) {
	tracker := newTestRequestTracker(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, entry, now)

	assert.Same(t, entry, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeA, now))
	assert.Same(t, entry, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeA, now))
	assert.Equal(t, uint64(2), tracker.hits.Load())
}

func TestRequestTrackerEvictsWhenFull(t *testing.T) {
	tracker, err := newRequestTracker(2, requestTrackerTTL)
	require.NoError(t, err)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	now := time.Now()

	tracker.RecordRequest(1, "a.example.com", testQTypeA, entry, now)
	tracker.RecordRequest(2, "b.example.com", testQTypeA, entry, now)
	tracker.RecordRequest(3, "c.example.com", testQTypeA, entry, now)

	assert.Nil(t, matchEntry(tracker, 1, "a.example.com", testQTypeA, now))
	assert.Same(t, entry, matchEntry(tracker, 3, "c.example.com", testQTypeA, now))
}

func TestRequestTrackerIgnoresNilProcessCacheEntry(t *testing.T) {
	tracker := newTestRequestTracker(t)
	now := time.Now()

	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, nil, now)

	assert.Nil(t, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeA, now))
}

// A tombstone must keep rejecting through the whole intermediate window, not only once its
// deadline has passed. Ambiguity resolving as a miss is the safety property the feature rests on.
func TestRequestTrackerPoisonHoldsThroughIntermediateCollisions(t *testing.T) {
	tracker := newTestRequestTracker(t)
	first := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	second := model.NewPlaceholderProcessCacheEntry(43, 43, false)
	third := model.NewPlaceholderProcessCacheEntry(44, 44, false)
	now := time.Now()

	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, first, now)
	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, second, now.Add(time.Second))

	// still poisoned partway through the original entry's life
	assert.Nil(t, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeA, now.Add(2*time.Second)))

	tracker.RecordRequest(0x1234, "one.one.one.one", testQTypeA, third, now.Add(3*time.Second))

	// a third colliding request must not lift the tombstone or hand the answer to anyone
	assert.Nil(t, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeA, now.Add(4*time.Second)))
	assert.Equal(t, uint64(2), tracker.collisions.Load())
}

// Every DNS response on the host goes through MatchResponse; when nothing is tracked none of them
// could match, and counting them as misses would pin the metric at ~100%.
func TestRequestTrackerDoesNotCountMissesWhenIdle(t *testing.T) {
	tracker := newTestRequestTracker(t)

	assert.Nil(t, matchEntry(tracker, 0x1234, "one.one.one.one", testQTypeA, time.Now()))
	assert.Equal(t, uint64(0), tracker.misses.Load())
}
