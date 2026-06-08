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

// makeEngineForDedup builds a bare-minimum engine with no detectors or
// correlators, suitable for unit-testing newCorrelations in isolation.
func makeEngineForDedup() *engine {
	return newEngine(engineConfig{
		storage: newTimeSeriesStorageWith(StorageConfig{}),
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

// TestNewCorrelations_FiresOnFirstAppearance verifies that a brand-new
// correlation is included in the first call's result.
func TestNewCorrelations_FiresOnFirstAppearance(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("A", 100))

	result := e.newCorrelations([]observerdef.ActiveCorrelation{ac("A", 100)})
	assert.Contains(t, patternNames(result), "A")
}

// TestNewCorrelations_SilentOnSameTimestamp verifies that a pattern already
// reported with the same LastUpdated is not returned again.
func TestNewCorrelations_SilentOnSameTimestamp(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("A", 100))

	e.newCorrelations([]observerdef.ActiveCorrelation{ac("A", 100)}) // first call — fires
	result := e.newCorrelations([]observerdef.ActiveCorrelation{ac("A", 100)})
	assert.NotContains(t, patternNames(result), "A",
		"same-timestamp pattern must not fire a second time")
}

// TestNewCorrelations_GenuineRecurrenceWithNewTimestamp verifies that after a
// pattern goes inactive and re-fires spuriously, a genuine recurrence with a
// different LastUpdated is still detected.
func TestNewCorrelations_GenuineRecurrenceWithNewTimestamp(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("A", 100))

	// Cycle 1: A active — fires.
	e.newCorrelations([]observerdef.ActiveCorrelation{ac("A", 100)})

	e.newCorrelations(nil) // A goes inactive
	e.newCorrelations(nil) // spurious fire (seenCorrelations["A"] was deleted by cleanup)

	// Now A genuinely recurs with a new LastUpdated.
	inject(e, ac("A", 200))
	result := e.newCorrelations([]observerdef.ActiveCorrelation{ac("A", 200)})

	assert.Contains(t, patternNames(result), "A",
		"genuine recurrence (new LastUpdated) must fire even after inactive+spurious-fire period")
}

// TestNewCorrelations_FiresWhenLastUpdatedAdvancesWhileActive verifies that a
// pattern which remains continuously active but receives a new contributing
// signal (LastUpdated changes) fires again without ever going inactive.
func TestNewCorrelations_FiresWhenLastUpdatedAdvancesWhileActive(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("A", 100))
	active := []observerdef.ActiveCorrelation{ac("A", 100)}

	e.newCorrelations(active) // first fire, seenCorrelations["A"] = 100

	result := e.newCorrelations(active) // same LastUpdated — silent
	assert.NotContains(t, patternNames(result), "A")

	// A new anomaly joins the cluster: accumulatedCorrelations["A"].LastUpdated advances.
	inject(e, ac("A", 200))
	active = []observerdef.ActiveCorrelation{ac("A", 200)}

	result = e.newCorrelations(active)
	assert.Contains(t, patternNames(result), "A",
		"updated LastUpdated while still active must trigger a new emission")
	assert.Equal(t, int64(200), e.seenCorrelations["A"],
		"seenCorrelations must record the new timestamp")

	// Confirm stable on the very next cycle with the same timestamp.
	result = e.newCorrelations(active)
	assert.NotContains(t, patternNames(result), "A",
		"must be silent again after the new timestamp is recorded")
}

// TestNewCorrelations_StableAfterSpuriousFire verifies that once the spurious
// fire stores LastUpdated=100, further inactive cycles with the same timestamp
// are silent.
func TestNewCorrelations_StableAfterSpuriousFire(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("A", 100))

	e.newCorrelations([]observerdef.ActiveCorrelation{ac("A", 100)}) // first fire
	e.newCorrelations(nil)                                           // goes inactive, arms for recurrence

	spurious := e.newCorrelations(nil) // one spurious re-fire from accumulated history
	assert.Contains(t, patternNames(spurious), "A",
		"one spurious re-fire is expected on the first inactive cycle after deactivation")

	// Further inactive cycles must all be silent.
	for i := 0; i < 5; i++ {
		result := e.newCorrelations(nil)
		assert.NotContains(t, patternNames(result), "A",
			"stable same-timestamp inactive pattern must not fire after seenCorrelations is restored")
	}
}

// TestNewCorrelations_IndependentPatterns verifies that dedup state for
// pattern "A" does not affect pattern "B".
func TestNewCorrelations_IndependentPatterns(t *testing.T) {
	e := makeEngineForDedup()
	inject(e, ac("A", 100), ac("B", 100))

	// Both fire on first appearance.
	result := e.newCorrelations([]observerdef.ActiveCorrelation{ac("A", 100), ac("B", 100)})
	assert.ElementsMatch(t, []string{"A", "B"}, patternNames(result))

	// Both seen — no fire next cycle.
	result = e.newCorrelations([]observerdef.ActiveCorrelation{ac("A", 100), ac("B", 100)})
	assert.Empty(t, patternNames(result))

	// A goes inactive; B stays. Neither fires.
	result = e.newCorrelations([]observerdef.ActiveCorrelation{ac("B", 100)})
	assert.Empty(t, patternNames(result))

	// A still inactive; B must not re-fire regardless of A's spurious-fire cycle.
	result = e.newCorrelations([]observerdef.ActiveCorrelation{ac("B", 100)})
	for _, p := range patternNames(result) {
		assert.NotEqual(t, "B", p, "B must never re-fire with same timestamp")
	}

	// A genuinely recurs with a new timestamp while B stays unchanged.
	inject(e, ac("A", 200))
	result = e.newCorrelations([]observerdef.ActiveCorrelation{ac("A", 200), ac("B", 100)})
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

	e.newCorrelations([]observerdef.ActiveCorrelation{ac("A", 100)}) // fires

	result := e.newCorrelations([]observerdef.ActiveCorrelation{ac("A", 100)})
	assert.NotContains(t, patternNames(result), "A", "no fire before reset")

	// resetCorrelations alone must NOT clear dedup (replay-safe reset).
	e.resetCorrelations()
	inject(e, ac("A", 100))
	result = e.newCorrelations([]observerdef.ActiveCorrelation{ac("A", 100)})
	assert.NotContains(t, patternNames(result), "A",
		"resetCorrelations must not clear dedup — replay must not re-fire known patterns")

	// resetFull clears dedup — patterns re-fire after a full teardown.
	e.resetFull()
	inject(e, ac("A", 100))
	result = e.newCorrelations([]observerdef.ActiveCorrelation{ac("A", 100)})
	assert.Contains(t, patternNames(result), "A", "must fire again after full reset")
}
