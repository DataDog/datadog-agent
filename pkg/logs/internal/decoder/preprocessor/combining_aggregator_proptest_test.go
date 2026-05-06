// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"bytes"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// Property tests for the CombiningAggregation surface declared in
// combining_aggregator.allium. Each test names the spec
// @guarantee it anchors so that drift in either direction is easy
// to spot during review.
//
// Anchoring unit tests for the same surface live in
// combining_aggregator_test.go and aggregator_test.go.

type combiningCall struct {
	content string
	label   Label
	tokens  []Token
}

func genCombiningCall() *rapid.Generator[combiningCall] {
	return rapid.Custom(func(t *rapid.T) combiningCall {
		content := string(rapid.SliceOfN(
			rapid.SampledFrom([]byte("abc 012")),
			0, 20,
		).Draw(t, "content"))
		label := rapid.SampledFrom([]Label{startGroup, noAggregate, aggregate}).Draw(t, "label")
		nTokens := rapid.IntRange(0, 4).Draw(t, "nTokens")
		tokens := make([]Token, nTokens)
		for i := 0; i < nTokens; i++ {
			tokens[i] = Token(rapid.IntRange(0, int(End)-1).Draw(t, "token"))
		}
		return combiningCall{content: content, label: label, tokens: tokens}
	})
}

// combiningEmission captures one emitted message's observable state.
type combiningEmission struct {
	content     string
	isTruncated bool
	isMultiLine bool
	tagsLen     int
	tokensLen   int
}

func runCombiningCalls(lineLimit int, tagTrunc, tagMulti bool, calls []combiningCall) []combiningEmission {
	ag := NewCombiningAggregator(lineLimit, tagTrunc, tagMulti, status.NewInfoRegistry())
	var out []combiningEmission
	for _, c := range calls {
		emitted := ag.Process(newMessage(c.content), c.label, c.tokens)
		for _, e := range emitted {
			out = append(out, combiningEmission{
				content:     string(e.Msg.GetContent()),
				isTruncated: e.Msg.ParsingExtra.IsTruncated,
				isMultiLine: e.Msg.ParsingExtra.IsMultiLine,
				tagsLen:     len(e.Msg.ParsingExtra.Tags),
				tokensLen:   len(e.Tokens),
			})
		}
	}
	for _, e := range ag.Flush() {
		out = append(out, combiningEmission{
			content:     string(e.Msg.GetContent()),
			isTruncated: e.Msg.ParsingExtra.IsTruncated,
			isMultiLine: e.Msg.ParsingExtra.IsMultiLine,
			tagsLen:     len(e.Msg.ParsingExtra.Tags),
			tokensLen:   len(e.Tokens),
		})
	}
	return out
}

// TestCombiningAggregator_Determinism_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee Determinism
//
// Two aggregators of identical configuration, fed identical call
// sequences, produce equal observed emission sequences.
func TestCombiningAggregator_Determinism_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genCombiningCall(), 1, 12).Draw(t, "calls")
		lineLimit := rapid.IntRange(1, 100).Draw(t, "lineLimit")

		outA := runCombiningCalls(lineLimit, false, false, calls)
		outB := runCombiningCalls(lineLimit, false, false, calls)

		if len(outA) != len(outB) {
			t.Fatalf("Determinism violated: emission counts %d vs %d", len(outA), len(outB))
		}
		for i := range outA {
			if outA[i] != outB[i] {
				t.Fatalf("Determinism violated at emission %d: %+v vs %+v", i, outA[i], outB[i])
			}
		}
	})
}

// TestCombiningAggregator_FlushClearsBucket_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee FlushDrainsBuffer — after flush, bucket_empty
//	                                    is true.
//	    @guarantee IsEmptyConsistency
//
// Across arbitrary call sequences, flush followed by is_empty
// always yields true. A second flush returns empty.
func TestCombiningAggregator_FlushClearsBucket_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genCombiningCall(), 0, 12).Draw(t, "calls")
		ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())
		for _, c := range calls {
			ag.Process(newMessage(c.content), c.label, c.tokens)
		}
		ag.Flush()
		if !ag.IsEmpty() {
			t.Fatal("FlushDrainsBuffer violated: is_empty false after flush")
		}
		second := ag.Flush()
		if len(second) != 0 {
			t.Fatalf("FlushDrainsBuffer violated: second flush returned %d messages, expected 0", len(second))
		}
	})
}

