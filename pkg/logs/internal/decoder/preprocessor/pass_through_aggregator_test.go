// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Anchoring unit tests for the PassThroughAggregation surface
// declared in pass_through_aggregator.allium. Each test names the
// spec construct (@guarantee or @guidance step) it anchors so that
// drift in either direction is easy to spot during review.
//
// Property tests for the same surface live in
// pass_through_aggregator_proptest_test.go. Helpers (newMessage,
// processMsg, flushMsgs, assertMessageContent) are shared with
// aggregator_test.go.

// TestPassThroughAggregator_NoGrouping anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee NoGrouping — Every call to process emits
//	                             exactly one AggregatedMessageWithTokens;
//	                             PassThroughAggregator does not
//	                             combine consecutive lines.
//
// Multiple consecutive process calls each emit their own message;
// no buffering occurs across calls.
func TestPassThroughAggregator_NoGrouping(t *testing.T) {
	ag := NewPassThroughAggregator(1000)

	first := processMsg(ag, newMessage("line one"), aggregate)
	require.Len(t, first, 1, "first call must emit exactly one message")

	second := processMsg(ag, newMessage("line two"), aggregate)
	require.Len(t, second, 1, "second call must emit exactly one message")

	third := processMsg(ag, newMessage("line three"), aggregate)
	require.Len(t, third, 1, "third call must emit exactly one message")
}

// TestPassThroughAggregator_LabelIgnored anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee LabelIgnored — The label argument to process is
//	                               consumed but has no observable
//	                               effect on the output sequence or
//	                               on the should_truncate state.
//
// The same input under three different labels produces the same
// emitted content. This guards against accidental label-driven
// branching being introduced in the aggregator.
func TestPassThroughAggregator_LabelIgnored(t *testing.T) {
	for _, label := range []Label{startGroup, noAggregate, aggregate} {
		ag := NewPassThroughAggregator(1000)
		msgs := processMsg(ag, newMessage("identical content"), label)
		require.Len(t, msgs, 1)
		assert.Equal(t, "identical content", string(msgs[0].GetContent()),
			"output content must be identical regardless of label (got under label %v)", label)
		assert.False(t, msgs[0].ParsingExtra.IsTruncated,
			"is_truncated must be identical regardless of label")
	}
}

// TestPassThroughAggregator_TokensForwarded anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee TokensForwarded — The tokens argument is
//	                                  forwarded unchanged into
//	                                  AggregatedMessageWithTokens.tokens
//	                                  of the emitted message.
//
// Both nil and non-empty token slices are forwarded byte-for-byte
// onto the emitted AggregatedMessageWithTokens.
func TestPassThroughAggregator_TokensForwarded(t *testing.T) {
	cases := []struct {
		name   string
		tokens []Token
	}{
		{"nil tokens", nil},
		{"non-empty tokens", []Token{D4, Space, C5, D2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ag := NewPassThroughAggregator(1000)
			emitted := ag.Process(newMessage("content"), aggregate, tc.tokens)
			require.Len(t, emitted, 1)
			assert.Equal(t, tc.tokens, emitted[0].Tokens,
				"tokens must be forwarded unchanged onto the emitted AggregatedMessageWithTokens")
		})
	}
}

// TestPassThroughAggregator_FlushReturnsEmpty anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee FlushDrainsBuffer — flush always returns an empty
//	                                    sequence (PassThrough has no
//	                                    buffer state to drain)
//
// Flush is empty at construction and after process calls — there
// is never anything to drain.
func TestPassThroughAggregator_FlushReturnsEmpty(t *testing.T) {
	ag := NewPassThroughAggregator(1000)

	assert.Empty(t, flushMsgs(ag), "flush must return empty at construction")

	processMsg(ag, newMessage("anything"), aggregate)
	assert.Empty(t, flushMsgs(ag), "flush must return empty after a process call")

	// Even after a truncation that updates should_truncate, flush stays empty.
	processMsg(ag, newMessage(strings.Repeat("x", 2000)), aggregate)
	assert.Empty(t, flushMsgs(ag), "flush must return empty even when should_truncate carry is set")
}

// TestPassThroughAggregator_IsEmptyAlwaysTrue anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee IsEmptyConsistency — is_empty always returns true
//	                                     (no buffered content state
//	                                     ever exists)
//
// Verifies the trivial form: is_empty is true at construction,
// after a normal process call, after a truncation, and after flush.
// The rolling should_truncate flag is not buffered content and
// therefore does not influence is_empty.
func TestPassThroughAggregator_IsEmptyAlwaysTrue(t *testing.T) {
	ag := NewPassThroughAggregator(1000)
	assert.True(t, ag.IsEmpty(), "is_empty must be true at construction")

	processMsg(ag, newMessage("anything"), aggregate)
	assert.True(t, ag.IsEmpty(), "is_empty must remain true after a process call")

	processMsg(ag, newMessage(strings.Repeat("x", 2000)), aggregate)
	assert.True(t, ag.IsEmpty(), "is_empty must remain true after should_truncate carry is set")

	flushMsgs(ag)
	assert.True(t, ag.IsEmpty(), "is_empty must remain true after flush")
}

