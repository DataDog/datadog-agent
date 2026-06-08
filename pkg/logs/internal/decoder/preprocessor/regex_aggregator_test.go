// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// Anchoring unit tests for the RegexAggregation surface declared
// in regex_aggregator.allium. Each test names the spec construct
// (@guarantee or @guidance step) it anchors so that drift in either
// direction is easy to spot during review.
//
// Property tests for the same surface live in
// regex_aggregator_proptest_test.go. The pre-match-single-pass and
// pattern-boundary scenarios are exercised by the three
// TestRegexAggregator* tests in aggregator_test.go, which carry
// their own anchoring docstrings; the tests in THIS file cover the
// surface guarantees those existing tests don't yet anchor.

func newRegexAggregator(t *testing.T, pattern string, lineLimit int) *RegexAggregator {
	t.Helper()
	re := regexp.MustCompile(pattern)
	return NewRegexAggregator(re, lineLimit, false, status.NewInfoRegistry(), "multi_line")
}

// TestRegexAggregator_LabelIgnored anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee LabelIgnored — The label argument to process is
//	                               consumed but has no observable
//	                               effect on the output sequence,
//	                               on the buffered state, or on the
//	                               should_truncate / is_buffer_
//	                               truncated flags.
//
// Two aggregators identically configured, fed identical line
// sequences but with different labels, produce identical output
// content and is_truncated flags.
func TestRegexAggregator_LabelIgnored(t *testing.T) {
	lines := []string{"START group 1", "continuation 1", "START group 2", "tail"}

	collect := func(label Label) []string {
		ag := newRegexAggregator(t, `^START`, 1000)
		var out []string
		for _, line := range lines {
			for _, m := range processMsg(ag, newMessage(line), label) {
				out = append(out, string(m.GetContent()))
			}
		}
		for _, m := range flushMsgs(ag) {
			out = append(out, string(m.GetContent()))
		}
		return out
	}

	assert.Equal(t, collect(startGroup), collect(noAggregate))
	assert.Equal(t, collect(startGroup), collect(aggregate))
}

// TestRegexAggregator_TokensFromAggregateLeader anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee TokensFromAggregateLeader — The tokens emitted
//	                                            on each
//	                                            AggregatedMessageWithTokens
//	                                            are the tokens that
//	                                            were passed to
//	                                            process alongside
//	                                            the FIRST line of
//	                                            that aggregate.
//
// Distinct token sequences on each input line; on emission, the
// aggregate's tokens equal the leader's tokens — not the last
// line's tokens and not a concatenation.
func TestRegexAggregator_TokensFromAggregateLeader(t *testing.T) {
	re := regexp.MustCompile(`^START`)
	ag := NewRegexAggregator(re, 1000, false, status.NewInfoRegistry(), "multi_line")

	leaderTokens := []Token{D4, Space, C5}
	continuationTokens := []Token{C5, Space, D1}
	finalTokens := []Token{D2}

	// First line matches — becomes the leader. Buffered, no emission yet.
	emitted := ag.Process(newMessage("START group"), aggregate, leaderTokens)
	require.Empty(t, emitted)

	// Continuation line, different tokens. Still buffered.
	emitted = ag.Process(newMessage("continuation"), aggregate, continuationTokens)
	require.Empty(t, emitted)

	// Next match — flushes the prior aggregate. The emitted tokens
	// must be the leader's, not the continuation's.
	emitted = ag.Process(newMessage("START next group"), aggregate, finalTokens)
	require.Len(t, emitted, 1)
	assert.Equal(t, leaderTokens, emitted[0].Tokens,
		"emitted tokens must be the aggregate leader's tokens, not the continuation's or the trigger's")
}