// TestCombiningAggregator_NoCombinedOverflow_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee OverflowExplosion — CombiningAggregator never
//	                                    produces a combined
//	                                    emission whose body
//	                                    crosses line_limit purely
//	                                    because of multi-line
//	                                    aggregation.
//
// The strong form: across arbitrary call sequences with arbitrary
// line_limits, no emission's content (minus markers) STRICTLY
// EXCEEDS line_limit when the emission is multi-line
// (is_multi_line set). A 2+ line combined emission staying at or
// under line_limit is the observable manifestation of
// OverflowExplosion's "abandon combination rather than
// truncate-combine" policy.
//
// Note on the "<=" rather than "<" bound: a multi-line emission's
// body can equal line_limit when trim-spacing reduces a buffer
// that crossed the limit by exactly the separator bytes (e.g. an
// empty-content first contributor + a limit-sized second line:
// buffer reaches line_limit + 2 pre-trim, trims down to
// line_limit). The OverflowExplosion @guarantee is about not
// EXCEEDING the limit through aggregation; equality is permitted.
func TestCombiningAggregator_NoCombinedOverflow_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		calls := rapid.SliceOfN(genCombiningCall(), 1, 12).Draw(t, "calls")
		// line_limit must be big enough to fit at least small inputs,
		// otherwise everything goes through the oversize-single-line
		// paths and the multi-line case never fires. >= 20 covers
		// typical small generated content.
		lineLimit := rapid.IntRange(20, 200).Draw(t, "lineLimit")

		ag := NewCombiningAggregator(lineLimit, false, false, status.NewInfoRegistry())
		marker := string(message.TruncatedFlag)
		check := func(emitted []AggregatedMessageWithTokens) {
			for _, e := range emitted {
				if !e.Msg.ParsingExtra.IsMultiLine {
					continue
				}
				body := strings.TrimPrefix(string(e.Msg.GetContent()), marker)
				body = strings.TrimSuffix(body, marker)
				if len(body) > lineLimit {
					t.Fatalf("NoCombinedOverflow violated: multi-line emission body %d bytes > line_limit %d (content %q)",
						len(body), lineLimit, e.Msg.GetContent())
				}
			}
		}
		for _, c := range calls {
			check(ag.Process(newMessage(c.content), c.label, c.tokens))
		}
		check(ag.Flush())
	})
}

// TestCombiningAggregator_NoAggregateClearsCarry_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee NoAggregateResetsCarry — no_aggregate resets
//	                                         should_truncate to
//	                                         false before
//	                                         processing.
//
// The observable manifestation: across arbitrary inputs, a
// no_aggregate-labelled line whose content fits within line_limit
// and is not upstream-flagged never emerges with a prepended
// truncation marker — even if the prior emission set the carry.
//
// We construct a deterministic preamble that sets the carry (an
// oversized startGroup), then sample a no_aggregate continuation.
// The no_aggregate line must not have the prepend marker.
func TestCombiningAggregator_NoAggregateClearsCarry_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(8, 50).Draw(t, "lineLimit")
		// Construct fitting content for the no_aggregate line: must
		// be <= line_limit and not upstream-flagged.
		contentLen := rapid.IntRange(1, lineLimit).Draw(t, "contentLen")
		content := strings.Repeat("x", contentLen)
		ag := NewCombiningAggregator(lineLimit, false, false, status.NewInfoRegistry())

		// Set the carry: oversized startGroup. Forces an immediate
		// single-line truncated emission and leaves should_truncate
		// set to true.
		oversized := strings.Repeat("y", lineLimit*2)
		msg := newMessage(oversized)
		msg.RawDataLen = len(oversized)
		first := ag.Process(msg, startGroup, nil)
		if len(first) != 1 {
			return // skip degenerate cases where startGroup doesn't immediately flush
		}

		// no_aggregate next: should NOT receive the prepended marker.
		nextMsg := newMessage(content)
		nextMsg.RawDataLen = len(content)
		emitted := ag.Process(nextMsg, noAggregate, nil)
		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission for fitting no_aggregate, got %d", len(emitted))
		}
		emittedContent := string(emitted[0].Msg.GetContent())
		if strings.HasPrefix(emittedContent, string(message.TruncatedFlag)) {
			t.Fatalf("NoAggregateResetsCarry violated: no_aggregate emission has prepended marker (content=%q)", emittedContent)
		}
	})
}

