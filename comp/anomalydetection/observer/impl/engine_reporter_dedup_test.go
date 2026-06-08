// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// Tests for engine.newCorrelations — the deduplication logic that computes
// which correlation patterns reporters should fire on.

import (
	"testing"

	"github.com/stretchr/testify/assert"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// makeEngineForDedup builds a bare-minimum engine using the default TTL and
// max-items values, suitable for unit-testing newCorrelations in isolation.
func makeEngineForDedup() *engine {
	return newEngine(engineConfig{
		storage: newTimeSeriesStorageWith(StorageConfig{}),
	})
}

// makeEngineForDedupWithTTL builds an engine with an explicit TTL for tests
// that need to exercise TTL expiry with manageable timestamp values.
func makeEngineForDedupWithTTL(ttlSec int64) *engine {
	return newEngine(engineConfig{
		storage:                newTimeSeriesStorageWith(StorageConfig{}),
		correlationDedupTTLSec: ttlSec,
	})
}

// makeEngineForDedupWithMax builds an engine with a specific max-items limit.
func makeEngineForDedupWithMax(maxItems int) *engine {
	return newEngine(engineConfig{
		storage:                  newTimeSeriesStorageWith(StorageConfig{}),
		correlationDedupTTLSec:   600,
		correlationDedupMaxItems: maxItems,
	})
}

// inject adds correlations directly to accumulatedCorrelations, simulating
// what accumulateCorrelations does during an advance.
func inject(e *engine, corrs ...observerdef.ActiveCorrelation) {
	e.correlationMu.Lock()
	defer e.correlationMu.Unlock()
	if e.accumulatedCorrelations == nil {
		e.accumulatedCorrelations = make(map[string]observerdef.ActiveCorrelation)
	}
	for _, ac := range corrs {
		e.accumulatedCorrelations[ac.Pattern] = ac
	}
}

// ac builds a minimal ActiveCorrelation.
func ac(pattern string, lastUpdated int64) observerdef.ActiveCorrelation {
	return observerdef.ActiveCorrelation{Pattern: pattern, LastUpdated: lastUpdated}
}

// patternNames extracts the Pattern fields from a slice for easy assertion.
func patternNames(corrs []observerdef.ActiveCorrelation) []string {
	out := make([]string, len(corrs))
	for i, c := range corrs {
		out[i] = c.Pattern
	}
	return out
}

// nc is a unit-test shorthand for newCorrelations where cycleActive and
// postActive are the same slice.  Unit tests do not simulate correlator.Advance
// so there is no divergence between the two sets; this avoids repeating the
// slice argument twice in every call.
func nc(e *engine, active []observerdef.ActiveCorrelation, upToSec int64) []observerdef.ActiveCorrelation {
	return e.newCorrelations(active, active, upToSec)
}

// TestNewCorrelations_FiresOnFirstAppearance verifies that a brand-new
// correlation is included in the first call's result.
func TestNewCorrelations_FiresOnFirstAppearance(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("A", 100))

	result := nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1000)
	assert.Contains(t, patternNames(result), "A")
}

// TestNewCorrelations_SilentOnSameTimestamp verifies that a pattern already
// reported with the same LastUpdated is not returned again.
func TestNewCorrelations_SilentOnSameTimestamp(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("A", 100))

	nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1000) // first call — fires
	result := nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1001)
	assert.NotContains(t, patternNames(result), "A",
		"same-timestamp pattern must not fire a second time")
}

// TestNewCorrelations_GenuineRecurrenceWithNewTimestamp verifies that a pattern
// with a changed LastUpdated fires again even while still in the TTL window.
func TestNewCorrelations_GenuineRecurrenceWithNewTimestamp(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("A", 100))

	nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1000) // fires

	// A goes inactive (but within TTL — no re-arm yet).
	nc(e, nil, 1001)

	// A genuinely recurs with a new LastUpdated — must fire regardless of TTL.
	inject(e, ac("A", 200))
	result := nc(e, []observerdef.ActiveCorrelation{ac("A", 200)}, 1002)

	assert.Contains(t, patternNames(result), "A",
		"genuine recurrence (new LastUpdated) must fire even within the TTL window")
}

// TestNewCorrelations_FiresWhenLastUpdatedAdvancesWhileActive verifies that a
// pattern which remains continuously active but receives a new contributing
// signal (LastUpdated changes) fires again without ever going inactive.
func TestNewCorrelations_FiresWhenLastUpdatedAdvancesWhileActive(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("A", 100))
	active := []observerdef.ActiveCorrelation{ac("A", 100)}

	nc(e, active, 1000) // first fire

	result := nc(e, active, 1001) // same LastUpdated — silent
	assert.NotContains(t, patternNames(result), "A")

	// A new anomaly joins the cluster: accumulatedCorrelations["A"].LastUpdated advances.
	inject(e, ac("A", 200))
	active = []observerdef.ActiveCorrelation{ac("A", 200)}

	result = nc(e, active, 1002)
	assert.Contains(t, patternNames(result), "A",
		"updated LastUpdated while still active must trigger a new emission")
	assert.Equal(t, int64(200), e.dedupTracker.entries["A"].lastUpdated,
		"tracker must record the new timestamp")

	// Confirm stable on the very next cycle with the same timestamp.
	result = nc(e, active, 1003)
	assert.NotContains(t, patternNames(result), "A",
		"must be silent again after the new timestamp is recorded")
}

