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
	severityeventsimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/impl"
	"github.com/DataDog/datadog-agent/pkg/logs/dynamicadaptivesampling"
)

// fakeScorer captures the listener passed to SubscribeSeverityEvents so tests can
// simulate transitions directly, without a real anomaly_scorer.
type fakeScorer struct {
	listener severityeventsdef.SeverityEventListener
}

func (f *fakeScorer) SubscribeSeverityEvents(_ severityeventsdef.SeverityEventsConfiguration, listener severityeventsdef.SeverityEventListener) (severityeventsdef.SeverityEventsSubscription, error) {
	f.listener = listener
	return severityeventsdef.SeverityEventsSubscription{Unsubscribe: func() {}}, nil
}

func (f *fakeScorer) SubscribeSeverityEventsReader(cfg severityeventsdef.SeverityEventsConfiguration) (severityeventsdef.SeverityEventsReaderSubscription, error) {
	return severityeventsimpl.NewSeverityReader(f, cfg)
}

func (f *fakeScorer) emit(toLevel severityeventsdef.SeverityLevel) {
	f.listener.OnSeverityTransition(severityeventsdef.SeverityEvent{ToLevel: toLevel})
}

// activateSeverity registers a fresh reader (backed by a fake scorer) as the
// active adaptivesampling reader for this test, and returns an emit function
// to simulate severity transitions.
func activateSeverity(t *testing.T) func(severityeventsdef.SeverityLevel) {
	fake := &fakeScorer{}
	sub, err := severityeventsimpl.NewSeverityReader(fake, severityeventsdef.SeverityEventsConfiguration{})
	require.NoError(t, err)
	dynamicadaptivesampling.SetReader(sub.Reader)
	t.Cleanup(func() { dynamicadaptivesampling.SetReader(nil) })
	return fake.emit
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

func newAnomalyProfileSampler(profiles [severityeventsdef.NumSeverityLevels]SamplerProfile) *AdaptiveSampler {
	low := profiles[severityeventsdef.SeverityLow]
	return NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:                  10,
		RateLimit:                    low.RateLimit,
		BurstSize:                    low.BurstSize,
		MatchThreshold:               0.9,
		SmartSeverityProfilesEnabled: true,
		Profiles:                     profiles,
	}, "test", 0)
}

func TestAdaptiveSampler_SmartSeverityProfilesDisabled_IgnoresPublishedLevel(t *testing.T) {
	emit := activateSeverity(t)
	emit(severityeventsdef.SeverityHigh)

	s := newSampler(10, 5, 1)
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	require.NotNil(t, s.Process(testMsg(), patternA))
	assert.Equal(t, 1.0, s.config.RateLimit)
	assert.Equal(t, 5.0, s.config.BurstSize)
}

func TestAdaptiveSampler_NewSamplerPicksUpActiveLevelOnFirstMessage(t *testing.T) {
	emit := activateSeverity(t)
	emit(severityeventsdef.SeverityHigh)

	s := newAnomalyProfileSampler(testProfiles())
	t0 := time.Now()
	s.now = func() time.Time { return t0 }

	require.NotNil(t, s.Process(testMsg(), patternA))
	assert.Equal(t, 10.0, s.config.RateLimit)
	assert.Equal(t, 100.0, s.config.BurstSize)
}

func TestAdaptiveSampler_EscalationGrantsFreshBurstImmediately(t *testing.T) {
	emit := activateSeverity(t)
	emit(severityeventsdef.SeverityLow)

	s := newAnomalyProfileSampler(testProfiles())
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

func TestAdaptiveSampler_DeescalationClampsCreditsNaturallyWithoutForcedReset(t *testing.T) {
	emit := activateSeverity(t)
	emit(severityeventsdef.SeverityHigh)

	s := newAnomalyProfileSampler(testProfiles())
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
	emit := activateSeverity(t)
	emit(severityeventsdef.SeverityLow)

	s := newAnomalyProfileSampler(testProfiles())
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