// combiningCapture is a deep-copied snapshot of one emission's
// observable state, including the full tag list (the existing
// combiningEmission tracks only tag count). Used for tests that
// must compare emissions across Process calls — required by the
// Aggregator contract's ResultLifetime invariant.
type combiningCapture struct {
	content     []byte
	isTruncated bool
	isMultiLine bool
	tags        []string
}

func captureCombiningEmissions(emitted []AggregatedMessageWithTokens) []combiningCapture {
	out := make([]combiningCapture, len(emitted))
	for i, e := range emitted {
		out[i] = combiningCapture{
			content:     append([]byte(nil), e.Msg.GetContent()...),
			isTruncated: e.Msg.ParsingExtra.IsTruncated,
			isMultiLine: e.Msg.ParsingExtra.IsMultiLine,
			tags:        append([]string(nil), e.Msg.ParsingExtra.Tags...),
		}
	}
	return out
}

func msgWithFlag(content string, isTruncated bool) *message.Message {
	m := newMessage(content)
	m.ParsingExtra.IsTruncated = isTruncated
	return m
}

// TestCombiningAggregator_PassThroughUnderThreshold_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee ByteConservation — emissions whose buffered
//	                                   content stays under
//	                                   line_limit carry no
//	                                   truncation markers.
//
// A single-line noAggregate emission under threshold (no carry,
// no upstream flag) emits with no markers and IsTruncated=false.
// This pins the baseline clean-emission case for the bucket.flush
// single-line path.
func TestCombiningAggregator_PassThroughUnderThreshold_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(50, 200).Draw(t, "lineLimit")
		content := strings.Repeat("a", rapid.IntRange(1, lineLimit/2).Draw(t, "len"))

		ag := NewCombiningAggregator(lineLimit, false, false, status.NewInfoRegistry())
		emitted := captureCombiningEmissions(ag.Process(newMessage(content), noAggregate, nil))

		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission for fitting noAggregate, got %d", len(emitted))
		}
		e := emitted[0]
		marker := message.TruncatedFlag
		if bytes.Contains(e.content, marker) {
			t.Fatalf("PassThroughUnderThreshold violated: marker found in clean emission %q", e.content)
		}
		if e.isTruncated {
			t.Fatalf("PassThroughUnderThreshold violated: IsTruncated=true on clean emission %q", e.content)
		}
	})
}

// TestCombiningAggregator_SingleLineOversizedTailMarker_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee StartGroupBoundary — a startGroup whose own
//	                                     RawDataLen is at or above
//	                                     line_limit triggers an
//	                                     immediate single-line
//	                                     flush via the truncation
//	                                     flow.
//
// An oversized startGroup is emitted immediately with the tail
// marker appended (single-line truncated emission via
// bucket.flush with contentLen >= lineLimit).
func TestCombiningAggregator_SingleLineOversizedTailMarker_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(5, 50).Draw(t, "lineLimit")
		oversized := strings.Repeat("x", lineLimit+rapid.IntRange(1, 30).Draw(t, "extra"))

		ag := NewCombiningAggregator(lineLimit, false, false, status.NewInfoRegistry())
		emitted := captureCombiningEmissions(ag.Process(newMessage(oversized), startGroup, nil))

		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission for oversized startGroup, got %d", len(emitted))
		}
		e := emitted[0]
		marker := message.TruncatedFlag
		if !bytes.HasSuffix(e.content, marker) {
			t.Fatalf("SingleLineOversizedTailMarker violated: no tail marker on oversized startGroup emission %q", e.content)
		}
		if !e.isTruncated {
			t.Fatal("SingleLineOversizedTailMarker violated: IsTruncated=false on truncated emission")
		}
	})
}

