// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

func newMessage(content string) *message.Message {
	m := message.NewMessage([]byte(content), nil, message.StatusInfo, 0)
	m.RawDataLen = len([]byte(content))
	return m
}

func newMessageWithTimestamp(content, timestamp string) *message.Message {
	m := newMessage(content)
	m.ParsingExtra.Timestamp = timestamp
	return m
}

func assertMessageContent(t *testing.T, m *message.Message, content string) {
	t.Helper()
	isMultiLine := len(strings.Split(content, "\\n")) > 1
	assert.Equal(t, content, string(m.GetContent()))
	assert.Equal(t, m.IsMultiLine, isMultiLine)
}

// processMsg calls Process with nil tokens and returns only the messages, for tests that
// don't need to inspect token propagation.
func processMsg(ag Aggregator, msg *message.Message, label Label) []*message.Message {
	completed := ag.Process(msg, label, nil)
	out := make([]*message.Message, len(completed))
	for i, c := range completed {
		out[i] = c.Msg
	}
	return out
}

// flushMsgs calls Flush and returns only the messages.
func flushMsgs(ag Aggregator) []*message.Message {
	completed := ag.Flush()
	out := make([]*message.Message, len(completed))
	for i, c := range completed {
		out[i] = c.Msg
	}
	return out
}

// NOTE: The Aggregator.Process return slice shares its backing array with the
// aggregator's internal buffer and is only valid until the next Process/Flush call.
// Tests must assert results before making the next call.

// TestNoAggregate anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee NoAggregateFlushes — A process call with the
//	                                     no_aggregate label flushes
//	                                     any buffered bucket then
//	                                     emits the current line.
//
// The simplest path: with an empty bucket, each no_aggregate call
// flushes nothing (bucket was empty) and emits the current line
// as single-line. Single-emission per call shape verified.
func TestNoAggregate(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	msgs := processMsg(ag, newMessage("1"), noAggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "1")

	msgs = processMsg(ag, newMessage("2"), noAggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "2")

	msgs = processMsg(ag, newMessage("3"), noAggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "3")
}

// TestNoAggregateEndsGroup anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee NoAggregateFlushes — produces 1 or 2 emissions
//	                                     per call: (bucket-flush
//	                                     if non-empty) followed
//	                                     by (current line).
//	    @guarantee StartGroupBoundary — start_group flushes the
//	                                     prior bucket and begins
//	                                     a new one.
//
// Verifies both boundary-flushing labels: start_group flushes the
// previously buffered start_group as combined (1 emission, since
// buffer holds 1 line), then no_aggregate flushes the next
// start_group AND emits its own line (2 emissions).
func TestNoAggregateEndsGroup(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	require.Empty(t, processMsg(ag, newMessage("1"), startGroup))

	msgs := processMsg(ag, newMessage("2"), startGroup) // flushes "1"
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "1")

	msgs = processMsg(ag, newMessage("3"), noAggregate) // flushes "2", then emits "3"
	require.Len(t, msgs, 2)
	assertMessageContent(t, msgs[0], "2")
	assertMessageContent(t, msgs[1], "3")
}

// TestAggregateGroups anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee StartGroupBoundary — start_group flushes prior
//	                                     bucket as combined.
//	    @guarantee ByteConservation (refined) — combined emission
//	                                             is concat with
//	                                             escaped-line-feed
//	                                             separator.
//
// Three lines aggregate into one bucket: startGroup + aggregate +
// aggregate. The next startGroup flushes them as one combined
// "1\\n2\\n3" message. Then a no_aggregate flushes "4" (single)
// and emits "5".
func TestAggregateGroups(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	// Accumulate a group
	require.Empty(t, processMsg(ag, newMessage("1"), startGroup))
	require.Empty(t, processMsg(ag, newMessage("2"), aggregate))
	require.Empty(t, processMsg(ag, newMessage("3"), aggregate))

	// New startGroup flushes the previous group
	msgs := processMsg(ag, newMessage("4"), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "1\\n2\\n3")

	// noAggregate flushes "4" then emits "5"
	msgs = processMsg(ag, newMessage("5"), noAggregate)
	require.Len(t, msgs, 2)
	assertMessageContent(t, msgs[0], "4")
	assertMessageContent(t, msgs[1], "5")
}

// TestAggregateDoesntStartGroup anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guidance step 3 — If label = aggregate AND bucket_empty:
//	                       add msg to the bucket, flush_bucket
//	                       immediately. Emits as single-line.
//
// An aggregate label on an empty bucket does NOT start a new group
// — it emits the line as a single-line message immediately, same
// shape as no_aggregate (modulo the carry-reset semantic).
func TestAggregateDoesntStartGroup(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	msgs := processMsg(ag, newMessage("1"), aggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "1")

	msgs = processMsg(ag, newMessage("2"), aggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "2")

	msgs = processMsg(ag, newMessage("3"), aggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "3")
}

