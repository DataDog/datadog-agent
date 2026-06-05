// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// schedulerPolicy decides when the engine should advance analysis.
// It is the single home for time-advancement rules.
type schedulerPolicy interface {
	// onObservation is called when a new observation arrives. Returns advance
	// requests that should be executed. The state parameter provides read-only
	// access to scheduler-relevant engine state.
	onObservation(dataTimeSec int64, st schedulerState) []advanceRequest

	// onIdle is called when wall-clock time passes without new observations.
	// Not used in current behavior, but the interface supports future periodic flushes.
	onIdle(nowUnixNano int64, st schedulerState) []advanceRequest

	// onReplayEnd is called when replay finishes. Returns final advance requests
	// to flush any remaining data.
	onReplayEnd(st schedulerState) []advanceRequest
}

// schedulerState provides read-only scheduler-relevant state.
type schedulerState struct {
	lastAnalyzedDataTime int64 // last time detectors ran up to
	latestDataTime       int64 // latest data timestamp seen
}

// advanceRequest tells the engine to advance analysis to a specific time.
type advanceRequest struct {
	upToSec int64
	reason  advanceReason
}

// advanceReason categorizes why an advance was requested.
type advanceReason int

const (
	advanceReasonInputDriven   advanceReason = iota // data arrival triggered
	advanceReasonPeriodicFlush                      // periodic timer triggered
	advanceReasonReplayEnd                          // replay finished
	advanceReasonManual                             // explicit test/debug trigger
)

// currentBehaviorPolicy reproduces the exact current scheduling semantics:
// - On observation: if dataTimeSec-1 > lastAnalyzedDataTime, emit advance to dataTimeSec-1
// - On idle: nothing (not currently used)
// - On replay end: advance to latestDataTime (for batch/testbench replay)
type currentBehaviorPolicy struct{}

func (p *currentBehaviorPolicy) onObservation(dataTimeSec int64, st schedulerState) []advanceRequest {
	analyzeUpTo := dataTimeSec - 1
	if analyzeUpTo <= st.lastAnalyzedDataTime {
		return nil
	}
	return []advanceRequest{{upToSec: analyzeUpTo, reason: advanceReasonInputDriven}}
}

func (p *currentBehaviorPolicy) onIdle(_ int64, _ schedulerState) []advanceRequest {
	return nil
}

func (p *currentBehaviorPolicy) onReplayEnd(st schedulerState) []advanceRequest {
	if st.latestDataTime <= st.lastAnalyzedDataTime {
		return nil
	}
	return []advanceRequest{{upToSec: st.latestDataTime, reason: advanceReasonReplayEnd}}
}