// TestCombiningAggregator_HeadMarkerOnCarryover_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee TruncationTagging — the should_truncate carry
//	                                    propagates the head marker
//	                                    to the next emission.
//
// After an oversized startGroup sets the carry, the next
// emission (here an aggregate-on-empty-bucket under threshold)
// receives the head marker. The carry transcends bucket lifecycle.
func TestCombiningAggregator_HeadMarkerOnCarryover_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(20, 60).Draw(t, "lineLimit")
		oversized := strings.Repeat("x", lineLimit+rapid.IntRange(1, 30).Draw(t, "extra"))
		nextLen := rapid.IntRange(1, 5).Draw(t, "nextLen")
		next := strings.Repeat("a", nextLen)

		ag := NewCombiningAggregator(lineLimit, false, false, status.NewInfoRegistry())
		// Oversized startGroup: emits single-line truncated, carry set.
		first := captureCombiningEmissions(ag.Process(newMessage(oversized), startGroup, nil))
		if len(first) != 1 {
			return // skip degenerate
		}
		// Next aggregate-on-empty-bucket: under threshold, carry
		// is consumed → head marker.
		second := captureCombiningEmissions(ag.Process(newMessage(next), aggregate, nil))

		if len(second) != 1 {
			t.Fatalf("expected 1 emission for aggregate-on-empty, got %d", len(second))
		}
		e := second[0]
		marker := message.TruncatedFlag
		if !bytes.HasPrefix(e.content, marker) {
			t.Fatalf("HeadMarkerOnCarryover violated: no head marker on emission after carry; content %q", e.content)
		}
		if !e.isTruncated {
			t.Fatal("HeadMarkerOnCarryover violated: IsTruncated=false on emission with head marker")
		}
	})
}

// TestCombiningAggregator_BucketFlushIgnoresUpstreamFlag_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee BucketFlushIgnoresUpstreamFlag — the
//	                                                 bucket.flush()
//	                                                 path does NOT
//	                                                 honour upstream
//	                                                 IsTruncated
//	                                                 flags on
//	                                                 contributing
//	                                                 lines. This
//	                                                 test covers
//	                                                 the combined-
//	                                                 emission case
//	                                                 specifically;
//	                                                 the @guarantee
//	                                                 itself applies
//	                                                 to all
//	                                                 bucket.flush()
//	                                                 paths.
//
// LOAD-BEARING for the refactor safety net. A combined emission
// of 2+ lines where the LATER contributor (not the leader)
// arrived with upstream IsTruncated=true MUST emit with no
// markers and IsTruncated=false (assuming no overflow). The
// upstream signal from non-leader contributors is fully dropped
// — both content-wise (no markers) and flag-wise (since the
// emitted message's IsTruncated mirrors only the leader's
// pre-emit state, which here is false).
func TestCombiningAggregator_BucketFlushIgnoresUpstreamFlag_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := 200 // large, no overflow
		leaderLen := rapid.IntRange(1, 20).Draw(t, "leaderLen")
		flaggedLen := rapid.IntRange(1, 20).Draw(t, "flaggedLen")
		leader := strings.Repeat("l", leaderLen)
		flagged := strings.Repeat("f", flaggedLen)

		ag := NewCombiningAggregator(lineLimit, false, false, status.NewInfoRegistry())
		// Leader: startGroup, no flag.
		ag.Process(newMessage(leader), startGroup, nil)
		// Continuation: aggregate, UPSTREAM-FLAGGED.
		ag.Process(msgWithFlag(flagged, true), aggregate, nil)
		// Trigger emission via boundary.
		emitted := captureCombiningEmissions(ag.Process(newMessage("trigger"), startGroup, nil))

		if len(emitted) != 1 {
			t.Fatalf("expected 1 combined emission, got %d", len(emitted))
		}
		e := emitted[0]
		marker := message.TruncatedFlag
		if bytes.Contains(e.content, marker) {
			t.Fatalf("BucketFlushIgnoresUpstreamFlag violated: combined emission contains marker — upstream flag on contributor was honoured; content %q", e.content)
		}
		if e.isTruncated {
			t.Fatalf("BucketFlushIgnoresUpstreamFlag violated: combined emission IsTruncated=true — upstream flag propagated; content %q", e.content)
		}
		if !e.isMultiLine {
			t.Fatal("BucketFlushIgnoresUpstreamFlag precondition: emission should be multi-line (lineCount > 1)")
		}
	})
}