// TestAggregateCarriesLastLineTimestamp asserts that when the auto-multiline
// aggregator emits a combined message, its ParsingExtra.Timestamp equals the
// LAST aggregated line's timestamp. The Docker socket tailer uses this field
// as the lastSince offset; if it held the first line's timestamp instead,
// any reader restart would cause Docker to replay lines 2..N as duplicates.
func TestAggregateCarriesLastLineTimestamp(t *testing.T) {
	const (
		ts1 = "2026-05-11T10:00:00.000000001Z"
		ts2 = "2026-05-11T10:00:00.000000002Z"
		ts3 = "2026-05-11T10:00:00.000000003Z"
		ts4 = "2026-05-11T10:00:00.000000004Z"
	)

	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	require.Empty(t, processMsg(ag, newMessageWithTimestamp("1", ts1), startGroup))
	require.Empty(t, processMsg(ag, newMessageWithTimestamp("2", ts2), aggregate))
	require.Empty(t, processMsg(ag, newMessageWithTimestamp("3", ts3), aggregate))

	// A new startGroup flushes the previous bucket [L1, L2, L3].
	msgs := processMsg(ag, newMessageWithTimestamp("4", ts4), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "1\\n2\\n3")
	assert.Equal(t, ts3, msgs[0].ParsingExtra.Timestamp,
		"aggregated message must carry the LAST line's parser timestamp so the tailer offset advances past every combined line")

	// The pending single-line "4" carries its own timestamp.
	msgs = flushMsgs(ag)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "4")
	assert.Equal(t, ts4, msgs[0].ParsingExtra.Timestamp)
}

// TestSingleLineKeepsOwnTimestamp confirms the fix is a no-op for single-line
// flushes (the carry-forward only runs when len(b.lines) > 1).
func TestSingleLineKeepsOwnTimestamp(t *testing.T) {
	const ts1 = "2026-05-11T10:00:00.000000001Z"

	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	msgs := processMsg(ag, newMessageWithTimestamp("solo", ts1), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, ts1, msgs[0].ParsingExtra.Timestamp)
}

// TestForceFlush anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee FlushDrainsBuffer — external Flush() emits the
//	                                    buffered bucket as combined.
//
// A bucket containing startGroup + 2 aggregates is drained by an
// external Flush() call into one combined emission. Tests that
// flush is a valid externally-triggered emission path (not just
// the per-label dispatch).
func TestForceFlush(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	require.Empty(t, processMsg(ag, newMessage("1"), startGroup))
	require.Empty(t, processMsg(ag, newMessage("2"), aggregate))
	require.Empty(t, processMsg(ag, newMessage("3"), aggregate))

	msgs := flushMsgs(ag)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "1\\n2\\n3")
}

// TestTagTruncatedLogs anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee TruncationTagging — truncated emissions get
//	                                    "single_line" reason tag
//	                                    when tag_truncated_logs.
//	    @guarantee OverflowExplosion — aggregate continuation that
//	                                    would overflow explodes
//	                                    the bucket.
//	    @guarantee NoAggregateResetsCarry — no_aggregate resets
//	                                         should_truncate to
//	                                         false before processing.
//
// Comprehensive truncation scenario test: oversized start_group is
// flushed and tagged single_line; following aggregate inherits the
// carry; an aggregate-on-empty path; then overflow explosion
// emits buffered lines individually; finally no_aggregate clears
// the carry — "00" emerges untruncated.
func TestTagTruncatedLogs(t *testing.T) {
	ag := NewCombiningAggregator(10, true, false, status.NewInfoRegistry())

	// "1234567890" (len=10) as startGroup: immediately flushed (size >= maxContentSize)
	msgs := processMsg(ag, newMessage("1234567890"), startGroup)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("single_line")}, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "1234567890...TRUNCATED...")

	// aggregate on empty bucket: add+flush immediately; carries TRUNCATED prefix
	msgs = processMsg(ag, newMessage("12345678901"), aggregate)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("single_line")}, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "...TRUNCATED...12345678901...TRUNCATED...")

	msgs = processMsg(ag, newMessage("12345"), aggregate)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("single_line")}, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "...TRUNCATED...12345")

	// "12\\n34" fits (6 < 10), but adding "567" would overflow (11 >= 10).
	// The aggregator should abandon multiline aggregation and emit standalone events.
	require.Empty(t, processMsg(ag, newMessage("12"), startGroup))
	require.Empty(t, processMsg(ag, newMessage("34"), aggregate))

	msgs = processMsg(ag, newMessage("567"), aggregate)
	require.Len(t, msgs, 3)
	assert.False(t, msgs[0].ParsingExtra.IsMultiLine)
	assert.False(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "12")

	assert.False(t, msgs[1].ParsingExtra.IsMultiLine)
	assert.False(t, msgs[1].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[1].ParsingExtra.Tags)
	assertMessageContent(t, msgs[1], "34")

	assert.False(t, msgs[2].ParsingExtra.IsMultiLine)
	assert.False(t, msgs[2].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[2].ParsingExtra.Tags)
	assertMessageContent(t, msgs[2], "567")

	// noAggregate resets truncation carry; "00" should not be truncated
	msgs = processMsg(ag, newMessage("00"), noAggregate)
	require.Len(t, msgs, 1)
	assert.False(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "00")
}

// TestSingleGroupOverflowStopsAggregation anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee OverflowExplosion — buffered lines are emitted
//	                                    INDIVIDUALLY (not combined+
//	                                    truncated) when aggregate
//	                                    continuation would overflow.
//
// The defining test for OverflowExplosion: line_limit=8,
// "123" + "456" would combine to "123\\n456" = 8 bytes ≥ 8.
// Verifies that BOTH lines come out intact and untagged — the
// combined message is NEVER produced.
func TestSingleGroupOverflowStopsAggregation(t *testing.T) {
	ag := NewCombiningAggregator(8, true, false, status.NewInfoRegistry())

	require.Empty(t, processMsg(ag, newMessage("123"), startGroup))

	// "123\\n456" would overflow (3+2+3=8 >= 8), so the lines stay separate.
	msgs := processMsg(ag, newMessage("456"), aggregate)
	require.Len(t, msgs, 2)
	assert.False(t, msgs[0].ParsingExtra.IsMultiLine)
	assert.False(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "123")

	assert.False(t, msgs[1].ParsingExtra.IsMultiLine)
	assert.False(t, msgs[1].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[1].ParsingExtra.Tags)
	assertMessageContent(t, msgs[1], "456")
}

