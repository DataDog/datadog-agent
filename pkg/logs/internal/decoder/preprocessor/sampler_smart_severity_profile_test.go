// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
)

// activateSeverity returns a SeverityProvider (to be assigned to
// AdaptiveSamplerConfig.SeverityProvider) and an emit function that simulates severity
// transitions, without depending on a real severity reader/scorer.
func activateSeverity() (provider func() (severityeventsdef.SeverityLevel, bool), emit func(severityeventsdef.SeverityLevel)) {
	var level severityeventsdef.SeverityLevel
	var active bool
	return func() (severityeventsdef.SeverityLevel, bool) {
			return level, active
		}, func(toLevel severityeventsdef.SeverityLevel) {
			level = toLevel
			active = true
		}
}

// testProfiles mirrors Low into Medium and makes High much more permissive,
// so escalation is easy to observe.
func testProfiles() [severityeventsdef.NumSeverityLevels]SamplerProfile {
	return [severityeventsdef.NumSeverityLevels]SamplerProfile{
		severityeventsdef.SeverityLow:    {RateLimit: 1, BurstSize: 5},
		severityeventsdef.SeverityMedium: {RateLimit: 1, BurstSize: 5},
		severityeventsdef.SeverityHigh:   {RateLimit: 10, BurstSize: 100},
	}
}

func newAnomalyProfileSampler(profiles [severityeventsdef.NumSeverityLevels]SamplerProfile, provider func() (severityeventsdef.SeverityLevel, bool)) *AdaptiveSampler {
	low := profiles[severityeventsdef.SeverityLow]
	return NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:                  10,
		RateLimit:                    low.RateLimit,
		BurstSize:                    low.BurstSize,
		MatchThreshold:               0.9,
		SmartSeverityProfilesEnabled: true,
		Profiles:                     profiles,
		SeverityProvider:             provider,
	}, "test", 0)
}

func TestAdaptiveSampler_SmartSeverityProfilesDisabled_IgnoresPublishedLevel(t *testing.T) {
	provider, emit := activateSeverity()
	emit(severityeventsdef.SeverityHigh)

	s := newSampler(10, 5, 1)
	s.config.SeverityProvider = provider
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	require.NotNil(t, s.Process(testMsg(), patternA))
	assert.Equal(t, 1.0, s.config.RateLimit)
	assert.Equal(t, 5.0, s.config.BurstSize)
}

func TestAdaptiveSampler_NewSamplerPicksUpActiveLevelOnFirstMessage(t *testing.T) {
	provider, emit := activateSeverity()
	emit(severityeventsdef.SeverityHigh)

	s := newAnomalyProfileSampler(testProfiles(), provider)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	require.NotNil(t, s.Process(testMsg(), patternA))
	assert.Equal(t, 10.0, s.config.RateLimit)
	assert.Equal(t, 100.0, s.config.BurstSize)
}

func TestAdaptiveSampler_EscalationGrantsFreshBurstImmediately(t *testing.T) {
	provider, emit := activateSeverity()
	emit(severityeventsdef.SeverityLow)

	s := newAnomalyProfileSampler(testProfiles(), provider)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	for i := 0; i < 5; i++ {
		require.NotNilf(t, s.Process(testMsg(), patternA), "message %d should fit in the initial burst", i)
	}
	require.Nil(t, s.Process(testMsg(), patternA), "burst is exhausted")

	emit(severityeventsdef.SeverityHigh)

	out := s.Process(testMsg(), patternA)
	require.NotNil(t, out, "fresh burst applied before matching, despite zero elapsed time")
	assert.Equal(t, 10.0, s.config.RateLimit)
	assert.Equal(t, 100.0, s.config.BurstSize)
}

func TestAdaptiveSampler_FirstAvailableHigherSeverityGrantsFreshBurst(t *testing.T) {
	provider, emit := activateSeverity()
	s := newAnomalyProfileSampler(testProfiles(), provider)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	// Before a severity is available, the sampler uses the base Low profile and
	// can exhaust an existing pattern's credits.
	for i := 0; i < 5; i++ {
		require.NotNilf(t, s.Process(testMsg(), patternA), "message %d should fit in the base burst", i)
	}
	require.Nil(t, s.Process(testMsg(), patternA), "base burst is exhausted")
	require.False(t, s.appliedLevelInitialized, "an unavailable provider must not be treated as a reported Low severity")

	// The first available High profile is an escalation from the effective base
	// Low profile, so existing entries receive High's burst immediately.
	emit(severityeventsdef.SeverityHigh)
	require.NotNil(t, s.Process(testMsg(), patternA))
	assert.True(t, s.appliedLevelInitialized)
	assert.Equal(t, severityeventsdef.SeverityHigh, s.appliedLevel)
	assert.InDelta(t, 99.0, s.entries[0].credits, 0.0001)
}

