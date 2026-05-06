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

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// Anchoring unit tests for the CombiningAggregation surface
// declared in combining_aggregator.allium. Each test names the
// spec construct (@guarantee or @guidance step) it anchors so that
// drift in either direction is easy to spot during review.
//
// Property tests for the same surface live in
// combining_aggregator_proptest_test.go. The bulk of the scenario
// coverage (per-label dispatch, overflow explosion, truncation
// flow, carry semantics) lives in aggregator_test.go's
// CombiningAggregator-related tests with their own anchoring
// docstrings; the tests in THIS file cover guarantees those
// existing tests don't yet anchor.

// TestCombiningAggregator_TokensFromAggregateLeader_Combined anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee TokensFromAggregateLeader — A combined
//	                                            emission's tokens
//	                                            are the tokens of
//	                                            the FIRST line of
//	                                            the bucket.
//
// On the combined-flush path (NOT the explosion path which the
// existing TestOverflowedGroupEmitsOriginalTokens covers), the
// emitted message's tokens are exactly the start_group leader's
// tokens — not the last continuation's, not a concatenation.
func TestCombiningAggregator_TokensFromAggregateLeader_Combined(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	leaderTokens := []Token{D4, Space, C5}
	continuationTokens := []Token{C5, Space, D1}

	// Build a 2-line bucket via startGroup + aggregate.
	require.Empty(t, ag.Process(newMessage("leader"), startGroup, leaderTokens))
	require.Empty(t, ag.Process(newMessage("cont"), aggregate, continuationTokens))

	// Force flush → combined emission.
	emitted := ag.Flush()
	require.Len(t, emitted, 1)
	assert.Equal(t, leaderTokens, emitted[0].Tokens,
		"combined emission must carry the leader's tokens, not the continuation's")
}

// TestCombiningAggregator_MultiLineTagAttached anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee MultiLineTagging — A combined emission of two
//	                                   or more lines receives the
//	                                   multi-line source tag with
//	                                   value "auto_multiline" —
//	                                   gated by tag_multi_line_logs.
//
// With tag_multi_line_logs = true and a 2-line bucket, the
// flushed combined message has the multi-line tag and the
// is_multi_line flag set.
func TestCombiningAggregator_MultiLineTagAttached(t *testing.T) {
	ag := NewCombiningAggregator(100, false, true, status.NewInfoRegistry())

	require.Empty(t, processMsg(ag, newMessage("leader"), startGroup))
	require.Empty(t, processMsg(ag, newMessage("cont"), aggregate))

	msgs := flushMsgs(ag)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsMultiLine, "is_multi_line must be set on a 2+ line combined emission")
	assert.Contains(t, msgs[0].ParsingExtra.Tags, message.MultiLineSourceTag("auto_multiline"),
		"multi-line source tag with 'auto_multiline' value must be attached when tag_multi_line_logs is true")
}

// TestCombiningAggregator_MultiLineTagNotAttachedSingleLine anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee MultiLineTagging (negative case) — A combined
//	                                                   emission of
//	                                                   exactly one
//	                                                   line does
//	                                                   NOT receive
//	                                                   the
//	                                                   multi-line
//	                                                   tag.
//
// A bucket with only one line (a lonely start_group, flushed
// via external flush) does not receive the multi-line tag even
// when tag_multi_line_logs is enabled. The bucket_lines_count > 1
// guard holds.
func TestCombiningAggregator_MultiLineTagNotAttachedSingleLine(t *testing.T) {
	ag := NewCombiningAggregator(100, false, true, status.NewInfoRegistry())

	require.Empty(t, processMsg(ag, newMessage("solo"), startGroup))

	msgs := flushMsgs(ag)
	require.Len(t, msgs, 1)
	assert.False(t, msgs[0].ParsingExtra.IsMultiLine, "is_multi_line must be false for single-line combined emission")
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, message.MultiLineSourceTag("auto_multiline"),
		"multi-line tag must NOT be attached to a single-line emission even with tag_multi_line_logs enabled")
}

