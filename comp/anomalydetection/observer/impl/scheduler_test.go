// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCurrentBehaviorPolicy_OnObservation(t *testing.T) {
	p := &currentBehaviorPolicy{}

	t.Run("advances when data time is ahead", func(t *testing.T) {
		st := schedulerState{lastAnalyzedDataTime: 10, latestDataTime: 10}
		reqs := p.onObservation(15, st)
		assert.Len(t, reqs, 1)
		assert.Equal(t, int64(14), reqs[0].upToSec)
		assert.Equal(t, advanceReasonInputDriven, reqs[0].reason)
	})

	t.Run("no advance when data time equals last analyzed plus one", func(t *testing.T) {
		st := schedulerState{lastAnalyzedDataTime: 10, latestDataTime: 10}
		reqs := p.onObservation(11, st)
		assert.Nil(t, reqs)
	})

	t.Run("no advance when data time is behind", func(t *testing.T) {
		st := schedulerState{lastAnalyzedDataTime: 10, latestDataTime: 10}
		reqs := p.onObservation(5, st)
		assert.Nil(t, reqs)
	})

	t.Run("out-of-order data does not trigger advance", func(t *testing.T) {
		st := schedulerState{lastAnalyzedDataTime: 20, latestDataTime: 25}
		// Data arrives at t=15, which is behind lastAnalyzedDataTime
		reqs := p.onObservation(15, st)
		assert.Nil(t, reqs)
	})

	t.Run("sparse input triggers advance across gap", func(t *testing.T) {
		st := schedulerState{lastAnalyzedDataTime: 10, latestDataTime: 10}
		// Data jumps from t=10 to t=1000
		reqs := p.onObservation(1000, st)
		assert.Len(t, reqs, 1)
		assert.Equal(t, int64(999), reqs[0].upToSec)
	})

	t.Run("advancing timestamps produce sequential advances", func(t *testing.T) {
		st := schedulerState{lastAnalyzedDataTime: 0, latestDataTime: 0}

		reqs := p.onObservation(5, st)
		assert.Len(t, reqs, 1)
		assert.Equal(t, int64(4), reqs[0].upToSec)

		// Simulate engine advancing
		st.lastAnalyzedDataTime = 4
		st.latestDataTime = 5

		reqs = p.onObservation(6, st)
		assert.Len(t, reqs, 1)
		assert.Equal(t, int64(5), reqs[0].upToSec)
	})
}

func TestCurrentBehaviorPolicy_OnIdle(t *testing.T) {
	p := &currentBehaviorPolicy{}

	st := schedulerState{lastAnalyzedDataTime: 10, latestDataTime: 20}
	reqs := p.onIdle(99999, st)
	assert.Nil(t, reqs)
}

func TestCurrentBehaviorPolicy_OnReplayEnd(t *testing.T) {
	p := &currentBehaviorPolicy{}

	t.Run("advances to latest when data remains", func(t *testing.T) {
		st := schedulerState{lastAnalyzedDataTime: 10, latestDataTime: 20}
		reqs := p.onReplayEnd(st)
		assert.Len(t, reqs, 1)
		assert.Equal(t, int64(20), reqs[0].upToSec)
		assert.Equal(t, advanceReasonReplayEnd, reqs[0].reason)
	})

	t.Run("no advance when already caught up", func(t *testing.T) {
		st := schedulerState{lastAnalyzedDataTime: 20, latestDataTime: 20}
		reqs := p.onReplayEnd(st)
		assert.Nil(t, reqs)
	})

	t.Run("no advance when latest is behind analyzed", func(t *testing.T) {
		st := schedulerState{lastAnalyzedDataTime: 25, latestDataTime: 20}
		reqs := p.onReplayEnd(st)
		assert.Nil(t, reqs)
	})

	t.Run("no advance when both zero", func(t *testing.T) {
		st := schedulerState{lastAnalyzedDataTime: 0, latestDataTime: 0}
		reqs := p.onReplayEnd(st)
		assert.Nil(t, reqs)
	})
}