// TestNewCorrelations_NoSpuriousFireWithinTTL verifies that once a pattern
// goes inactive it does NOT re-fire as long as it stays within the TTL window.
// This replaces the old StableAfterSpuriousFire test: with TTL the spurious
// re-fire on the first inactive cycle no longer occurs.
func TestNewCorrelations_NoSpuriousFireWithinTTL(t *testing.T) {
	const ttl = 60
	e := makeEngineForDedupWithTTL(ttl)
	inject(e, ac("A", 100))

	nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1000) // fires

	// Multiple inactive cycles within TTL — all must be silent.
	for i := int64(1); i < ttl; i++ {
		result := nc(e, nil, 1000+i)
		assert.NotContains(t, patternNames(result), "A",
			"must not fire within TTL window (cycle %d)", i)
	}
}

// TestNewCorrelations_RearmsAfterTTLExpiry verifies that a pattern re-fires
// once it becomes active again after having been continuously inactive for at
// least ttlSec data-time seconds.  While inactive (even post-TTL) there must
// be no spurious fire from the accumulated-correlations history.
func TestNewCorrelations_RearmsAfterTTLExpiry(t *testing.T) {
	const ttl = 60
	e := makeEngineForDedupWithTTL(ttl)
	inject(e, ac("A", 100))

	// First emission.
	nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1000)

	// A goes inactive at t=1001; remains inactive throughout.
	nc(e, nil, 1001)

	// Within TTL, still inactive — must be silent.
	result := nc(e, nil, 1000+ttl-1)
	assert.NotContains(t, patternNames(result), "A", "must still be silent just before TTL expires")

	// Past TTL, still inactive — must remain silent (no stale fire from history).
	result = nc(e, nil, 1001+ttl)
	assert.NotContains(t, patternNames(result), "A",
		"must not fire from accumulated history even after TTL — pattern is still inactive")

	// A becomes active again after TTL expiry — the dedup entry was evicted so
	// it fires as if seen for the first time.
	result = nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1002+ttl)
	assert.Contains(t, patternNames(result), "A",
		"pattern must re-fire once active again after TTL has elapsed")
}

// TestNewCorrelations_RearmedPatternBecomesActiveBeforeTTL verifies that a
// pattern coming back active before TTL expiry resets its inactiveSince clock,
// so TTL only applies to continuous inactivity.
func TestNewCorrelations_RearmedPatternBecomesActiveBeforeTTL(t *testing.T) {
	const ttl = 60
	e := makeEngineForDedupWithTTL(ttl)
	inject(e, ac("A", 100))

	nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1000) // fires

	// A goes inactive at t=1001.
	nc(e, nil, 1001)

	// A becomes active again before TTL — entry stays, no re-fire (same LastUpdated).
	result := nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1030)
	assert.NotContains(t, patternNames(result), "A",
		"re-activation within TTL must not fire (same LastUpdated)")

	// A goes inactive again at t=1031 — inactiveSince resets to 1031.
	// TTL from 1031: must not fire until 1031+ttl, not at 1001+ttl.
	nc(e, nil, 1031)
	result = nc(e, nil, 1001+ttl) // would expire if clock was still 1001, but it's not
	assert.NotContains(t, patternNames(result), "A",
		"TTL must be counted from the most recent inactiveSince, not the original one")

	// After TTL elapses from the reset inactiveSince, A becomes active again and fires.
	result = nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1031+ttl)
	assert.Contains(t, patternNames(result), "A",
		"must fire when active again after TTL elapses from the reset inactiveSince")
}

// TestNewCorrelations_IndependentPatterns verifies that dedup state for
// pattern "A" does not affect pattern "B".
func TestNewCorrelations_IndependentPatterns(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("A", 100), ac("B", 100))

	// Both fire on first appearance.
	result := nc(e, []observerdef.ActiveCorrelation{ac("A", 100), ac("B", 100)}, 1000)
	assert.ElementsMatch(t, []string{"A", "B"}, patternNames(result))

	// Both seen — no fire next cycle.
	result = nc(e, []observerdef.ActiveCorrelation{ac("A", 100), ac("B", 100)}, 1001)
	assert.Empty(t, patternNames(result))

	// A goes inactive; B stays. Neither fires within TTL.
	result = nc(e, []observerdef.ActiveCorrelation{ac("B", 100)}, 1002)
	assert.Empty(t, patternNames(result))

	// A genuinely recurs with a new timestamp while B stays unchanged.
	inject(e, ac("A", 200))
	result = nc(e, []observerdef.ActiveCorrelation{ac("A", 200), ac("B", 100)}, 1003)
	assert.Contains(t, patternNames(result), "A",
		"only A should fire on genuine recurrence")
	assert.NotContains(t, patternNames(result), "B",
		"B must not re-fire (same timestamp)")
}

