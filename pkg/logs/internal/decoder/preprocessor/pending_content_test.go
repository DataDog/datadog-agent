// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// handoff moves whatever from is buffering into to, the way a file rotation
// does, and asserts something was actually carried over.
func handoff(t *testing.T, from, to PendingContentCarrier) {
	t.Helper()
	pending := from.TakePendingContent()
	require.NotEmpty(t, pending, "the stage should have had buffered content to hand over")
	to.SeedPendingContent(pending)
}

// TestJSONAggregatorPendingContentRoundTrip covers a pretty-printed JSON object
// split by a rotation: the halves must end up aggregated by the replacement
// stage exactly as a single stage would have aggregated them.
func TestJSONAggregatorPendingContentRoundTrip(t *testing.T) {
	before := `{"message": "this is a",`
	after := `"level": "info"}`

	// Reference: what one aggregator produces for the same two lines.
	reference := NewJSONAggregator(true, 1000)
	assert.Empty(t, reference.Process(newTestMessage(before)))
	want := reference.Process(newTestMessage(after))
	require.Len(t, want, 1)

	old := NewJSONAggregator(true, 1000)
	assert.Empty(t, old.Process(newTestMessage(before)))
	assert.False(t, old.IsEmpty(), "the opening half should be buffered")

	replacement := NewJSONAggregator(true, 1000)
	handoff(t, old.(PendingContentCarrier), replacement.(PendingContentCarrier))

	assert.True(t, old.IsEmpty(), "the old stage must not keep content it handed over")
	assert.False(t, replacement.IsEmpty(), "the replacement stage must be holding the carried-over half")

	got := replacement.Process(newTestMessage(after))
	require.Len(t, got, 1, "the closing half should complete the carried-over object")
	assert.Equal(t, string(want[0].GetContent()), string(got[0].GetContent()))
}

// TestJSONAggregatorPendingContentFlushesWhenNotCompleted checks the
// abandonment side: a carried-over half that is never completed is still
// emitted, not swallowed.
func TestJSONAggregatorPendingContentFlushesWhenNotCompleted(t *testing.T) {
	old := NewJSONAggregator(true, 1000)
	assert.Empty(t, old.Process(newTestMessage(`{"message": "this is a",`)))

	replacement := NewJSONAggregator(true, 1000)
	handoff(t, old.(PendingContentCarrier), replacement.(PendingContentCarrier))

	flushed := replacement.Flush()
	require.Len(t, flushed, 1)
	assert.Equal(t, `{"message": "this is a",`, string(flushed[0].GetContent()))
}

// TestStackTraceAggregatorPendingContentRoundTrip covers a Go stack trace split
// by a rotation. The parser behind this stage is stateful, so the carried-over
// lines have to be replayed through it for the continuation to still be
// recognised.
func TestStackTraceAggregatorPendingContentRoundTrip(t *testing.T) {
	trace := "panic: something went wrong\n\ngoroutine 1 [running]:\nmain.plainPanic(...)\n\t/path/main.go:81\nmain.main()\n\t/path/main.go:46 +0x5b4"
	lines := strings.Split(trace, "\n")
	split := 3

	// Reference: the same trace through a single aggregator.
	reference := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize, true)
	want := feedLines(reference, trace+"\n")
	assertCombined(t, want)

	old := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize, true)
	for _, line := range lines[:split] {
		assert.Empty(t, old.Process(makeMsg(line)), "the trace should still be buffering")
	}
	assert.False(t, old.IsEmpty())

	replacement := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize, true)
	handoff(t, old.(PendingContentCarrier), replacement.(PendingContentCarrier))

	assert.True(t, old.IsEmpty(), "the old stage must not keep content it handed over")
	assert.False(t, replacement.IsEmpty(), "the replacement stage must be holding the carried-over lines")

	var got []*message.Message
	for _, line := range lines[split:] {
		got = append(got, replacement.Process(makeMsg(line))...)
	}
	got = append(got, replacement.Flush()...)

	assertCombined(t, got)
	assert.Equal(t, string(want[0].GetContent()), string(got[0].GetContent()),
		"a trace spanning the rotation should combine into the same message as an uninterrupted one")
}

// TestStackTraceAggregatorPendingContentFlushesWhenNotCombined checks that
// carried-over lines the parser ends up rejecting are still emitted.
func TestStackTraceAggregatorPendingContentFlushesWhenNotCombined(t *testing.T) {
	old := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize, true)
	assert.Empty(t, old.Process(makeMsg("panic: something went wrong")))

	replacement := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize, true)
	handoff(t, old.(PendingContentCarrier), replacement.(PendingContentCarrier))

	flushed := replacement.Flush()
	require.Len(t, flushed, 1)
	assert.Equal(t, "panic: something went wrong", string(flushed[0].GetContent()))
}

// TestPreprocessorPendingContentRoutesPerStage checks that the Preprocessor
// consults every buffering sub-stage and puts each piece of content back where
// it came from, rather than assuming which stage is holding it.
func TestPreprocessorPendingContentRoutesPerStage(t *testing.T) {
	newPreprocessor := func() *Preprocessor {
		out := make(chan *message.Message, 10)
		return NewPreprocessor(
			NewRegexAggregator(regexp.MustCompile(`^\d{4}`), testMaxContentSize, false, status.NewInfoRegistry(), "multi_line"),
			NewTokenizer(0), NewNoopLabeler(), NewNoopSampler(), out,
			NewJSONAggregator(true, 1000),
			NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize, true),
			time.Second, 0)
	}

	t.Run("json stage", func(t *testing.T) {
		old := newPreprocessor()
		old.Process(newTestMessage(`{"message": "this is a",`))

		pending := old.TakePendingContent()
		require.Len(t, pending, 1)
		assert.Equal(t, StageJSONAggregation, pending[0].Stage)

		replacement := newPreprocessor()
		replacement.SeedPendingContent(pending)
		assert.False(t, replacement.jsonAggregator.IsEmpty(), "content must land back in the JSON stage")
		assert.True(t, replacement.aggregator.IsEmpty())
		assert.True(t, replacement.stackTraceAggregator.IsEmpty())
	})

	t.Run("stack trace stage", func(t *testing.T) {
		old := newPreprocessor()
		old.Process(makeMsg("panic: something went wrong"))

		pending := old.TakePendingContent()
		require.Len(t, pending, 1)
		assert.Equal(t, StageStackTraceAggregation, pending[0].Stage)

		replacement := newPreprocessor()
		replacement.SeedPendingContent(pending)
		assert.False(t, replacement.stackTraceAggregator.IsEmpty(), "content must land back in the stack trace stage")
		assert.True(t, replacement.jsonAggregator.IsEmpty())
		assert.True(t, replacement.aggregator.IsEmpty())
	})

	t.Run("line aggregation stage", func(t *testing.T) {
		old := newPreprocessor()
		old.Process(makeMsg("2026 group start"))

		pending := old.TakePendingContent()
		require.Len(t, pending, 1)
		assert.Equal(t, StageLineAggregation, pending[0].Stage)

		replacement := newPreprocessor()
		replacement.SeedPendingContent(pending)
		assert.False(t, replacement.aggregator.IsEmpty(), "content must land back in the line aggregation stage")
		assert.True(t, replacement.jsonAggregator.IsEmpty())
		assert.True(t, replacement.stackTraceAggregator.IsEmpty())
	})
}