// TestOverflowedGroupEmitsOriginalTokens anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee TokensFromAggregateLeader — exploded emissions
//	                                            carry their own
//	                                            line's tokens (not
//	                                            a leader's).
//
// In the explosion path, each emission's tokens are those passed
// in the call that buffered that line. The first emission has the
// start_group's tokens, the second has the aggregate continuation's
// tokens — they don't all share the leader's.
func TestOverflowedGroupEmitsOriginalTokens(t *testing.T) {
	ag := NewCombiningAggregator(8, false, false, status.NewInfoRegistry())

	firstTokens := []Token{1, 2}
	secondTokens := []Token{3, 4}

	require.Empty(t, ag.Process(newMessage("123"), startGroup, firstTokens))

	completed := ag.Process(newMessage("456"), aggregate, secondTokens)
	require.Len(t, completed, 2)
	assert.Equal(t, firstTokens, completed[0].Tokens)
	assert.Equal(t, secondTokens, completed[1].Tokens)
}

// TestSingleLineTruncatedLogIsTaggedSingleLine anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee TruncationTagging — single-line emissions get
//	                                    "single_line" reason tag.
//	    @guidance step 4c — start_group whose RawDataLen >=
//	                        line_limit triggers immediate
//	                        single-line flush.
//
// A startGroup with content exactly at line_limit triggers the
// immediate-flush path. Verifies it's tagged "single_line"
// (not "auto_multiline" — even though it went through a
// multi-line-capable aggregator).
func TestSingleLineTruncatedLogIsTaggedSingleLine(t *testing.T) {
	ag := NewCombiningAggregator(5, true, false, status.NewInfoRegistry())

	// Exactly maxContentSize — simulates truncation in the framer
	msgs := processMsg(ag, newMessage("12345"), startGroup)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("single_line")}, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "12345...TRUNCATED...")

	msgs = processMsg(ag, newMessage("456"), aggregate)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("single_line")}, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "...TRUNCATED...456")
}

// TestTagMultiLineLogs anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee OverflowExplosion — emission of overflowed
//	                                    group does NOT receive
//	                                    multi-line tag (each
//	                                    exploded line is single).
//
// Counter-test for MultiLineTagging: even with
// tag_multi_line_logs=true, the explosion path produces
// individually emitted lines, none of which are flagged
// is_multi_line or tagged "auto_multiline". The multi-line
// signal requires an actual combined emission.
func TestTagMultiLineLogs(t *testing.T) {
	ag := NewCombiningAggregator(12, false, true, status.NewInfoRegistry())

	require.Empty(t, processMsg(ag, newMessage("1234"), startGroup))
	require.Empty(t, processMsg(ag, newMessage("5678"), aggregate))

	// "1234\\n5678" fits (10 < 12), but adding "90" would overflow (14 >= 12).
	msgs := processMsg(ag, newMessage("90"), aggregate)
	require.Len(t, msgs, 3)
	assert.False(t, msgs[0].ParsingExtra.IsMultiLine)
	assert.False(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "1234")

	assert.False(t, msgs[1].ParsingExtra.IsMultiLine)
	assert.False(t, msgs[1].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[1].ParsingExtra.Tags)
	assertMessageContent(t, msgs[1], "5678")

	assert.False(t, msgs[2].ParsingExtra.IsMultiLine)
	assert.False(t, msgs[2].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[2].ParsingExtra.Tags)
	assertMessageContent(t, msgs[2], "90")

	msgs = processMsg(ag, newMessage("2"), noAggregate)
	require.Len(t, msgs, 1)
	assert.False(t, msgs[0].ParsingExtra.IsMultiLine)
	assert.False(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "2")
}

// TestSingleLineTooLongTruncation anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee NoAggregateResetsCarry — no_aggregate resets
//	                                         should_truncate
//	                                         before processing
//	                                         (the JSON-protection
//	                                         semantic).
//	    @guarantee TruncationTagging (rolling carry behaviour)
//
// Three phases of single-line truncation cases:
//  1. Aggregation overflow leaves no carry (exploded lines aren't
//     marked truncated by the explosion itself).
//  2. Consecutive oversized single-line emissions chain via the
//     should_truncate carry, producing prepended-and-appended
//     markers in the middle.
//  3. A no_aggregate clears the carry — even with carry set from
//     prior emission, the no_aggregate line emerges with no
//     prepended marker. This is the JSON-protection guarantee.
func TestSingleLineTooLongTruncation(t *testing.T) {
	ag := NewCombiningAggregator(5, false, true, status.NewInfoRegistry())

	// Phase 1: aggregation overflow should emit intact standalone lines and not start truncation carry.
	require.Empty(t, processMsg(ag, newMessage("12"), startGroup))
	msgs := processMsg(ag, newMessage("3"), aggregate)
	require.Len(t, msgs, 2)
	assertMessageContent(t, msgs[0], "12")
	assertMessageContent(t, msgs[1], "3")

	// bucket empty, add "123456" → immediately flushed (6 >= 5)
	msgs = processMsg(ag, newMessage("123456"), aggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "123456...TRUNCATED...")

	// bucket empty, shouldTruncate=true, add "123" → flushed with prefix
	msgs = processMsg(ag, newMessage("123"), aggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "...TRUNCATED...123")

	// Force flush: start empty group — nothing emitted
	require.Empty(t, processMsg(ag, newMessage(""), startGroup))

	// Phase 2: single-line logs each too large
	msgs = processMsg(ag, newMessage("123456"), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "123456...TRUNCATED...")

	msgs = processMsg(ag, newMessage("123456"), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "...TRUNCATED...123456...TRUNCATED...")

	msgs = processMsg(ag, newMessage("123456"), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "...TRUNCATED...123456...TRUNCATED...")

	// "123" fits (3 < 5): buffered
	require.Empty(t, processMsg(ag, newMessage("123"), startGroup))

	// Force flush: flushes "123" with prefix
	msgs = processMsg(ag, newMessage(""), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "...TRUNCATED...123")

	// Phase 3: noAggregate clears the TRUNCATED carry
	msgs = processMsg(ag, newMessage("123456"), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "123456...TRUNCATED...")

	// noAggregate: shouldTruncate is explicitly cleared → no prefix
	msgs = processMsg(ag, newMessage("123456"), noAggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "123456...TRUNCATED...")

	msgs = processMsg(ag, newMessage("123456"), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "...TRUNCATED...123456...TRUNCATED...")

	require.Empty(t, processMsg(ag, newMessage("123"), startGroup))

	msgs = processMsg(ag, newMessage(""), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "...TRUNCATED...123")
}