// TestPassThroughAggregator_TrimsWhitespace anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guidance step 3 — Trim leading and trailing whitespace from
//	                       the input content. This is unconditional;
//	                       it applies whether or not the line was
//	                       truncated.
//
// Leading and trailing whitespace is removed from the emitted
// content. Interior whitespace is preserved.
func TestPassThroughAggregator_TrimsWhitespace(t *testing.T) {
	ag := NewPassThroughAggregator(1000)
	msgs := processMsg(ag, newMessage("  \t  hello  world  \r\n"), aggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "hello  world", string(msgs[0].GetContent()))
}

// TestPassThroughAggregator_ByteConservation_NoTruncation anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guarantee ByteConservation — emitted bytes derive only
//	                                   from the input bytes plus
//	                                   well-known truncation markers
//
// When neither truncation condition fires, the emitted content
// is bytewise equal to the trimmed input bytes — no truncation
// marker is added.
func TestPassThroughAggregator_ByteConservation_NoTruncation(t *testing.T) {
	ag := NewPassThroughAggregator(1000)
	in := "  short content with internal spaces  "
	msgs := processMsg(ag, newMessage(in), aggregate)
	require.Len(t, msgs, 1)
	emitted := string(msgs[0].GetContent())
	assert.Equal(t, strings.TrimSpace(in), emitted)
	assert.False(t, strings.Contains(emitted, string(message.TruncatedFlag)),
		"no truncation marker must appear when truncation does not fire")
}

// TestPassThroughAggregator_AppendsMarkerOnOversize anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guidance step 5 — If should_truncate is true (i.e., this
//	                       line itself triggers truncation), append
//	                       the truncation marker to the content.
//
// When the input content exceeds line_limit, the emitted content
// has the truncation marker appended and the emitted message is
// flagged truncated.
func TestPassThroughAggregator_AppendsMarkerOnOversize(t *testing.T) {
	ag := NewPassThroughAggregator(10)
	msgs := processMsg(ag, newMessage("12345678901234567890"), aggregate)
	require.Len(t, msgs, 1)
	emitted := string(msgs[0].GetContent())
	assert.Equal(t, "12345678901234567890"+string(message.TruncatedFlag), emitted)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
}

// TestPassThroughAggregator_AppendsMarkerOnUpstreamTruncated anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guidance step 2 — should_truncate becomes true if … the
//	                       input msg.is_truncated is already true
//	    @guidance step 5 — append the truncation marker
//
// Upstream-flagged truncation (input msg.IsTruncated = true) is
// itself a truncation trigger, even if the content fits within
// line_limit.
func TestPassThroughAggregator_AppendsMarkerOnUpstreamTruncated(t *testing.T) {
	ag := NewPassThroughAggregator(1000)
	in := newMessage("short content")
	in.ParsingExtra.IsTruncated = true
	msgs := processMsg(ag, in, aggregate)
	require.Len(t, msgs, 1)
	emitted := string(msgs[0].GetContent())
	assert.True(t, strings.HasSuffix(emitted, string(message.TruncatedFlag)),
		"upstream-truncated input must produce a marker-suffixed emission; got %q", emitted)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
}

// TestPassThroughAggregator_PrependsMarkerAfterPriorTruncation anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guidance step 1+4 — Capture the current should_truncate
//	                          value as `prepend_marker` … If
//	                          prepend_marker is true, prepend the
//	                          truncation marker to the trimmed
//	                          content.
//
// Two-call test: the first call's oversize content sets the
// should_truncate carry; the second call's emitted content has the
// truncation marker prepended. This is the "continuation" half of
// the marker pair.
func TestPassThroughAggregator_PrependsMarkerAfterPriorTruncation(t *testing.T) {
	ag := NewPassThroughAggregator(10)

	// First call: oversize content. Sets should_truncate.
	first := processMsg(ag, newMessage("12345678901234567890"), aggregate)
	require.Len(t, first, 1)

	// Second call: short content (would not truncate on its own).
	// Emitted content must have the truncation marker prepended.
	second := processMsg(ag, newMessage("short"), aggregate)
	require.Len(t, second, 1)
	emitted := string(second[0].GetContent())
	assert.Equal(t, string(message.TruncatedFlag)+"short", emitted)
	assert.True(t, second[0].ParsingExtra.IsTruncated,
		"emitted message must be flagged truncated when the marker is prepended")
}

