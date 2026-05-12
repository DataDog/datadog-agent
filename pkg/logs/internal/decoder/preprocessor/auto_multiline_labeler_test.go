// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Anchoring unit tests for the AutoMultilineLabeling surface declared
// in auto_multiline_labeler.allium. Each test names the spec construct
// (@guarantee or @guidance step) it anchors so that drift in either
// direction is easy to spot during review.
//
// Property tests for the same surface live in
// auto_multiline_labeler_proptest_test.go. Token-forwarding behaviour
// is anchored separately in labeler_token_reuse_test.go.

// mockHeuristic invokes processFunc as its ProcessAndContinue body.
// Tests script chain behaviour by constructing mockHeuristics with
// deterministic processFuncs.
type mockHeuristic struct {
	processFunc func(*messageContext) bool
}

func (m *mockHeuristic) ProcessAndContinue(context *messageContext) bool {
	return m.processFunc(context)
}

// TestAutoMultilineLabeler_HeuristicReceivesRawMessage anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance step 1 — raw_message = content
//
// The HeuristicContext seen by a heuristic carries the same bytes
// passed as the `content` argument to Label.
func TestAutoMultilineLabeler_HeuristicReceivesRawMessage(t *testing.T) {
	var seen []byte
	labeler := NewLabeler([]Heuristic{
		&mockHeuristic{processFunc: func(ctx *messageContext) bool {
			seen = ctx.rawMessage
			return true
		}},
	}, nil)
	labeler.Label([]byte("test 123"), nil, nil)
	assert.Equal(t, []byte("test 123"), seen)
}

// TestAutoMultilineLabeler_DefaultLabelIsAggregate anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance step 1 — label = aggregate (the default label)
//
// When no heuristic claims the label (every labelling heuristic
// either returns false without setting label, or simply doesn't
// set it), the final label is aggregate.
func TestAutoMultilineLabeler_DefaultLabelIsAggregate(t *testing.T) {
	labeler := NewLabeler([]Heuristic{
		&mockHeuristic{processFunc: func(*messageContext) bool {
			return false
		}},
	}, nil)
	assert.Equal(t, aggregate, labeler.Label([]byte("test 123"), nil, nil))
}

// TestAutoMultilineLabeler_DefaultLabelAssignedByIsSentinel anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance step 1 — label_assigned_by = "default" (sentinel
//	                       value identifying the default assignment,
//	                       before any heuristic has claimed the label)
//
// Heuristics in a fresh chain observe label_assigned_by equal to the
// "default" sentinel; they rely on this to detect whether a prior
// heuristic has claimed the label.
func TestAutoMultilineLabeler_DefaultLabelAssignedByIsSentinel(t *testing.T) {
	var seen string
	labeler := NewLabeler([]Heuristic{
		&mockHeuristic{processFunc: func(ctx *messageContext) bool {
			seen = ctx.labelAssignedBy
			return true
		}},
	}, nil)
	labeler.Label([]byte("test 123"), nil, nil)
	assert.Equal(t, defaultLabelSource, seen)
}

// TestAutoMultilineLabeler_EmptyChainsReturnDefault anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance step 1 + step 4
//
// With both chains empty, no heuristic runs and Label returns the
// initial value of context.label — aggregate. Pins the chain-
// evaluation procedure's edge case at the lower boundary.
func TestAutoMultilineLabeler_EmptyChainsReturnDefault(t *testing.T) {
	labeler := NewLabeler(nil, nil)
	assert.Equal(t, aggregate, labeler.Label([]byte("test 123"), nil, nil))
}

// TestAutoMultilineLabeler_LabellingChainProceedsOnTrue anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance step 2 — if the return value is true, iteration
//	                       proceeds to the next Heuristic
//
// A labelling heuristic that returns true does not terminate the
// chain; subsequent heuristics run and may overwrite the label.
func TestAutoMultilineLabeler_LabellingChainProceedsOnTrue(t *testing.T) {
	labeler := NewLabeler([]Heuristic{
		&mockHeuristic{processFunc: func(ctx *messageContext) bool {
			ctx.label = startGroup
			return true
		}},
		&mockHeuristic{processFunc: func(ctx *messageContext) bool {
			ctx.label = noAggregate
			return true
		}},
	}, nil)
	assert.Equal(t, noAggregate, labeler.Label([]byte("test 123"), nil, nil))
}