// Tests for RegexAggregator

// TestRegexAggregatorNoMatchSendsLinesIndividually anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee PreMatchSinglePass — Before pattern_matched_once
//	                                     becomes true, each call's
//	                                     input is the sole occupant
//	                                     of the buffer … each
//	                                     pre-match emission contains
//	                                     exactly one input line
//
// The delayed-emission shape is intrinsic to the guarantee: a
// pre-match line is buffered on the call that receives it and
// emitted on the next call (or flush). The assertion that the
// first call returns empty AND the second call emits "first line"
// (not "first line\\nsecond line") is what pins "no multi-line
// combination during pre-match."
func TestRegexAggregatorNoMatchSendsLinesIndividually(t *testing.T) {
	re := regexp.MustCompile(`^NEVER_MATCHES_ANYTHING$`)
	ag := NewRegexAggregator(re, 1000, false, status.NewInfoRegistry(), "multi_line")

	msgs := processMsg(ag, newMessage("first line"), noAggregate)
	require.Empty(t, msgs, "first line should be buffered until a second line arrives")

	msgs = processMsg(ag, newMessage("second line"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "first line", string(msgs[0].GetContent()))

	msgs = processMsg(ag, newMessage("third line"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "second line", string(msgs[0].GetContent()))

	msgs = flushMsgs(ag)
	require.Len(t, msgs, 1)
	assert.Equal(t, "third line", string(msgs[0].GetContent()))
}

// TestRegexAggregatorNoMatchThenMatchSwitchesToMultiLine anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee PreMatchSinglePass (sticky bit transition)
//	    @guarantee PatternBoundary — After pattern_matched_once is
//	                                  true, an incoming line whose
//	                                  content satisfies regex_matches
//	                                  is a flush boundary
//
// Exercises the transition out of pre-match: the first line that
// matches the pattern flips pattern_matched_once permanently. From
// that point on, non-matching lines aggregate into the current
// buffer and the next pattern match flushes the combined group.
func TestRegexAggregatorNoMatchThenMatchSwitchesToMultiLine(t *testing.T) {
	re := regexp.MustCompile(`^START`)
	ag := NewRegexAggregator(re, 1000, false, status.NewInfoRegistry(), "multi_line")

	// Lines before the first match are sent individually
	require.Empty(t, processMsg(ag, newMessage("no match line 1"), noAggregate))

	msgs := processMsg(ag, newMessage("no match line 2"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "no match line 1", string(msgs[0].GetContent()))

	// Pattern matches — flushes buffered line, starts multiline aggregation
	msgs = processMsg(ag, newMessage("START of multiline"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "no match line 2", string(msgs[0].GetContent()))

	// Continuation is now aggregated (pattern has matched)
	require.Empty(t, processMsg(ag, newMessage("continuation line"), noAggregate))

	// Next match flushes the combined group
	msgs = processMsg(ag, newMessage("START of second group"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "START of multiline\\ncontinuation line", string(msgs[0].GetContent()))

	msgs = flushMsgs(ag)
	require.Len(t, msgs, 1)
	assert.Equal(t, "START of second group", string(msgs[0].GetContent()))
}

// TestRegexAggregatorFirstLineMatchesWorksNormally anchors:
//
//	surface RegexAggregation (regex_aggregator.allium)
//	    @guarantee PatternBoundary — A line matching the pattern
//	                                  flushes the buffer and starts
//	                                  a new aggregate; non-matching
//	                                  lines are appended.
//	    @guarantee ByteConservation (refined) — Combined emission
//	                                  is the trim-spaced
//	                                  concatenation of contributing
//	                                  lines, joined by the
//	                                  escaped-line-feed separator.
//
// The simplest path: the first line already matches the pattern,
// so pattern_matched_once is set on call 1. Continuation line
// aggregates. Next match flushes the combined "first group" with
// the escaped-line-feed separator between contributing lines.
func TestRegexAggregatorFirstLineMatchesWorksNormally(t *testing.T) {
	re := regexp.MustCompile(`^START`)
	ag := NewRegexAggregator(re, 1000, false, status.NewInfoRegistry(), "multi_line")

	require.Empty(t, processMsg(ag, newMessage("START first group"), noAggregate))
	require.Empty(t, processMsg(ag, newMessage("continuation"), noAggregate))

	msgs := processMsg(ag, newMessage("START second group"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "START first group\\ncontinuation", string(msgs[0].GetContent()))

	msgs = flushMsgs(ag)
	require.Len(t, msgs, 1)
	assert.Equal(t, "START second group", string(msgs[0].GetContent()))
}

// Tests for detectingAggregator

// TestDetectingAggregator_TagsMultilineStartOnly anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee MultiLineDetectionTag — When aggregate follows
//	                                        a pending start_group,
//	                                        the buffered message is
//	                                        emitted with
//	                                        "auto_multiline_detected:true"
//	                                        appended. The aggregate
//	                                        line itself is emitted
//	                                        WITHOUT the tag.
//	    @guarantee StartGroupBufferedUntilNextCall
//	    @guarantee NoLineCombination — both messages emit as
//	                                    separate AggregatedMessage-
//	                                    WithTokens entries, not
//	                                    combined.
//
// Pins the load-bearing detection-tag asymmetry: the tag goes
// only on the start_group line, not on the aggregate that
// triggered it, and not on subsequent aggregates.
func TestDetectingAggregator_TagsMultilineStartOnly(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 100, false, false)

	// startGroup: stored as pending, nothing emitted
	require.Empty(t, processMsg(ag, newMessage("Error: Exception"), startGroup))

	// First aggregate: emits tagged startGroup + current line (leading spaces trimmed)
	msgs := processMsg(ag, newMessage("  at line 1"), aggregate)
	require.Len(t, msgs, 2)
	assert.Equal(t, "Error: Exception", string(msgs[0].GetContent()))
	assert.Contains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
	assert.Equal(t, "at line 1", string(msgs[1].GetContent()))
	assert.NotContains(t, msgs[1].ParsingExtra.Tags, "auto_multiline_detected:true")

	// Subsequent aggregate: emitted immediately without tags (leading spaces trimmed)
	msgs = processMsg(ag, newMessage("  at line 2"), aggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "at line 2", string(msgs[0].GetContent()))
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
}

// TestDetectingAggregator_SingleLineNotTagged anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee MultiLineDetectionTag (negative case) — A
//	                                        start_group flushed by
//	                                        a SUBSEQUENT start_group
//	                                        (not by aggregate) is
//	                                        emitted without the
//	                                        detection tag.
//
// The detection signal is "start_group followed by aggregate" —
// not just "start_group seen." A start_group followed by another
// start_group represents two distinct single-line entries that
// happen to share the start_group classification; neither earns
// the detection tag.
func TestDetectingAggregator_SingleLineNotTagged(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 100, false, false)

	// startGroup: stored
	require.Empty(t, processMsg(ag, newMessage("Single line 1"), startGroup))

	// Another startGroup flushes the previous without tagging
	msgs := processMsg(ag, newMessage("Single line 2"), startGroup)
	require.Len(t, msgs, 1)
	assert.Equal(t, "Single line 1", string(msgs[0].GetContent()))
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")

	// Flush to get the second message
	msgs = flushMsgs(ag)
	require.Len(t, msgs, 1)
	assert.Equal(t, "Single line 2", string(msgs[0].GetContent()))
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
}

// TestDetectingAggregator_NoAggregateOutputsImmediately anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guidance step 3 — If label = noAggregate: emit current
//	                       immediately (after flushing any pending).
//
// Pure noAggregate sequence emits each message on the same call.
// No buffering, no tags applied.
func TestDetectingAggregator_NoAggregateOutputsImmediately(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 100, false, false)

	msgs := processMsg(ag, newMessage("No aggregate 1"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "No aggregate 1", string(msgs[0].GetContent()))
	assert.Empty(t, msgs[0].ParsingExtra.Tags)

	msgs = processMsg(ag, newMessage("No aggregate 2"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "No aggregate 2", string(msgs[0].GetContent()))
	assert.Empty(t, msgs[0].ParsingExtra.Tags)
}

// TestDetectingAggregator_FlushPendingMessage anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee FlushDrainsBuffer — flush emits any pending
//	                                    message and clears state.
//	    @guidance flush procedure — Pending is emitted WITHOUT the
//	                                detection tag (flush is not an
//	                                aggregate trigger).
//
// A buffered start_group that never sees an aggregate label
// is emitted on flush as a plain single-line message.
func TestDetectingAggregator_FlushPendingMessage(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 100, false, false)

	require.Empty(t, processMsg(ag, newMessage("Pending message"), startGroup))

	msgs := flushMsgs(ag)
	require.Len(t, msgs, 1)
	assert.Equal(t, "Pending message", string(msgs[0].GetContent()))
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
}

// TestDetectingAggregator_MixedSingleAndMultiLine anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guidance steps 2 + 4 (label-dispatch interleaving)
//
// Walks a realistic sequence: standalone start_group → next
// start_group (flushes prior untagged) → aggregate (tags this
// pending start + emits cont) → standalone start_group (pending).
// The full per-label dispatch table runs across one test.
func TestDetectingAggregator_MixedSingleAndMultiLine(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 100, false, false)

	// Single line stored
	require.Empty(t, processMsg(ag, newMessage("Single"), startGroup))

	// New startGroup flushes "Single" without tag
	msgs := processMsg(ag, newMessage("Multi start"), startGroup)
	require.Len(t, msgs, 1)
	assert.Equal(t, "Single", string(msgs[0].GetContent()))
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")

	// aggregate: tags "Multi start" and emits + continuation (leading spaces trimmed)
	msgs = processMsg(ag, newMessage("  continuation"), aggregate)
	require.Len(t, msgs, 2)
	assert.Equal(t, "Multi start", string(msgs[0].GetContent()))
	assert.Contains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
	assert.Equal(t, "continuation", string(msgs[1].GetContent()))
	assert.NotContains(t, msgs[1].ParsingExtra.Tags, "auto_multiline_detected:true")

	// Another single line stored
	require.Empty(t, processMsg(ag, newMessage("Another single"), startGroup))

	msgs = flushMsgs(ag)
	require.Len(t, msgs, 1)
	assert.Equal(t, "Another single", string(msgs[0].GetContent()))
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
}

// TestDetectingAggregator_IsEmpty anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee IsEmptyConsistency — is_empty reflects
//	                                     pending_message_present
//	                                     inverted exactly.
//
// is_empty tracks the buffered-message state: true at
// construction, false while a start_group is pending, true after
// flush drains it, true after a noAggregate emission (which never
// buffers).
func TestDetectingAggregator_IsEmpty(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 100, false, false)

	assert.True(t, ag.IsEmpty())

	require.Empty(t, processMsg(ag, newMessage("Pending"), startGroup))
	assert.False(t, ag.IsEmpty())

	msgs := flushMsgs(ag)
	require.Len(t, msgs, 1)
	assert.True(t, ag.IsEmpty())

	msgs = processMsg(ag, newMessage("Immediate"), noAggregate)
	require.Len(t, msgs, 1)
	assert.True(t, ag.IsEmpty())
}

// TestDetectingAggregator_TruncatesTaggedStartLineAndPrefixesContinuation anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee PerEmissionTruncationFlow — Identical in shape
//	                                           to PassThrough's:
//	                                           rolling carry,
//	                                           prepend on prior,
//	                                           append on overflow.
//	    @guarantee MultiLineDetectionTag (interaction with
//	                                      truncation tagging)
//
// Three properties verified at once:
//  1. The start_group line gets truncated (oversize) → marker
//     appended, is_truncated set, "single_line" truncation tag,
//     AND the detection tag.
//  2. The carry from emission #1 prepends the marker to
//     emission #2's content.
//  3. The detection tag stays on the start_group line only —
//     the aggregate line carries no detection tag.
func TestDetectingAggregator_TruncatesTaggedStartLineAndPrefixesContinuation(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 5, true, false)

	require.Empty(t, processMsg(ag, newMessage("123456"), startGroup))

	msgs := processMsg(ag, newMessage("abc"), aggregate)
	require.Len(t, msgs, 2)

	assert.Equal(t, "123456...TRUNCATED...", string(msgs[0].GetContent()))
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Contains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
	assert.Contains(t, msgs[0].ParsingExtra.Tags, message.TruncatedReasonTag("single_line"))

	assert.Equal(t, "...TRUNCATED...abc", string(msgs[1].GetContent()))
	assert.True(t, msgs[1].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("single_line")}, msgs[1].ParsingExtra.Tags)
}