// TestRegexAggregator_FlushDrainsBuffer anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee FlushDrainsBuffer — After flush returns,
//	                                    buffer_empty is true …
//	                                    A flush call on an
//	                                    already-empty buffer
//	                                    returns an empty sequence.
//
// Three cases: flush at construction (empty), flush after a
// pattern-match has buffered content (non-empty drain), flush
// immediately after another flush (idempotent on empty).
func TestRegexAggregator_FlushDrainsBuffer(t *testing.T) {
	ag := newRegexAggregator(t, `^START`, 1000)

	// Empty at construction — flush returns nothing.
	assert.Empty(t, flushMsgs(ag), "flush on empty must return nothing")
	assert.True(t, ag.IsEmpty(), "is_empty must remain true after flush on empty")

	// Buffer some content.
	require.Empty(t, processMsg(ag, newMessage("START group"), aggregate))
	require.Empty(t, processMsg(ag, newMessage("continuation"), aggregate))
	assert.False(t, ag.IsEmpty(), "is_empty must be false while content is buffered")

	// Flush drains.
	msgs := flushMsgs(ag)
	require.Len(t, msgs, 1)
	assert.Equal(t, "START group\\ncontinuation", string(msgs[0].GetContent()))
	assert.True(t, ag.IsEmpty(), "is_empty must be true after flush drains the buffer")

	// Second flush is idempotent.
	assert.Empty(t, flushMsgs(ag), "second flush on now-empty buffer must return nothing")
	assert.True(t, ag.IsEmpty())
}

// TestRegexAggregator_IsEmptyConsistency anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee IsEmptyConsistency — is_empty reflects
//	                                     buffer_empty exactly.
//
// is_empty tracks the buffer through every state transition: true
// at construction, false while pre-match line is buffered, true
// after the buffered line is emitted by the next call's opening
// flush, and so on.
func TestRegexAggregator_IsEmptyConsistency(t *testing.T) {
	ag := newRegexAggregator(t, `^START`, 1000)
	assert.True(t, ag.IsEmpty(), "construction: is_empty must be true")

	// Pre-match line is buffered.
	processMsg(ag, newMessage("first"), aggregate)
	assert.False(t, ag.IsEmpty(), "after pre-match line is buffered: is_empty must be false")

	// Next call's opening flush drains the first line, then buffers the second.
	processMsg(ag, newMessage("second"), aggregate)
	assert.False(t, ag.IsEmpty(), "second pre-match line is now buffered: is_empty must be false")

	// Match transitions out of pre-match: first the buffered "second" is emitted,
	// then "START" becomes the leader of a new aggregate. Buffer is non-empty.
	processMsg(ag, newMessage("START match"), aggregate)
	assert.False(t, ag.IsEmpty(), "after START becomes leader: is_empty must be false")

	flushMsgs(ag)
	assert.True(t, ag.IsEmpty(), "after explicit flush: is_empty must be true")
}

// TestRegexAggregator_ByteConservation_SeparatorBetweenLines anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee ByteConservation (refined) — joined by a fixed
//	                                             escaped-line-feed
//	                                             separator between
//	                                             adjacent lines
//
// A multi-line aggregate's emitted content consists of input
// contents joined by exactly one EscapedLineFeed per gap. No
// extra bytes appear between adjacent contributors.
func TestRegexAggregator_ByteConservation_SeparatorBetweenLines(t *testing.T) {
	ag := newRegexAggregator(t, `^START`, 1000)

	processMsg(ag, newMessage("START leader"), aggregate)
	processMsg(ag, newMessage("cont 1"), aggregate)
	processMsg(ag, newMessage("cont 2"), aggregate)

	msgs := flushMsgs(ag)
	require.Len(t, msgs, 1)
	separator := string(message.EscapedLineFeed)
	expected := strings.Join([]string{"START leader", "cont 1", "cont 2"}, separator)
	assert.Equal(t, expected, string(msgs[0].GetContent()))
}