// TestAutoMultilineLabeler_LabellingChainTerminatesOnFalse anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance step 2 — if the return value is false, iteration
//	                       of the labelling chain ends immediately;
//	                       subsequent labelling heuristics are NOT
//	                       invoked
//
// Verifies both halves: the first heuristic's label is preserved
// (no overwrite from a later heuristic), and the later heuristic's
// processFunc is never invoked.
func TestAutoMultilineLabeler_LabellingChainTerminatesOnFalse(t *testing.T) {
	secondCalled := false
	labeler := NewLabeler([]Heuristic{
		&mockHeuristic{processFunc: func(ctx *messageContext) bool {
			ctx.label = startGroup
			return false
		}},
		&mockHeuristic{processFunc: func(ctx *messageContext) bool {
			secondCalled = true
			ctx.label = noAggregate
			return true
		}},
	}, nil)
	result := labeler.Label([]byte("test 123"), nil, nil)
	assert.Equal(t, startGroup, result,
		"second labelling heuristic should not have overwritten the label")
	assert.False(t, secondCalled,
		"second labelling heuristic should not have been invoked after the first returned false")
}

// TestAutoMultilineLabeler_LabellingChainHonoursOrder anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance step 2 — Iterate the labelling_chain in order
//
// Labelling heuristics are invoked in the order they appear in the
// chain — not reverse order, not concurrently.
func TestAutoMultilineLabeler_LabellingChainHonoursOrder(t *testing.T) {
	var order []int
	makeRecording := func(id int) *mockHeuristic {
		return &mockHeuristic{processFunc: func(*messageContext) bool {
			order = append(order, id)
			return true
		}}
	}
	labeler := NewLabeler([]Heuristic{
		makeRecording(0), makeRecording(1), makeRecording(2),
	}, nil)
	labeler.Label([]byte("test 123"), nil, nil)
	assert.Equal(t, []int{0, 1, 2}, order)
}

// TestAutoMultilineLabeler_AnalyticsChainRunsAfterEarlyTermination anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance step 3 — The full analytics chain is always
//	                       invoked regardless of how the labelling
//	                       chain terminated
//
// Specifically: when the labelling chain terminates early via a
// false return, the analytics chain still runs in full.
func TestAutoMultilineLabeler_AnalyticsChainRunsAfterEarlyTermination(t *testing.T) {
	analyticsCalled := false
	labeler := NewLabeler([]Heuristic{
		&mockHeuristic{processFunc: func(*messageContext) bool {
			return false
		}},
	}, []Heuristic{
		&mockHeuristic{processFunc: func(*messageContext) bool {
			analyticsCalled = true
			return true
		}},
	})
	labeler.Label([]byte("test 123"), nil, nil)
	assert.True(t, analyticsCalled,
		"analytics heuristic should run even after labelling chain terminated early")
}

// TestAutoMultilineLabeler_AnalyticsChainRunsAfterExhaustion anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance step 3 — The full analytics chain is always
//	                       invoked regardless of how the labelling
//	                       chain terminated
//
// Specifically: when the labelling chain runs to natural exhaustion
// (every heuristic returns true), the analytics chain still runs.
func TestAutoMultilineLabeler_AnalyticsChainRunsAfterExhaustion(t *testing.T) {
	analyticsCalled := false
	labeler := NewLabeler([]Heuristic{
		&mockHeuristic{processFunc: func(*messageContext) bool {
			return true
		}},
	}, []Heuristic{
		&mockHeuristic{processFunc: func(*messageContext) bool {
			analyticsCalled = true
			return true
		}},
	})
	labeler.Label([]byte("test 123"), nil, nil)
	assert.True(t, analyticsCalled,
		"analytics heuristic should run when labelling chain exhausts naturally")
}