// TestDetectingAggregator_NoAggregateInheritsTruncationCarry anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee PerEmissionTruncationFlow (carry across noAggregate)
//
// The truncation carry follows the SEQUENCE of emissions, not the
// label sequence — a noAggregate that overflows sets the carry,
// and the next noAggregate (which emits immediately) gets the
// prepended marker.
func TestDetectingAggregator_NoAggregateInheritsTruncationCarry(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 5, true, false)

	msgs := processMsg(ag, newMessage("123456"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "123456...TRUNCATED...", string(msgs[0].GetContent()))

	msgs = processMsg(ag, newMessage("ok"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "...TRUNCATED...ok", string(msgs[0].GetContent()))
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("single_line")}, msgs[0].ParsingExtra.Tags)
}

// TestDetectingAggregator_StartGroupInheritsTruncationCarry anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee PerEmissionTruncationFlow (carry across the
//	                                           start_group buffer)
//
// The carry crosses the start_group buffering boundary: a
// noAggregate overflow sets the carry, then a start_group is
// buffered (no emission), then an aggregate flushes the buffered
// start_group — which now gets the prepended marker from the
// carry. After that emission, the carry is consumed; the
// aggregate's own emission is unaffected.
func TestDetectingAggregator_StartGroupInheritsTruncationCarry(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 5, true, false)

	msgs := processMsg(ag, newMessage("123456"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "123456...TRUNCATED...", string(msgs[0].GetContent()))

	require.Empty(t, processMsg(ag, newMessage("abc"), startGroup))

	msgs = processMsg(ag, newMessage("tail"), aggregate)
	require.Len(t, msgs, 2)
	assert.Equal(t, "...TRUNCATED...abc", string(msgs[0].GetContent()))
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Contains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
	assert.Contains(t, msgs[0].ParsingExtra.Tags, message.TruncatedReasonTag("single_line"))

	assert.Equal(t, "tail", string(msgs[1].GetContent()))
	assert.False(t, msgs[1].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[1].ParsingExtra.Tags)
}