// TestRegexAggregator_MidAggregateTruncation anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee MidAggregateTruncation — When the buffered
//	                                         aggregate reaches or
//	                                         exceeds line_limit
//	                                         bytes, the buffer is
//	                                         flushed mid-stream …
//	                                         and should_truncate
//	                                         is set as a
//	                                         continuation-marker
//	                                         carry … consumed by
//	                                         the NEXT post-match
//	                                         non-matching process
//	                                         call.
//
// Three phases:
//
//  1. Buffered match line + overflowing continuation: the overflow
//     flushes within the same call with the marker appended.
//  2. Subsequent non-matching short line: the carry is consumed —
//     marker prepended, buffered (does not overflow on its own).
//  3. Flush drains the carry-marked buffer; emission has the
//     prefix marker and is_truncated set.
//
// The "non-matching" qualifier is load-bearing — a
// pattern-matching continuation would reset the carry via the
// opening-flush's sendBuffer defer.
func TestRegexAggregator_MidAggregateTruncation(t *testing.T) {
	// line_limit large enough that prepending the marker (15 bytes)
	// plus a short content does not itself overflow — so phase (2)
	// genuinely tests "buffered with prepend" rather than "overflow
	// while prepending."
	ag := newRegexAggregator(t, `^START`, 50)

	// Phase 1a: pattern match buffers a leader. No emission.
	require.Empty(t, processMsg(ag, newMessage("START"), aggregate))

	// Phase 1b: overflowing continuation emits within the same call.
	msgs := processMsg(ag, newMessage("a very very long continuation that exceeds the buffer"), aggregate)
	require.Len(t, msgs, 1, "overflow must flush within the same call")
	assert.True(t, msgs[0].ParsingExtra.IsTruncated, "overflow emission must have is_truncated set")
	assert.True(t, strings.HasSuffix(string(msgs[0].GetContent()), string(message.TruncatedFlag)),
		"overflow emission must have the truncation marker appended; got %q", msgs[0].GetContent())

	// Phase 2: subsequent non-matching short line consumes the carry.
	// Buffered, not emitted yet.
	require.Empty(t, processMsg(ag, newMessage("short"), aggregate),
		"short non-matching continuation must be buffered, not emitted in-call")

	// Phase 3: flush drains. Emission has the prefix marker and is_truncated.
	flushed := flushMsgs(ag)
	require.Len(t, flushed, 1, "flush must drain the carry-marked buffer")
	assert.True(t, flushed[0].ParsingExtra.IsTruncated,
		"flushed emission carrying the continuation marker must have is_truncated set")
	assert.True(t, strings.HasPrefix(string(flushed[0].GetContent()), string(message.TruncatedFlag)),
		"emission following an overflow (with a non-matching continuation) must have the truncation marker prepended; got %q",
		flushed[0].GetContent())
}

// TestRegexAggregator_TruncationTagging anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee TruncationTagging — When an emission's
//	                                    is_buffer_truncated is true,
//	                                    the emitted message receives
//	                                    a truncation-reason tag
//	                                    identifying multiline-regex
//	                                    as the source — but only
//	                                    when the runtime's
//	                                    tag-truncated-logs
//	                                    configuration is enabled.
//
// Two sub-tests: enabled (tag present) and disabled (tag absent).
// The is_truncated flag itself is independent of the config flag.
// The tag reason value is "multiline_regex" (distinct from
// PassThrough's "single_line").
func TestRegexAggregator_TruncationTagging(t *testing.T) {
	expectedTag := message.TruncatedReasonTag("multiline_regex")

	run := func(t *testing.T, enabled bool) {
		mockConfig := configmock.New(t)
		prev := mockConfig.GetBool("logs_config.tag_truncated_logs")
		mockConfig.Set("logs_config.tag_truncated_logs", enabled, pkgconfigmodel.SourceAgentRuntime)
		defer mockConfig.Set("logs_config.tag_truncated_logs", prev, pkgconfigmodel.SourceAgentRuntime)

		ag := newRegexAggregator(t, `^START`, 10)
		processMsg(ag, newMessage("START"), aggregate)
		msgs := processMsg(ag, newMessage("a very long continuation line"), aggregate)
		require.Len(t, msgs, 1)
		assert.True(t, msgs[0].ParsingExtra.IsTruncated, "is_truncated is independent of the tag config")
		if enabled {
			assert.Contains(t, msgs[0].ParsingExtra.Tags, expectedTag, "tag must be present when config enabled")
		} else {
			assert.NotContains(t, msgs[0].ParsingExtra.Tags, expectedTag, "tag must be absent when config disabled")
		}
	}

	t.Run("enabled", func(t *testing.T) { run(t, true) })
	t.Run("disabled", func(t *testing.T) { run(t, false) })
}