// TestCombiningAggregator_ExplosionPathHonorsUpstream_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee EmitSingleHonorsUpstreamFlag — the
//	                                               bucket.emitSingle()
//	                                               path DOES honour
//	                                               upstream
//	                                               IsTruncated. It
//	                                               is invoked only
//	                                               from the
//	                                               explosion path.
//
// When the wouldOverflowBucket guard fires, the bucket is
// exploded into individual emissions via bucket.emitSingle. The
// current aggregate line (the one that triggered overflow) is
// then emitted via bucket.emitSingle as well. emit_single
// honours upstream IsTruncated, so a flagged current line
// emits with the tail marker even when content is under
// line_limit on its own.
func TestCombiningAggregator_ExplosionPathHonorsUpstream_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(15, 40).Draw(t, "lineLimit")
		// Build up a bucket close to overflow capacity, then
		// trigger overflow with a flagged line that's under
		// threshold on its own.
		leader := strings.Repeat("l", lineLimit/3)
		cont := strings.Repeat("c", lineLimit/3)
		flaggedLen := rapid.IntRange(1, lineLimit/4).Draw(t, "flaggedLen")
		flagged := strings.Repeat("f", flaggedLen)

		ag := NewCombiningAggregator(lineLimit, false, false, status.NewInfoRegistry())
		ag.Process(newMessage(leader), startGroup, nil)
		ag.Process(newMessage(cont), aggregate, nil)
		// Bucket now ~ 2/3 lineLimit. Next aggregate pushes over.
		emitted := captureCombiningEmissions(ag.Process(msgWithFlag(flagged, true), aggregate, nil))

		// Explosion: emits 2 buffered lines + the current line.
		// Skip if we didn't actually hit explosion (e.g. the
		// projected size didn't cross lineLimit due to rapid's
		// chosen sizes).
		if len(emitted) < 3 {
			return
		}
		marker := message.TruncatedFlag
		// The LAST emission is the current msg (flagged, under
		// threshold by itself). It should have tail marker due
		// to upstream flag propagating through emitSingle.
		last := emitted[len(emitted)-1]
		if !bytes.HasSuffix(last.content, marker) {
			t.Fatalf("ExplosionPathHonorsUpstream violated: no tail marker on flagged current line emission %q", last.content)
		}
		if !last.isTruncated {
			t.Fatal("ExplosionPathHonorsUpstream violated: IsTruncated=false on flagged emission")
		}
	})
}