// TestCombiningAggregator_MultiLineTagNotAttachedConfigDisabled anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee MultiLineTagging (config gating) — The tag is
//	                                                   gated by
//	                                                   tag_multi_line_logs.
//	                                                   is_multi_line
//	                                                   flag is set
//	                                                   regardless.
//
// With tag_multi_line_logs = false but a 2-line combined emission,
// the tag is absent — but is_multi_line is still set on the
// emitted message.
func TestCombiningAggregator_MultiLineTagNotAttachedConfigDisabled(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	require.Empty(t, processMsg(ag, newMessage("leader"), startGroup))
	require.Empty(t, processMsg(ag, newMessage("cont"), aggregate))

	msgs := flushMsgs(ag)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsMultiLine, "is_multi_line is set independently of the tag config")
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, message.MultiLineSourceTag("auto_multiline"),
		"multi-line tag must NOT be attached when tag_multi_line_logs is disabled")
}

// TestCombiningAggregator_IsEmptyConsistency anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee IsEmptyConsistency — is_empty reflects
//	                                     bucket_empty exactly.
//
// Walks state transitions and asserts IsEmpty's value at each step:
// true at construction, true after no_aggregate (no buffering),
// false during a buffered start_group, true after flush.
func TestCombiningAggregator_IsEmptyConsistency(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	assert.True(t, ag.IsEmpty(), "construction")

	processMsg(ag, newMessage("first"), noAggregate)
	assert.True(t, ag.IsEmpty(), "after no_aggregate (never buffered)")

	require.Empty(t, processMsg(ag, newMessage("leader"), startGroup))
	assert.False(t, ag.IsEmpty(), "while start_group is buffered")

	processMsg(ag, newMessage("cont"), aggregate)
	assert.False(t, ag.IsEmpty(), "while aggregate is added to bucket")

	flushMsgs(ag)
	assert.True(t, ag.IsEmpty(), "after explicit flush")
}

// TestCombiningAggregator_LabelDriven anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee LabelDriven — output is determined by (label,
//	                              state) — contradicting
//	                              LabelIgnored from
//	                              PassThroughAggregator and
//	                              RegexAggregator.
//
// Identical content delivered under different label sequences
// produces observably different output sequences. Anything but a
// label-observing aggregator would emit identical outputs.
//
// Comparison: ("A" start_group, "B" aggregate) → 0 emissions
// then 0 emissions, flush produces 1 combined emission "A\\nB".
// Versus ("A" no_aggregate, "B" no_aggregate) → 2 separate
// emissions, flush produces 0.
func TestCombiningAggregator_LabelDriven(t *testing.T) {
	t.Run("startGroup then aggregate combines", func(t *testing.T) {
		ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())
		assert.Empty(t, processMsg(ag, newMessage("A"), startGroup))
		assert.Empty(t, processMsg(ag, newMessage("B"), aggregate))
		msgs := flushMsgs(ag)
		require.Len(t, msgs, 1)
		assert.Equal(t, "A\\nB", string(msgs[0].GetContent()))
	})

	t.Run("no_aggregate then no_aggregate stays separate", func(t *testing.T) {
		ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())
		first := processMsg(ag, newMessage("A"), noAggregate)
		require.Len(t, first, 1)
		assert.Equal(t, "A", string(first[0].GetContent()))
		second := processMsg(ag, newMessage("B"), noAggregate)
		require.Len(t, second, 1)
		assert.Equal(t, "B", string(second[0].GetContent()))
		// Nothing left to flush.
		assert.Empty(t, flushMsgs(ag))
	})
}