func TestAdaptiveSampler_DeescalationClampsCreditsNaturallyWithoutForcedReset(t *testing.T) {
	provider, emit := activateSeverity()
	emit(severityeventsdef.SeverityHigh)

	s := newAnomalyProfileSampler(testProfiles(), provider)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	require.NotNil(t, s.Process(testMsg(), patternA))
	require.Len(t, s.entries, 1)
	require.InDelta(t, 99.0, s.entries[0].credits, 0.0001, "new pattern seeds BurstSize-1 credits")

	emit(severityeventsdef.SeverityLow)

	// No forced reset: the refill-time clamp in processMatchedEntry shrinks
	// credits to the new BurstSize on the next match instead.
	out := s.Process(testMsg(), patternA)
	require.NotNil(t, out, "clamped credits (5) still satisfy the >=1 check")
	assert.Equal(t, 1.0, s.config.RateLimit)
	assert.Equal(t, 5.0, s.config.BurstSize)
	assert.InDelta(t, 4.0, s.entries[0].credits, 0.0001, "clamped to 5, then one credit spent")
}

func TestAdaptiveSampler_EscalationResetsEveryTrackedPattern(t *testing.T) {
	provider, emit := activateSeverity()
	emit(severityeventsdef.SeverityLow)

	s := newAnomalyProfileSampler(testProfiles(), provider)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	require.NotNil(t, s.Process(testMsg(), patternA))
	require.NotNil(t, s.Process(testMsg(), patternB))
	require.Len(t, s.entries, 2)

	emit(severityeventsdef.SeverityHigh)
	// Escalation resets every tracked pattern, not just the matched one.
	require.NotNil(t, s.Process(testMsg(), patternA))

	require.Len(t, s.entries, 2)
	assert.InDelta(t, 99.0, s.entries[0].credits, 0.0001, "patternA: reset to 100, then one credit spent")
	assert.InDelta(t, 100.0, s.entries[1].credits, 0.0001, "patternB: reset to 100, untouched by the match")
}

func TestAdaptiveSampler_MultipleStepTransitionsApplyEachProfile(t *testing.T) {
	provider, emit := activateSeverity()
	emit(severityeventsdef.SeverityLow)

	profiles := [severityeventsdef.NumSeverityLevels]SamplerProfile{
		severityeventsdef.SeverityLow:    {RateLimit: 1, BurstSize: 5},
		severityeventsdef.SeverityMedium: {RateLimit: 4, BurstSize: 20},
		severityeventsdef.SeverityHigh:   {RateLimit: 10, BurstSize: 100},
	}
	s := newAnomalyProfileSampler(profiles, provider)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	require.NotNil(t, s.Process(testMsg(), patternA))
	require.Len(t, s.entries, 1)
	assert.Equal(t, 1.0, s.config.RateLimit)
	assert.Equal(t, 5.0, s.config.BurstSize)
	assert.InDelta(t, 4.0, s.entries[0].credits, 0.0001)

	emit(severityeventsdef.SeverityMedium)
	require.NotNil(t, s.Process(testMsg(), patternA))
	assert.Equal(t, 4.0, s.config.RateLimit)
	assert.Equal(t, 20.0, s.config.BurstSize)
	assert.InDelta(t, 19.0, s.entries[0].credits, 0.0001, "escalation to medium resets burst, then spends one credit")

	emit(severityeventsdef.SeverityHigh)
	require.NotNil(t, s.Process(testMsg(), patternA))
	assert.Equal(t, 10.0, s.config.RateLimit)
	assert.Equal(t, 100.0, s.config.BurstSize)
	assert.InDelta(t, 99.0, s.entries[0].credits, 0.0001, "escalation to high resets burst, then spends one credit")

	emit(severityeventsdef.SeverityMedium)
	require.NotNil(t, s.Process(testMsg(), patternA))
	assert.Equal(t, 4.0, s.config.RateLimit)
	assert.Equal(t, 20.0, s.config.BurstSize)
	assert.InDelta(t, 19.0, s.entries[0].credits, 0.0001, "de-escalation to medium clamps to new burst, then spends one credit")

	emit(severityeventsdef.SeverityLow)
	require.NotNil(t, s.Process(testMsg(), patternA))
	assert.Equal(t, 1.0, s.config.RateLimit)
	assert.Equal(t, 5.0, s.config.BurstSize)
	assert.InDelta(t, 4.0, s.entries[0].credits, 0.0001, "de-escalation to low clamps again, then spends one credit")
}