// TestCombiningAggregator_TruncationTaggingSingleLine_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee TruncationTagging — single-line truncated
//	                                    emissions receive the
//	                                    "single_line" reason tag
//	                                    when tag_truncated_logs is
//	                                    true.
//
// With per-aggregator tag_truncated_logs=true at construction,
// an oversized startGroup emits with the "single_line"
// truncation-reason tag (the bucket.flush path with lineCount=1
// hardcodes truncatedReason="single_line").
func TestCombiningAggregator_TruncationTaggingSingleLine_Property(t *testing.T) {
	expectedTag := message.TruncatedReasonTag("single_line")
	autoMultilineTag := message.TruncatedReasonTag("auto_multiline")

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(5, 50).Draw(t, "lineLimit")
		oversized := strings.Repeat("x", lineLimit+rapid.IntRange(1, 30).Draw(t, "extra"))

		ag := NewCombiningAggregator(lineLimit, true, false, status.NewInfoRegistry())
		emitted := captureCombiningEmissions(ag.Process(newMessage(oversized), startGroup, nil))

		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission for oversized startGroup, got %d", len(emitted))
		}
		e := emitted[0]
		hasSingleLine := false
		for _, tag := range e.tags {
			if tag == expectedTag {
				hasSingleLine = true
			}
			if tag == autoMultilineTag {
				t.Fatalf("TruncationTaggingSingleLine violated: single-line emission has %q tag (should be %q); tags=%v", autoMultilineTag, expectedTag, e.tags)
			}
		}
		if !hasSingleLine {
			t.Fatalf("TruncationTaggingSingleLine violated: single-line truncated emission missing %q tag; tags=%v", expectedTag, e.tags)
		}
	})
}

// TestCombiningAggregator_TruncationTaggingAutoMultiline_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee TruncationTagging — combined truncated
//	                                    emissions receive the
//	                                    "auto_multiline" reason tag
//	                                    when tag_truncated_logs is
//	                                    true.
//
// A combined emission (lineCount > 1) carrying truncation
// (via carry from a prior oversized emission) receives the
// "auto_multiline" reason tag — distinct from the "single_line"
// reason used for lineCount=1 emissions.
func TestCombiningAggregator_TruncationTaggingAutoMultiline_Property(t *testing.T) {
	expectedTag := message.TruncatedReasonTag("auto_multiline")
	singleLineTag := message.TruncatedReasonTag("single_line")

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(30, 80).Draw(t, "lineLimit")
		oversized := strings.Repeat("x", lineLimit+rapid.IntRange(1, 30).Draw(t, "extra"))
		// Two short lines after carry: combined emission gets
		// the head marker from carry, and lineCount=2 triggers
		// the auto_multiline tag reason.
		short1 := strings.Repeat("a", rapid.IntRange(1, 3).Draw(t, "s1Len"))
		short2 := strings.Repeat("b", rapid.IntRange(1, 3).Draw(t, "s2Len"))

		ag := NewCombiningAggregator(lineLimit, true, false, status.NewInfoRegistry())
		// Oversized startGroup: emits single-line truncated,
		// carry set.
		ag.Process(newMessage(oversized), startGroup, nil)
		// startGroup with short content: flushes nothing
		// (prior bucket already empty after the oversized
		// flush). Actually wait — bucket is empty after the
		// oversized flush. Hmm.
		// Use noAggregate to reset carry. NO — we WANT the
		// carry. Use aggregate-on-empty instead.
		ag.Process(newMessage(short1), aggregate, nil) // emits with head marker, lineCount=1
		// Hmm that's already emitted. Let me restructure.
		// To get a multi-line combined with carry, we need:
		//   1. Carry set
		//   2. Bucket buffers 2+ lines without overflow
		//   3. Boundary flushes the combined message

		// Reset and try again with the correct sequence.
		ag = NewCombiningAggregator(lineLimit, true, false, status.NewInfoRegistry())
		ag.Process(newMessage(oversized), startGroup, nil)                                 // emit + carry
		ag.Process(newMessage("S"), startGroup, nil)                                       // new group leader (no emit, carry still set)
		ag.Process(newMessage(short1), aggregate, nil)                                     // accumulate
		ag.Process(newMessage(short2), aggregate, nil)                                     // accumulate (2-line bucket)
		emitted := captureCombiningEmissions(ag.Process(newMessage("T"), startGroup, nil)) // flush combined

		if len(emitted) != 1 {
			t.Fatalf("expected 1 combined emission, got %d", len(emitted))
		}
		e := emitted[0]
		if !e.isMultiLine {
			t.Fatal("TruncationTaggingAutoMultiline precondition: emission should be multi-line")
		}
		if !e.isTruncated {
			t.Fatal("TruncationTaggingAutoMultiline precondition: emission should be truncated (carry head marker)")
		}
		hasAutoMultiline := false
		for _, tag := range e.tags {
			if tag == expectedTag {
				hasAutoMultiline = true
			}
			if tag == singleLineTag {
				t.Fatalf("TruncationTaggingAutoMultiline violated: combined truncated emission has %q tag (should be %q); tags=%v", singleLineTag, expectedTag, e.tags)
			}
		}
		if !hasAutoMultiline {
			t.Fatalf("TruncationTaggingAutoMultiline violated: combined truncated emission missing %q tag; tags=%v", expectedTag, e.tags)
		}
	})
}