// TestDetectingAggregator_TruncationDoesNotTagWhenDisabled anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guarantee PerEmissionTruncationFlow — the truncation-
//	                                           reason tag is
//	                                           attached when
//	                                           tag_truncated_logs
//	                                           is true (and only
//	                                           then).
//
// With tag_truncated_logs = false at construction, a truncated
// emission still has is_truncated set and the appended marker —
// but the "truncated:single_line" tag is absent. Note that
// DetectingAggregator takes tag_truncated_logs as a constructor
// argument, NOT from global runtime config (unlike PassThrough
// and Regex).
func TestDetectingAggregator_TruncationDoesNotTagWhenDisabled(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 5, false, false)

	msgs := processMsg(ag, newMessage("123456"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "123456...TRUNCATED...", string(msgs[0].GetContent()))
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[0].ParsingExtra.Tags)
}

// TestDetectingAggregator_UsesExistingTruncatedFlag anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guidance SingleLineTruncationFlow step 1 — should_truncate
//	                                                 becomes true
//	                                                 iff the input
//	                                                 message's
//	                                                 is_truncated
//	                                                 is already
//	                                                 true (OR
//	                                                 content exceeds
//	                                                 line_limit).
//
// An input message whose is_truncated flag is already set
// triggers the truncation flow even when content fits within
// line_limit. The marker is appended; the rolling carry is set.
func TestDetectingAggregator_UsesExistingTruncatedFlag(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 100, true, false)

	msg := newMessage("already-truncated")
	msg.ParsingExtra.IsTruncated = true

	msgs := processMsg(ag, msg, noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "already-truncated...TRUNCATED...", string(msgs[0].GetContent()))
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("single_line")}, msgs[0].ParsingExtra.Tags)
}

