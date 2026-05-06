// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// Anchoring unit tests for the DetectingAggregation surface
// declared in detecting_aggregator.allium. Each test names the
// spec construct (@guarantee or @guidance step) it anchors so that
// drift in either direction is easy to spot during review.
//
// Property tests for the same surface live in
// detecting_aggregator_proptest_test.go. The bulk of the
// scenario coverage (label dispatch, detection tagging, truncation
// flow) lives in aggregator_test.go's Test_DetectingAggregator*
// tests with their own anchoring docstrings; the tests in THIS
// file cover guarantees those existing tests don't yet anchor.

func newDetectingAggregator(_ *testing.T, lineLimit int, tagTruncated bool) Aggregator {
	return NewDetectingAggregator(status.NewInfoRegistry(), lineLimit, tagTruncated, false)
}

// TestDetectingAggregator_LabelDriven anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee LabelDriven — output emissions are determined
//	                              by the (label, state) tuple;
//	                              contradicts LabelIgnored from
//	                              PassThroughAggregator and
//	                              RegexAggregator.
//
// Identical content delivered under different labels produces
// observably different output sequences. This is the explicit
// counter to PassThrough/Regex's LabelIgnored:
//
//   - "start_group" then "aggregate" → two emissions, first
//     with the detection tag.
//   - "no_aggregate" then "no_aggregate" → two emissions, no
//     detection tag on either.
//
// The bytes-of-content are identical between runs; only the
// label sequence differs. Anything but a label-observing
// aggregator would emit identical outputs.
func TestDetectingAggregator_LabelDriven(t *testing.T) {
	const detectionTag = "auto_multiline_detected:true"

	t.Run("start_group then aggregate emits with detection tag", func(t *testing.T) {
		ag := newDetectingAggregator(t, 100, false)
		require.Empty(t, processMsg(ag, newMessage("A"), startGroup))
		msgs := processMsg(ag, newMessage("B"), aggregate)
		require.Len(t, msgs, 2)
		assert.Contains(t, msgs[0].ParsingExtra.Tags, detectionTag)
		assert.NotContains(t, msgs[1].ParsingExtra.Tags, detectionTag)
	})

	t.Run("no_aggregate then no_aggregate emits without detection tag", func(t *testing.T) {
		ag := newDetectingAggregator(t, 100, false)
		first := processMsg(ag, newMessage("A"), noAggregate)
		require.Len(t, first, 1)
		assert.NotContains(t, first[0].ParsingExtra.Tags, detectionTag)
		second := processMsg(ag, newMessage("B"), noAggregate)
		require.Len(t, second, 1)
		assert.NotContains(t, second[0].ParsingExtra.Tags, detectionTag)
	})
}

// TestDetectingAggregator_NoLineCombination anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee NoLineCombination — Every emitted
//	                                    AggregatedMessageWithTokens
//	                                    carries the content of
//	                                    exactly one input message.
//	                                    DetectingAggregator never
//	                                    combines content bytes from
//	                                    multiple input lines into
//	                                    one emission.
//
// Across a mixed-label sequence, total emissions (across all
// process calls plus final flush) equals total input calls, and
// each emission's trim-spaced content equals exactly one input's
// trim-spaced content. No emission's content contains the
// concatenation of multiple inputs.
func TestDetectingAggregator_NoLineCombination(t *testing.T) {
	ag := newDetectingAggregator(t, 100, false)

	inputs := []struct {
		content string
		label   Label
	}{
		{"first", startGroup},
		{"second", aggregate},
		{"third", noAggregate},
		{"fourth", startGroup},
		{"fifth", aggregate},
	}

	var emitted []string
	for _, in := range inputs {
		for _, m := range processMsg(ag, newMessage(in.content), in.label) {
			emitted = append(emitted, string(m.GetContent()))
		}
	}
	for _, m := range flushMsgs(ag) {
		emitted = append(emitted, string(m.GetContent()))
	}

	// Total emissions == total inputs.
	require.Equal(t, len(inputs), len(emitted),
		"total emissions must equal total input count (no combination, no drops)")

	// Each emission's content equals exactly one input's content
	// (in arrival order — DetectingAggregator preserves order).
	for i, in := range inputs {
		assert.Equal(t, in.content, emitted[i],
			"emission %d must carry exactly one input's content; got %q expected %q", i, emitted[i], in.content)
	}
}

// TestDetectingAggregator_TokensFromSameCall anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee TokensFromSameCall — The tokens emitted on each
//	                                     AggregatedMessageWithTokens
//	                                     are the tokens passed to
//	                                     process alongside the SAME
//	                                     input message whose
//	                                     content the emission
//	                                     carries.
//
// Distinct token sequences attached to each input call. After
// the test runs, each emission's tokens match exactly the tokens
// passed in the call whose content the emission carries — not
// the call that triggered emission (when those differ, as in
// start_group → aggregate).
func TestDetectingAggregator_TokensFromSameCall(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 100, false, false)

	tokensA := []Token{D4, Space, C5}
	tokensB := []Token{C5, Space, D1}
	tokensC := []Token{D2}

	// start_group A buffers, no emission.
	emitted := ag.Process(newMessage("A"), startGroup, tokensA)
	require.Empty(t, emitted)

	// aggregate B emits A (with A's tokens, plus detection tag) then B (with B's tokens).
	emitted = ag.Process(newMessage("B"), aggregate, tokensB)
	require.Len(t, emitted, 2)
	assert.Equal(t, tokensA, emitted[0].Tokens, "emission of buffered start_group must carry that call's tokens, not the triggering aggregate's tokens")
	assert.Equal(t, tokensB, emitted[1].Tokens, "emission of aggregate must carry that call's tokens")

	// no_aggregate C emits C (with C's tokens).
	emitted = ag.Process(newMessage("C"), noAggregate, tokensC)
	require.Len(t, emitted, 1)
	assert.Equal(t, tokensC, emitted[0].Tokens)
}

// TestDetectingAggregator_FlushIdempotentOnEmpty anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee FlushDrainsBuffer — A flush call on an aggregator
//	                                    with no pending message
//	                                    returns an empty sequence
//	                                    and changes no observable
//	                                    state.
//
// Second consecutive flush returns empty; is_empty remains true.
func TestDetectingAggregator_FlushIdempotentOnEmpty(t *testing.T) {
	ag := newDetectingAggregator(t, 100, false)

	// Empty at construction.
	assert.Empty(t, flushMsgs(ag))
	assert.True(t, ag.IsEmpty())

	// Buffer + flush.
	require.Empty(t, processMsg(ag, newMessage("pending"), startGroup))
	require.Len(t, flushMsgs(ag), 1)
	assert.True(t, ag.IsEmpty())

	// Second flush on already-empty.
	assert.Empty(t, flushMsgs(ag))
	assert.True(t, ag.IsEmpty())
}