// TestCombiningAggregator_StartGroupBoundary_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee StartGroupBoundary — a process call with the
//	                                     start_group label flushes
//	                                     any buffered bucket as a
//	                                     combined emission and
//	                                     then begins a new bucket
//	                                     containing the current
//	                                     message.
//
// After accumulating startGroup + aggregate(s) into a bucket, a
// subsequent startGroup call emits the prior bucket as ONE
// combined emission and starts a fresh accumulation with the
// new startGroup line.
func TestCombiningAggregator_StartGroupBoundary_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		nContinuations := rapid.IntRange(0, 3).Draw(t, "nContinuations")

		ag := NewCombiningAggregator(200, false, false, status.NewInfoRegistry())
		ag.Process(newMessage("S1 leader"), startGroup, nil)
		for i := 0; i < nContinuations; i++ {
			ag.Process(newMessage("cont"), aggregate, nil)
		}
		// New startGroup should flush prior bucket.
		emitted := captureCombiningEmissions(ag.Process(newMessage("S2 leader"), startGroup, nil))

		if len(emitted) != 1 {
			t.Fatalf("StartGroupBoundary violated: expected exactly 1 emission (the prior bucket flushed), got %d", len(emitted))
		}
		// The emission should contain the leader's content.
		if !bytes.Contains(emitted[0].content, []byte("S1 leader")) {
			t.Fatalf("StartGroupBoundary violated: emission missing leader content; got %q", emitted[0].content)
		}
		// If continuations were added, emission should be multi-line.
		if nContinuations > 0 && !emitted[0].isMultiLine {
			t.Fatalf("StartGroupBoundary violated: %d-continuation emission not multi-line", nContinuations)
		}
	})
}

// TestCombiningAggregator_NoAggregateFlushes_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee NoAggregateFlushes — a process call with the
//	                                     no_aggregate label flushes
//	                                     any buffered bucket and
//	                                     emits the current line as
//	                                     a single-line message
//	                                     (producing 1 or 2
//	                                     emissions).
//
// After accumulating startGroup + aggregate(s) into a bucket, a
// noAggregate call produces 2 emissions: the prior bucket (as a
// combined message if 2+ lines, single-line if 1) and the
// current noAggregate message as a single-line. With an empty
// prior bucket, noAggregate produces just 1 emission.
func TestCombiningAggregator_NoAggregateFlushes_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		nContinuations := rapid.IntRange(1, 3).Draw(t, "nContinuations")

		ag := NewCombiningAggregator(200, false, false, status.NewInfoRegistry())
		ag.Process(newMessage("S leader"), startGroup, nil)
		for i := 0; i < nContinuations; i++ {
			ag.Process(newMessage("cont"), aggregate, nil)
		}
		// noAggregate should flush prior bucket + emit current.
		emitted := captureCombiningEmissions(ag.Process(newMessage("naX"), noAggregate, nil))

		if len(emitted) != 2 {
			t.Fatalf("NoAggregateFlushes violated: expected 2 emissions (prior bucket + current), got %d", len(emitted))
		}
		// Emission 1: the prior combined bucket.
		if !bytes.Contains(emitted[0].content, []byte("S leader")) {
			t.Fatalf("NoAggregateFlushes violated: emission 1 missing leader content; got %q", emitted[0].content)
		}
		if !emitted[0].isMultiLine {
			t.Fatalf("NoAggregateFlushes violated: emission 1 with %d-line prior bucket should be multi-line", 1+nContinuations)
		}
		// Emission 2: the current noAggregate as single-line.
		if !bytes.Contains(emitted[1].content, []byte("naX")) {
			t.Fatalf("NoAggregateFlushes violated: emission 2 missing current message; got %q", emitted[1].content)
		}
		if emitted[1].isMultiLine {
			t.Fatalf("NoAggregateFlushes violated: emission 2 (current noAggregate) should be single-line, got multi-line; content %q", emitted[1].content)
		}
	})
}