func TestAdaptiveSampler_NewPatternAfterSeverityChangeUsesActiveProfile(t *testing.T) {
	provider, emit := activateSeverity()
	emit(severityeventsdef.SeverityLow)

	s := newAnomalyProfileSampler(testProfiles(), provider)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	require.NotNil(t, s.Process(testMsg(), patternA))
	require.Len(t, s.entries, 1)
	assert.InDelta(t, 4.0, s.entries[0].credits, 0.0001, "low profile seeds BurstSize-1 credits")

	emit(severityeventsdef.SeverityHigh)
	require.NotNil(t, s.Process(testMsg(), patternB))

	require.Len(t, s.entries, 2)
	assert.Equal(t, 10.0, s.config.RateLimit)
	assert.Equal(t, 100.0, s.config.BurstSize)
	assert.InDelta(t, 100.0, s.entries[0].credits, 0.0001, "existing pattern is reset on escalation before the new pattern is tracked")
	assert.InDelta(t, 99.0, s.entries[1].credits, 0.0001, "new pattern seeds High BurstSize-1 credits after the severity change")
}

func TestAdaptiveSampler_PassThroughKeepsTrackedPatternsAtMaxCredits(t *testing.T) {
	provider, emit := activateSeverity()
	profiles := testProfiles()
	profiles[severityeventsdef.SeverityHigh].PassThrough = true
	emit(severityeventsdef.SeverityHigh)

	s := newAnomalyProfileSampler(profiles, provider)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	for i := 0; i < 3; i++ {
		require.NotNilf(t, s.Process(testMsg(), patternA), "message %d should pass through", i)
	}

	require.Len(t, s.entries, 1)
	assert.True(t, s.config.PassThrough)
	assert.InDelta(t, 100.0, s.entries[0].credits, 0.0001, "pass-through should behave like an infinite rate limit and keep credits full")
}

func TestAdaptiveSampler_DeescalationFromPassThroughSeedsEveryTrackedPatternWithMaxCredits(t *testing.T) {
	provider, emit := activateSeverity()
	profiles := testProfiles()
	profiles[severityeventsdef.SeverityHigh].PassThrough = true
	emit(severityeventsdef.SeverityHigh)

	s := newAnomalyProfileSampler(profiles, provider)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	require.NotNil(t, s.Process(testMsg(), patternA))
	require.NotNil(t, s.Process(testMsg(), patternB))
	require.Len(t, s.entries, 2)
	assert.InDelta(t, 100.0, s.entries[0].credits, 0.0001)
	assert.InDelta(t, 100.0, s.entries[1].credits, 0.0001)
	s.entries[0].sampled = 3
	s.entries[1].sampled = 4

	emit(severityeventsdef.SeverityLow)

	require.NotNil(t, s.Process(testMsg(), patternA))
	require.Len(t, s.entries, 2)
	assert.False(t, s.config.PassThrough)
	assert.Equal(t, 1.0, s.config.RateLimit)
	assert.Equal(t, 5.0, s.config.BurstSize)
	assert.InDelta(t, 4.0, s.entries[0].credits, 0.0001, "matched pattern should be reset to the new burst size, then spend one credit")
	assert.InDelta(t, 5.0, s.entries[1].credits, 0.0001, "unmatched patterns should still be reseeded to the new burst size")
	assert.Zero(t, s.entries[0].sampled, "matched pattern stale sampled count should be cleared when pass-through ends")
	assert.Zero(t, s.entries[1].sampled, "unmatched pattern stale sampled count should be cleared when pass-through ends")
}