// TestAutoMultilineLabeler_AnalyticsChainReturnValueIgnored anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance step 3 — The return value is ignored; iteration
//	                       always proceeds to the next heuristic
//
// A false return from an analytics heuristic does NOT stop
// analytics-chain iteration — every subsequent analytics
// heuristic still runs.
func TestAutoMultilineLabeler_AnalyticsChainReturnValueIgnored(t *testing.T) {
	secondCalled := false
	labeler := NewLabeler(nil, []Heuristic{
		&mockHeuristic{processFunc: func(*messageContext) bool {
			return false
		}},
		&mockHeuristic{processFunc: func(*messageContext) bool {
			secondCalled = true
			return true
		}},
	})
	labeler.Label([]byte("test 123"), nil, nil)
	assert.True(t, secondCalled,
		"second analytics heuristic should run despite first returning false")
}

// TestAutoMultilineLabeler_AnalyticsChainHonoursOrder anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance step 3 — Iterate the analytics_chain in order
//
// Analytics heuristics are invoked in the order they appear in the
// chain.
func TestAutoMultilineLabeler_AnalyticsChainHonoursOrder(t *testing.T) {
	var order []int
	makeRecording := func(id int) *mockHeuristic {
		return &mockHeuristic{processFunc: func(*messageContext) bool {
			order = append(order, id)
			return true
		}}
	}
	labeler := NewLabeler(nil, []Heuristic{
		makeRecording(0), makeRecording(1), makeRecording(2),
	})
	labeler.Label([]byte("test 123"), nil, nil)
	assert.Equal(t, []int{0, 1, 2}, order)
}

// TestAutoMultilineLabeler_AnalyticsChainRunsAfterLabelling anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance step 3 — analytics_chain iteration follows
//	                       labelling_chain iteration
//
// The labelling chain runs to completion (or terminates) before
// any analytics heuristic is invoked.
func TestAutoMultilineLabeler_AnalyticsChainRunsAfterLabelling(t *testing.T) {
	var order []string
	labellingH := &mockHeuristic{processFunc: func(*messageContext) bool {
		order = append(order, "labelling")
		return true
	}}
	analyticsH := &mockHeuristic{processFunc: func(*messageContext) bool {
		order = append(order, "analytics")
		return true
	}}
	labeler := NewLabeler([]Heuristic{labellingH}, []Heuristic{analyticsH})
	labeler.Label([]byte("test 123"), nil, nil)
	assert.Equal(t, []string{"labelling", "analytics"}, order)
}

// TestAutoMultilineLabeler_AnalyticsChainMayModifyLabel anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance final paragraph — The labelling-vs-analytics
//	                                 distinction is one of caller
//	                                 intent rather than enforced
//	                                 behaviour: any Heuristic
//	                                 placed in the analytics_chain
//	                                 is contractually permitted to
//	                                 modify context.label
//
// Verifies that a label modification by an analytics heuristic
// IS reflected in the labeller's return value. The convention is
// that analytics heuristics don't modify, but the surface places
// no enforcement.
func TestAutoMultilineLabeler_AnalyticsChainMayModifyLabel(t *testing.T) {
	labeler := NewLabeler(nil, []Heuristic{
		&mockHeuristic{processFunc: func(ctx *messageContext) bool {
			ctx.label = noAggregate
			ctx.labelAssignedBy = "test_analytics"
			return false
		}},
	})
	assert.Equal(t, noAggregate, labeler.Label([]byte("test 123"), nil, nil))
}

// TestAutoMultilineLabeler_ReturnsContextLabel anchors:
//
//	surface AutoMultilineLabeling (auto_multiline_labeler.allium)
//	    @guidance step 4 — Return context.label as the result of
//	                       label()
//
// The labeller returns the final value of context.label after all
// chain iteration completes — not an internal cached copy, and
// not a value computed independently of context. A later
// modification by an analytics heuristic is therefore visible in
// the return value (per the labelling-vs-analytics paragraph).
func TestAutoMultilineLabeler_ReturnsContextLabel(t *testing.T) {
	labeler := NewLabeler([]Heuristic{
		&mockHeuristic{processFunc: func(ctx *messageContext) bool {
			ctx.label = startGroup
			return true
		}},
	}, []Heuristic{
		&mockHeuristic{processFunc: func(ctx *messageContext) bool {
			ctx.label = noAggregate
			return true
		}},
	})
	assert.Equal(t, noAggregate, labeler.Label([]byte("test 123"), nil, nil))
}