// TestCombiningAggregator_AutoMultilineReasonViaCarry anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee TruncationTagging — "auto_multiline" reason
//	                                    fires on combined flushes
//	                                    (lines > 1) that have ANY
//	                                    truncation applied,
//	                                    including via the
//	                                    prepended carry from a
//	                                    prior single-line oversize
//	                                    emission.
//
// Sequence:
//
//  1. Oversize start_group flushes immediately with append marker
//     and sets should_truncate = true.
//  2. Smaller start_group buffers (no flush).
//  3. Aggregate that fits is added to the bucket (now 2 lines).
//  4. External flush emits the combined message with the
//     PREPEND marker (inherited carry) and the "auto_multiline"
//     truncation-reason tag — even though the combined content
//     itself is well under line_limit.
//
// This exercises the dispatch-reachable "auto_multiline" reason
// path (carry-driven) — distinct from the structurally-present
// but dispatch-unreachable append-on-combined-overflow path
// noted in the spec.
func TestCombiningAggregator_AutoMultilineReasonViaCarry(t *testing.T) {
	ag := NewCombiningAggregator(10, true, false, status.NewInfoRegistry())

	// Phase 1: oversize start_group sets the carry.
	first := processMsg(ag, newMessage("1234567890"), startGroup)
	require.Len(t, first, 1)
	require.True(t, first[0].ParsingExtra.IsTruncated)
	require.Equal(t, []string{message.TruncatedReasonTag("single_line")}, first[0].ParsingExtra.Tags,
		"phase-1 emission must be tagged single_line (lineCount=1)")

	// Phase 2: build a 2-line bucket with content well under line_limit.
	require.Empty(t, processMsg(ag, newMessage("ab"), startGroup))
	require.Empty(t, processMsg(ag, newMessage("cd"), aggregate))

	// Phase 3: external flush emits the combined message — carry
	// drives the prepend marker, lineCount=2 selects auto_multiline.
	flushed := flushMsgs(ag)
	require.Len(t, flushed, 1)
	assert.True(t, flushed[0].ParsingExtra.IsTruncated, "combined flush following carry must have is_truncated set")
	assert.True(t, flushed[0].ParsingExtra.IsMultiLine, "combined flush with 2+ lines is multi-line")
	assert.Contains(t, flushed[0].ParsingExtra.Tags, message.TruncatedReasonTag("auto_multiline"),
		"combined flush carrying truncation must use auto_multiline reason (not single_line)")
	assert.NotContains(t, flushed[0].ParsingExtra.Tags, message.TruncatedReasonTag("single_line"),
		"combined flush must NOT receive the single_line reason — auto_multiline overrides")

	// Sanity: the combined content has the prepend marker (carry)
	// but NOT an appended marker (combined content fits in
	// line_limit, per OverflowExplosion).
	emitted := string(flushed[0].GetContent())
	assert.Equal(t, string(message.TruncatedFlag)+"ab\\ncd", emitted,
		"combined emission has prepend marker (from carry) but not append (would_overflow_bucket prevented it)")
}

// TestCombiningAggregator_FlushIdempotentOnEmpty anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee FlushDrainsBuffer — A flush call on an empty
//	                                    bucket returns an empty
//	                                    sequence and changes no
//	                                    observable state.
//
// Second consecutive flush returns empty; is_empty remains true.
// flush after no_aggregate (which itself flushes) returns empty.
func TestCombiningAggregator_FlushIdempotentOnEmpty(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	// Empty at construction.
	assert.Empty(t, flushMsgs(ag))
	assert.True(t, ag.IsEmpty())

	// Buffer + flush.
	require.Empty(t, processMsg(ag, newMessage("pending"), startGroup))
	require.Len(t, flushMsgs(ag), 1)
	assert.True(t, ag.IsEmpty())

	// Second flush.
	assert.Empty(t, flushMsgs(ag))
	assert.True(t, ag.IsEmpty())

	// Flush after no_aggregate (no_aggregate already drained the bucket).
	processMsg(ag, newMessage("standalone"), noAggregate)
	assert.Empty(t, flushMsgs(ag))
	assert.True(t, ag.IsEmpty())
}