// COAT telemetry tests

// AGNTLOG-617: The DetectingAggregator must TrimSpace content just like SingleLineHandler,
// PassThroughAggregator, and CombiningAggregator do. Without TrimSpace, trailing \r from
// CRLF line endings (or other trailing whitespace) breaks anchored log_processing_rules
// like ^\{.*\}$ because the anchor can't match past the trailing bytes.
// TestDetectingAggregator_TrimSpaceMatchesOtherAggregators anchors:
//
//	surface DetectingAggregation (detecting_aggregator.allium)
//	    @guidance SingleLineTruncationFlow step 2 — Trim leading
//	                                                 and trailing
//	                                                 whitespace
//	                                                 from
//	                                                 emit_msg.content.
//
// The trim-space step ensures trailing whitespace (notably \r
// from windows-style framing) does not appear in emitted content.
// Compares behavior against PassThrough and Combining as a
// cross-aggregator sanity check.
func TestDetectingAggregator_TrimSpaceMatchesOtherAggregators(t *testing.T) {
	jsonPattern := regexp.MustCompile(`^\{.*\}$`)

	testCases := []struct {
		name    string
		content string
		label   Label
	}{
		{"trailing CR (noAggregate)", `{"key":"val"}` + "\r", noAggregate},
		{"trailing space (noAggregate)", `{"key":"val"}` + " ", noAggregate},
		{"trailing tab (noAggregate)", `{"key":"val"}` + "\t", noAggregate},
		{"leading space (noAggregate)", " " + `{"key":"val"}`, noAggregate},
		{"leading+trailing whitespace (noAggregate)", " \t" + `{"key":"val"}` + "\r ", noAggregate},
		{"trailing CR (aggregate)", `{"key":"val"}` + "\r", aggregate},
		{"trailing CR (startGroup then flush)", `{"key":"val"}` + "\r", startGroup},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ag := NewDetectingAggregator(status.NewInfoRegistry(), 100, false, false)

			var msgs []*message.Message
			msgs = processMsg(ag, newMessage(tc.content), tc.label)
			if tc.label == startGroup {
				require.Empty(t, msgs)
				msgs = flushMsgs(ag)
			}
			require.Len(t, msgs, 1)

			emitted := string(msgs[0].GetContent())
			assert.True(t, jsonPattern.MatchString(emitted),
				"anchored pattern should match trimmed content %q but got %q", `{"key":"val"}`, emitted)
			assert.Equal(t, `{"key":"val"}`, emitted,
				"content should be trimmed to match SingleLineHandler / PassThroughAggregator behavior")
		})
	}
}

// TestPassThroughAndCombiningAggregator_TrimSpaceBaseline anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guidance flush_bucket / emit_single — trim-space the
//	                                            combined / single
//	                                            content.
//	(plus the analogous step in PassThroughAggregation —
//	anchored from this same test for the PassThrough side.)
//
// Verifies that both PassThroughAggregator and CombiningAggregator
// trim trailing whitespace (notably \r from windows-style framing).
// The aggregators' shared use of bytes.TrimSpace before emission
// ensures the JSON-pattern regex matches.
func TestPassThroughAndCombiningAggregator_TrimSpaceBaseline(t *testing.T) {
	jsonPattern := regexp.MustCompile(`^\{.*\}$`)
	contentWithCR := `{"key":"val"}` + "\r"

	t.Run("PassThroughAggregator", func(t *testing.T) {
		ag := NewPassThroughAggregator(100)
		msgs := processMsg(ag, newMessage(contentWithCR), noAggregate)
		require.Len(t, msgs, 1)
		emitted := string(msgs[0].GetContent())
		assert.True(t, jsonPattern.MatchString(emitted),
			"PassThroughAggregator should trim; got %q", emitted)
	})

	t.Run("CombiningAggregator_noAggregate", func(t *testing.T) {
		ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())
		msgs := processMsg(ag, newMessage(contentWithCR), noAggregate)
		require.Len(t, msgs, 1)
		emitted := string(msgs[0].GetContent())
		assert.True(t, jsonPattern.MatchString(emitted),
			"CombiningAggregator should trim; got %q", emitted)
	})
}

func TestDetectingAggregator_COATTelemetry_WouldCombine(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 1000, false, true)

	totalBefore := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineBefore := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	truncBefore := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()

	// startGroup followed by two aggregates: both aggregates would be combined
	ag.Process(newMessage("timestamp line"), startGroup, nil)
	ag.Process(newMessage("  continuation 1"), aggregate, nil)
	ag.Process(newMessage("  continuation 2"), aggregate, nil)
	ag.Flush()

	totalAfter := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineAfter := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	truncAfter := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()

	assert.Equal(t, float64(3), totalAfter-totalBefore)
	assert.Equal(t, float64(3), combineAfter-combineBefore)
	assert.Equal(t, float64(0), truncAfter-truncBefore)
}