// TestRegexAggregator_MultiLineTagging anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee MultiLineTagging — When an emission combined
//	                                   two or more input lines …
//	                                   the emitted message receives
//	                                   the multi-line source tag
//	                                   … but only when the runtime's
//	                                   tag-multi-line-logs
//	                                   configuration is enabled.
//
// Three cases: (a) 2+ lines + config enabled → tag present;
// (b) 2+ lines + config disabled → tag absent; (c) single-line
// aggregate + config enabled → tag absent (the lines_combined > 1
// guard is enforced regardless of config).
func TestRegexAggregator_MultiLineTagging(t *testing.T) {
	expectedTag := message.MultiLineSourceTag("multi_line")

	t.Run("multi-line and enabled", func(t *testing.T) {
		mockConfig := configmock.New(t)
		prev := mockConfig.GetBool("logs_config.tag_multi_line_logs")
		mockConfig.Set("logs_config.tag_multi_line_logs", true, pkgconfigmodel.SourceAgentRuntime)
		defer mockConfig.Set("logs_config.tag_multi_line_logs", prev, pkgconfigmodel.SourceAgentRuntime)

		ag := newRegexAggregator(t, `^START`, 1000)
		processMsg(ag, newMessage("START leader"), aggregate)
		processMsg(ag, newMessage("continuation"), aggregate)
		msgs := flushMsgs(ag)
		require.Len(t, msgs, 1)
		assert.Contains(t, msgs[0].ParsingExtra.Tags, expectedTag)
	})

	t.Run("multi-line and disabled", func(t *testing.T) {
		mockConfig := configmock.New(t)
		prev := mockConfig.GetBool("logs_config.tag_multi_line_logs")
		mockConfig.Set("logs_config.tag_multi_line_logs", false, pkgconfigmodel.SourceAgentRuntime)
		defer mockConfig.Set("logs_config.tag_multi_line_logs", prev, pkgconfigmodel.SourceAgentRuntime)

		ag := newRegexAggregator(t, `^START`, 1000)
		processMsg(ag, newMessage("START leader"), aggregate)
		processMsg(ag, newMessage("continuation"), aggregate)
		msgs := flushMsgs(ag)
		require.Len(t, msgs, 1)
		assert.NotContains(t, msgs[0].ParsingExtra.Tags, expectedTag)
	})

	t.Run("single-line aggregate is not tagged", func(t *testing.T) {
		mockConfig := configmock.New(t)
		prev := mockConfig.GetBool("logs_config.tag_multi_line_logs")
		mockConfig.Set("logs_config.tag_multi_line_logs", true, pkgconfigmodel.SourceAgentRuntime)
		defer mockConfig.Set("logs_config.tag_multi_line_logs", prev, pkgconfigmodel.SourceAgentRuntime)

		ag := newRegexAggregator(t, `^START`, 1000)
		processMsg(ag, newMessage("START solo"), aggregate)
		msgs := flushMsgs(ag)
		require.Len(t, msgs, 1)
		assert.NotContains(t, msgs[0].ParsingExtra.Tags, expectedTag,
			"single-line aggregate must not receive the multi-line tag even when config is enabled")
	})
}

// TestRegexAggregator_PreMatchTruncationCarryDoesNotPropagate anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee PreMatchSinglePass (the should_truncate-carry
//	                                    clarification: during pre-
//	                                    match, the opening flush
//	                                    resets should_truncate;
//	                                    after pattern_matched_once
//	                                    is set, should_truncate
//	                                    propagates)
//
// A pre-match line that overflows line_limit sets should_truncate.
// The opening flush on the NEXT pre-match call resets it, so the
// next line does not receive a prepended continuation marker.
// Contrast with the post-match flow (MidAggregateTruncation test
// above), where the prepended marker DOES appear.
func TestRegexAggregator_PreMatchTruncationCarryDoesNotPropagate(t *testing.T) {
	ag := newRegexAggregator(t, `^NEVER_MATCHES$`, 10)

	// Pre-match line that overflows: emits within the same call.
	msgs := processMsg(ag, newMessage("a very long pre-match line"), aggregate)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated, "overflow emission must be truncated")

	// Next pre-match line is short. The opening flush should reset
	// should_truncate, so the buffered "next short" should NOT
	// receive the prepended marker.
	processMsg(ag, newMessage("short"), aggregate)
	msgs = flushMsgs(ag)
	require.Len(t, msgs, 1)
	emitted := string(msgs[0].GetContent())
	assert.False(t, strings.HasPrefix(emitted, string(message.TruncatedFlag)),
		"pre-match overflow's should_truncate must not propagate; got prefix on %q", emitted)
	assert.False(t, msgs[0].ParsingExtra.IsTruncated,
		"continuation line must not be marked truncated when carry was reset")
}