// TestCombiningAggregator_TokensFromAggregateLeader_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee TokensFromAggregateLeader — a combined
//	                                            emission's tokens
//	                                            are the tokens
//	                                            passed alongside
//	                                            the FIRST line of
//	                                            the bucket (the
//	                                            start_group line
//	                                            that initiated the
//	                                            group).
//
// Build a multi-line group with leader-tokens vs. continuation-
// tokens. The boundary-flushed combined emission carries the
// leader's tokens, not the continuation's.
func TestCombiningAggregator_TokensFromAggregateLeader_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		leaderTokens := []Token{Token(1), Token(2), Token(3)}
		contTokens := []Token{Token(7), Token(8)}

		ag := NewCombiningAggregator(200, false, false, status.NewInfoRegistry())
		ag.Process(newMessage("S leader"), startGroup, leaderTokens)
		ag.Process(newMessage("cont"), aggregate, contTokens)
		// Boundary triggers emission of the prior group.
		emitted := ag.Process(newMessage("S next"), startGroup, []Token{Token(0)})

		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission, got %d", len(emitted))
		}
		gotTokens := emitted[0].Tokens
		if len(gotTokens) != len(leaderTokens) {
			t.Fatalf("TokensFromAggregateLeader violated: emission has %d tokens, leader had %d", len(gotTokens), len(leaderTokens))
		}
		for i := range leaderTokens {
			if gotTokens[i] != leaderTokens[i] {
				t.Fatalf("TokensFromAggregateLeader violated: emission token[%d]=%v, expected leader token[%d]=%v (continuation had %v)", i, gotTokens[i], i, leaderTokens[i], contTokens)
			}
		}
	})
}

// TestCombiningAggregator_MultiLineSourceTag_Property anchors:
//
//	surface CombiningAggregation (combining_aggregator.allium)
//	    @guarantee MultiLineTagging — combined emissions of 2+
//	                                   lines set is_multi_line=true
//	                                   AND, when tag_multi_line_logs
//	                                   is true, append the multi-line
//	                                   source tag with value
//	                                   "auto_multiline".
//
// A 2+ line combined emission with tag_multi_line_logs=true at
// construction carries the multi-line source tag.
func TestCombiningAggregator_MultiLineSourceTag_Property(t *testing.T) {
	multiLineTag := message.MultiLineSourceTag("auto_multiline")

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := 200
		nContinuations := rapid.IntRange(1, 4).Draw(t, "nContinuations")

		ag := NewCombiningAggregator(lineLimit, false, true, status.NewInfoRegistry())
		ag.Process(newMessage("leader"), startGroup, nil)
		for i := 0; i < nContinuations; i++ {
			ag.Process(newMessage("more"), aggregate, nil)
		}
		emitted := captureCombiningEmissions(ag.Process(newMessage("trigger"), startGroup, nil))

		if len(emitted) != 1 {
			t.Fatalf("expected 1 combined emission, got %d", len(emitted))
		}
		e := emitted[0]
		if !e.isMultiLine {
			t.Fatal("MultiLineSourceTag precondition: emission should be multi-line")
		}
		hasTag := false
		for _, tag := range e.tags {
			if tag == multiLineTag {
				hasTag = true
				break
			}
		}
		if !hasTag {
			t.Fatalf("MultiLineSourceTag violated: %d-line combined emission missing tag %q; tags=%v", 1+nContinuations, multiLineTag, e.tags)
		}
	})
}