func TestDetectingAggregator_COATTelemetry_NoCombineForStandaloneAggregates(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 1000, false, true)

	totalBefore := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineBefore := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	truncBefore := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()

	// Aggregate lines without a preceding startGroup should NOT count as would-combine
	ag.Process(newMessage("orphan 1"), aggregate, nil)
	ag.Process(newMessage("orphan 2"), aggregate, nil)

	totalAfter := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineAfter := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	truncAfter := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()
	assert.Equal(t, float64(2), totalAfter-totalBefore)
	assert.Equal(t, float64(0), combineAfter-combineBefore)
	assert.Equal(t, float64(0), truncAfter-truncBefore)
}

func TestDetectingAggregator_COATTelemetry_OverflowingGroupsAreNotCombined(t *testing.T) {
	// maxContentSize=20 so combining would overflow and be abandoned
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 20, false, true)

	truncBefore := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()
	totalBefore := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineBefore := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()

	// startGroup(10 bytes) + aggregate(15 bytes) would overflow once the escaped
	// newline separator is added, so combining mode would abandon aggregation.
	ag.Process(newMessage("1234567890"), startGroup, nil)     // 10 bytes content
	ag.Process(newMessage("123456789012345"), aggregate, nil) // 10+2+15 >= 20 → do not combine

	truncAfter := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()
	totalAfter := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineAfter := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()

	assert.Equal(t, float64(0), truncAfter-truncBefore)
	assert.Equal(t, float64(2), totalAfter-totalBefore)
	assert.Equal(t, float64(0), combineAfter-combineBefore)
}

func TestDetectingAggregator_COATTelemetry_LateOverflowDropsWholeGroup(t *testing.T) {
	// The first three lines fit together, but the fourth would overflow and cause
	// combining mode to abandon aggregation for the whole group.
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 15, false, true)

	totalBefore := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineBefore := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	truncBefore := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()

	ag.Process(newMessage("1234"), startGroup, nil) // 4
	ag.Process(newMessage("12"), aggregate, nil)    // 4+2+2 = 8
	ag.Process(newMessage("12"), aggregate, nil)    // 8+2+2 = 12
	ag.Process(newMessage("12"), aggregate, nil)    // 12+2+2 = 16 >= 15, abandon group

	totalAfter := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineAfter := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	truncAfter := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()

	assert.Equal(t, float64(4), totalAfter-totalBefore)
	assert.Equal(t, float64(0), combineAfter-combineBefore)
	assert.Equal(t, float64(0), truncAfter-truncBefore)
}

func TestDetectingAggregator_COATTelemetry_NoTruncateForOversizedSingleLine(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 5, false, true)

	totalBefore := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	truncBefore := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()
	combineBefore := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()

	// A single startGroup >= maxContentSize is excluded from truncation counts
	// (it would be truncated regardless of auto-multiline)
	ag.Process(newMessage("12345"), startGroup, nil) // RawDataLen=5 >= maxContentSize=5
	ag.Process(newMessage("67"), aggregate, nil)     // Not in group since startGroup was oversized

	totalAfter := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineAfter := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	truncAfter := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()
	assert.Equal(t, float64(0), truncAfter-truncBefore)
	assert.Equal(t, float64(2), totalAfter-totalBefore)
	assert.Equal(t, float64(0), combineAfter-combineBefore)
}

func TestDetectingAggregator_COATTelemetry_NoCountsWhenNotDefaultPath(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 1000, false, false)

	totalBefore := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineBefore := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	truncBefore := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()

	ag.Process(newMessage("timestamp line"), startGroup, nil)
	ag.Process(newMessage("  continuation"), aggregate, nil)

	totalAfter := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineAfter := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	truncAfter := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()

	assert.Equal(t, float64(0), totalAfter-totalBefore)
	assert.Equal(t, float64(0), combineAfter-combineBefore)
	assert.Equal(t, float64(0), truncAfter-truncBefore)
}

func TestDetectingAggregator_COATTelemetry_MultiGroupWithOverflow(t *testing.T) {
	// maxContentSize=15 to make the second group overflow and be abandoned
	ag := NewDetectingAggregator(status.NewInfoRegistry(), 15, false, true)

	totalBefore := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineBefore := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	truncBefore := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()

	// Group 1: fits (5 content + 2 LF + 3 content = 10 < 15)
	ag.Process(newMessage("12345"), startGroup, nil) // 5 bytes
	ag.Process(newMessage("678"), aggregate, nil)    // 5+2+3 = 10 < 15 → combine

	// Group 2: would overflow (10+2+10 = 22 >= 15), so it is not counted as combined.
	ag.Process(newMessage("1234567890"), startGroup, nil) // 10 bytes, starts new group
	ag.Process(newMessage("1234567890"), aggregate, nil)  // 10+2+10 >= 15 → abandon aggregation
	ag.Process(newMessage("123"), aggregate, nil)         // Aggregate out of a group should not count as would-combine

	// noAggregate standalone
	ag.Process(newMessage("standalone"), noAggregate, nil)

	totalAfter := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineAfter := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	truncAfter := metrics.TlmAutoMultilineWouldTruncate.WithValues().Get()

	assert.Equal(t, float64(6), totalAfter-totalBefore)
	assert.Equal(t, float64(2), combineAfter-combineBefore) // only group 1 is combined
	assert.Equal(t, float64(0), truncAfter-truncBefore)
}