// TestNewCorrelations_FullResetClearsDedup verifies that resetFull clears
// the reporter dedup state so patterns fire again after a full teardown.
// resetCorrelations alone (used for replay resets) must NOT clear dedup so
// that reporters do not re-fire already-reported patterns during replay.
func TestNewCorrelations_FullResetClearsDedup(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("A", 100))

	nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1000) // fires

	result := nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1001)
	assert.NotContains(t, patternNames(result), "A", "no fire before reset")

	// resetCorrelations alone must NOT clear dedup (replay-safe reset).
	e.resetCorrelations()
	inject(e, ac("A", 100))
	result = nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1002)
	assert.NotContains(t, patternNames(result), "A",
		"resetCorrelations must not clear dedup — replay must not re-fire known patterns")

	// resetFull clears dedup — patterns re-fire after a full teardown.
	e.resetFull()
	inject(e, ac("A", 100))
	result = nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1003)
	assert.Contains(t, patternNames(result), "A", "must fire again after full reset")
}

// TestNewCorrelations_MaxItemsEvictsOldest verifies that when the tracker
// exceeds its maxItems limit, the oldest inactive entry is evicted, making
// room for new patterns.
func TestNewCorrelations_MaxItemsEvictsOldest(t *testing.T) {
	const max = 2
	e := makeEngineForDedupWithMax(max)

	// Fill the tracker with two patterns, both going inactive.
	inject(e, ac("A", 100), ac("B", 100))
	nc(e, []observerdef.ActiveCorrelation{ac("A", 100), ac("B", 100)}, 1000)
	// A goes inactive at 1001, B goes inactive at 1002 (so A is older).
	nc(e, []observerdef.ActiveCorrelation{ac("B", 100)}, 1001) // A inactive
	nc(e, nil, 1002)                                           // B inactive

	// Now inject C. On the next newCorrelations call the tracker has 2 inactive
	// entries (A, B) and will see a new pattern (C). After markSeen(C) the map
	// has 3 entries which exceeds max=2, so evictOverLimit must drop A (oldest).
	inject(e, ac("C", 100))
	result := nc(e, []observerdef.ActiveCorrelation{ac("C", 100)}, 1003)
	assert.Contains(t, patternNames(result), "C", "new pattern C must fire")

	assert.Nil(t, e.dedupTracker.entries["A"],
		"A (oldest inactive) must have been evicted to make room for C")
	assert.NotNil(t, e.dedupTracker.entries["B"],
		"B (newer inactive) must still be tracked")
}

// TestNewCorrelations_HistoricalTimestampPatternFiresDespiteAdvanceEviction
// reproduces the scenario described in the PR review: ScanMW/ScanWelch can
// emit changepoints with timestamps hundreds of seconds behind upTo, forming
// correlations that correlator.Advance(upTo) evicts immediately.  The engine
// accumulates those correlations before Advance runs (cycleActive), so they
// must still reach reporters even though they are absent from postActive.
func TestNewCorrelations_HistoricalTimestampPatternFiresDespiteAdvanceEviction(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("hist", 100))

	// Simulate: pattern was active before Advance but is gone after it.
	cycleActive := []observerdef.ActiveCorrelation{ac("hist", 100)}
	postActive := []observerdef.ActiveCorrelation{} // evicted by Advance

	result := e.newCorrelations(cycleActive, postActive, 1000)
	assert.Contains(t, patternNames(result), "hist",
		"historical-timestamp pattern must fire even when absent from post-Advance active set")
}

// TestNewCorrelations_StaleHistoryDoesNotFireWhenNeitherCycleNorPostActive
// verifies that a pattern accumulated in a previous cycle that is not present
// in either cycleActive or postActive does not fire — only current-cycle or
// post-Advance activity unlocks emission.
func TestNewCorrelations_StaleHistoryDoesNotFireWhenNeitherCycleNorPostActive(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("A", 100))

	// First cycle: A is active and fires.
	nc(e, []observerdef.ActiveCorrelation{ac("A", 100)}, 1000)

	// Second cycle: A is absent from both cycleActive and postActive (dedup TTL
	// has not expired yet; A is simply not returned by the correlators).
	result := e.newCorrelations(nil, nil, 1001)
	assert.NotContains(t, patternNames(result), "A",
		"stale accumulated pattern must not fire when absent from both active sets")
}