// TestPassThroughAggregator_PrependAndAppendOnConsecutiveOversize anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guidance step 4 + step 5 (in combination)
//
// Two consecutive oversized lines: the second line gets both
// prepend (from the first line's truncation carry) AND append (from
// its own oversize state).
func TestPassThroughAggregator_PrependAndAppendOnConsecutiveOversize(t *testing.T) {
	ag := NewPassThroughAggregator(10)

	first := processMsg(ag, newMessage("aaaaaaaaaaaaaaaa"), aggregate) // 16 chars > 10
	require.Len(t, first, 1)
	assert.Equal(t, "aaaaaaaaaaaaaaaa"+string(message.TruncatedFlag), string(first[0].GetContent()))

	second := processMsg(ag, newMessage("bbbbbbbbbbbbbbbb"), aggregate) // 16 chars > 10
	require.Len(t, second, 1)
	expected := string(message.TruncatedFlag) + "bbbbbbbbbbbbbbbb" + string(message.TruncatedFlag)
	assert.Equal(t, expected, string(second[0].GetContent()))
	assert.True(t, second[0].ParsingExtra.IsTruncated)
}

// TestPassThroughAggregator_TruncationCarryClearedAfterNormalLine anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guidance step 2 — Otherwise should_truncate becomes false
//
// After an oversized line sets the carry, a subsequent short line
// gets the prepended marker AND clears the carry. The line AFTER
// that one is back to normal — no markers added.
func TestPassThroughAggregator_TruncationCarryClearedAfterNormalLine(t *testing.T) {
	ag := NewPassThroughAggregator(10)

	processMsg(ag, newMessage("12345678901234567890"), aggregate) // oversize: sets carry
	processMsg(ag, newMessage("short"), aggregate)                // gets prepended marker; clears carry

	third := processMsg(ag, newMessage("clean"), aggregate)
	require.Len(t, third, 1)
	emitted := string(third[0].GetContent())
	assert.Equal(t, "clean", emitted, "third line must have no truncation markers — carry should have been cleared")
	assert.False(t, third[0].ParsingExtra.IsTruncated)
}

// TestPassThroughAggregator_TruncationTagsWhenEnabled anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guidance step 6 — when the runtime's truncated-log
//	                       tagging is enabled — attach a
//	                       truncation-reason tag identifying this
//	                       aggregator as the source
//
// With the logs_config.tag_truncated_logs flag enabled, a truncated
// emission has the "truncated:single_line" tag appended.
func TestPassThroughAggregator_TruncationTagsWhenEnabled(t *testing.T) {
	mockConfig := configmock.New(t)
	prev := mockConfig.GetBool("logs_config.tag_truncated_logs")
	mockConfig.Set("logs_config.tag_truncated_logs", true, pkgconfigmodel.SourceAgentRuntime)
	defer mockConfig.Set("logs_config.tag_truncated_logs", prev, pkgconfigmodel.SourceAgentRuntime)

	ag := NewPassThroughAggregator(10)
	msgs := processMsg(ag, newMessage("12345678901234567890"), aggregate)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].ParsingExtra.Tags, message.TruncatedReasonTag("single_line"),
		"truncation-reason tag must be attached when the runtime flag is enabled")
}

// TestPassThroughAggregator_TruncationDoesNotTagWhenDisabled anchors:
//
//	surface PassThroughAggregation (pass_through_aggregator.allium)
//	    @guidance step 6 — when the runtime's truncated-log
//	                       tagging is enabled (and only then)
//
// With the flag disabled, a truncated emission has no
// truncation-reason tag appended — even though IsTruncated is still
// set and the marker is still in the content.
func TestPassThroughAggregator_TruncationDoesNotTagWhenDisabled(t *testing.T) {
	mockConfig := configmock.New(t)
	prev := mockConfig.GetBool("logs_config.tag_truncated_logs")
	mockConfig.Set("logs_config.tag_truncated_logs", false, pkgconfigmodel.SourceAgentRuntime)
	defer mockConfig.Set("logs_config.tag_truncated_logs", prev, pkgconfigmodel.SourceAgentRuntime)

	ag := NewPassThroughAggregator(10)
	msgs := processMsg(ag, newMessage("12345678901234567890"), aggregate)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated,
		"is_truncated must still be set even when tagging is disabled")
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, message.TruncatedReasonTag("single_line"),
		"truncation-reason tag must NOT be attached when the runtime flag is disabled")
}
