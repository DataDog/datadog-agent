// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Property tests for the SingleLineHandling surface declared in
// single_line_handler.allium. Each test names the spec @guarantee
// it anchors so drift in either direction is easy to spot during
// review.
//
// Anchoring unit tests for the same surface live in
// single_line_handler_test.go.
//
// # Input distributions of interest
//
// The generators in this file are shaped to hit the following
// scenarios so that random rapid generation cannot under-cover
// the divergence cases.
//
//	(a) under-threshold inputs with carry=false → no markers,
//	    flag false (PassThrough).
//	(b) over-threshold inputs with carry=false → tail marker,
//	    flag true, next carry=true.
//	(c) under-threshold inputs with carry=true → head marker,
//	    flag true, next carry=false (HeadMarkerOnCarryover +
//	    CarryoverConsumed).
//	(d) over-threshold inputs with carry=true → both markers,
//	    flag true, next carry=true.
//	(e) upstream-truncated inputs (input flag true) under
//	    threshold with carry=false → tail marker, flag true,
//	    next carry=true (UpstreamFlagPropagation).
//	(f) sequences alternating truncated and untruncated inputs
//	    — verifies carry transition logic across calls.
//	(g) inputs with leading/trailing whitespace — verifies
//	    WhitespaceTrimmedBeforeMarkers.

// singleLineInput bundles a single randomly-generated process call.
type singleLineInput struct {
	content             []byte
	upstreamIsTruncated bool
}

// singleLineEmission captures the observable result of one process
// call. We copy the fields out of the *message.Message at the
// callback boundary because the handler mutates the input message
// in place — see @guarantee InputMessageMutated.
type singleLineEmission struct {
	content     []byte
	isTruncated bool
	tags        []string
}

// genSingleLineInput produces a single call's worth of input.
// Content is drawn from a printable alphabet (letters, digits,
// space, tab) so that whitespace-trim effects are exercised. The
// length range spans both sides of any reasonable line_limit so
// generators that draw a sequence of calls hit the "fits" and
// "oversize" branches of @guidance step 1.
func genSingleLineInput() *rapid.Generator[singleLineInput] {
	return rapid.Custom(func(t *rapid.T) singleLineInput {
		raw := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef 0123\t")),
			0, 60,
		).Draw(t, "raw")
		isTruncated := rapid.Bool().Draw(t, "isTruncated")
		return singleLineInput{
			content:             raw,
			upstreamIsTruncated: isTruncated,
		}
	})
}

// runHandler builds a SingleLineHandler with the given line_limit
// and drives it through the input sequence. Returns the sequence
// of emissions (each emission's content and tags are deep-copied
// so subsequent process calls cannot perturb them).
func runHandler(lineLimit int, inputs []singleLineInput) []singleLineEmission {
	var emitted []singleLineEmission
	h := NewSingleLineHandler(func(m *message.Message) {
		emitted = append(emitted, singleLineEmission{
			content:     append([]byte(nil), m.GetContent()...),
			isTruncated: m.ParsingExtra.IsTruncated,
			tags:        append([]string(nil), m.ParsingExtra.Tags...),
		})
	}, lineLimit)
	for _, in := range inputs {
		msg := message.NewMessage(append([]byte(nil), in.content...), nil, "", time.Now().UnixNano())
		msg.RawDataLen = len(in.content)
		msg.ParsingExtra.IsTruncated = in.upstreamIsTruncated
		h.process(msg)
	}
	return emitted
}

// TestSingleLineHandler_PassThroughUnderThreshold_Property anchors:
//
//	surface SingleLineHandling (single_line_handler.allium)
//	    @guarantee PassThroughUnderThreshold — under-threshold +
//	                                            no carry +
//	                                            no upstream flag
//	                                            ⇒ no markers,
//	                                            IsTruncated false.
//
// A single process call on a fresh handler whose content is
// strictly shorter than line_limit and whose ParsingExtra.IsTruncated
// is false emits whitespace-trimmed content with no truncation
// markers and IsTruncated=false.
func TestSingleLineHandler_PassThroughUnderThreshold_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(50, 200).Draw(t, "lineLimit")
		// Bound generated length to half the limit so we stay
		// strictly under-threshold.
		raw := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef 0123\t")),
			0, lineLimit/2,
		).Draw(t, "raw")

		emitted := runHandler(lineLimit, []singleLineInput{{content: raw, upstreamIsTruncated: false}})
		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission, got %d", len(emitted))
		}
		e := emitted[0]

		marker := message.TruncatedFlag
		if bytes.HasPrefix(e.content, marker) {
			t.Fatalf("PassThroughUnderThreshold violated: head marker present in %q", e.content)
		}
		if bytes.HasSuffix(e.content, marker) {
			t.Fatalf("PassThroughUnderThreshold violated: tail marker present in %q", e.content)
		}
		if e.isTruncated {
			t.Fatal("PassThroughUnderThreshold violated: IsTruncated=true on under-threshold non-upstream input")
		}
		expected := bytes.TrimSpace(raw)
		if !bytes.Equal(e.content, expected) {
			t.Fatalf("PassThroughUnderThreshold violated: content %q != trimmed input %q", e.content, expected)
		}
	})
}

// TestSingleLineHandler_TailMarkerOnOverThreshold_Property anchors:
//
//	surface SingleLineHandling (single_line_handler.allium)
//	    @guarantee TailMarkerOnOverThreshold — content over
//	                                            threshold ⇒ tail
//	                                            marker, IsTruncated
//	                                            true.
//
// A single process call on a fresh handler whose content has byte
// length strictly greater than line_limit emits content ending
// with the truncation marker and IsTruncated=true.
func TestSingleLineHandler_TailMarkerOnOverThreshold_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(5, 100).Draw(t, "lineLimit")
		// Generate content longer than the limit.
		extra := rapid.IntRange(1, 60).Draw(t, "extra")
		raw := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef0123")),
			lineLimit+extra, lineLimit+extra,
		).Draw(t, "raw")

		emitted := runHandler(lineLimit, []singleLineInput{{content: raw, upstreamIsTruncated: false}})
		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission, got %d", len(emitted))
		}
		e := emitted[0]

		marker := message.TruncatedFlag
		if !bytes.HasSuffix(e.content, marker) {
			t.Fatalf("TailMarkerOnOverThreshold violated: no tail marker in %q", e.content)
		}
		if bytes.HasPrefix(e.content, marker) {
			t.Fatalf("TailMarkerOnOverThreshold violated: unexpected head marker on fresh handler in %q", e.content)
		}
		if !e.isTruncated {
			t.Fatal("TailMarkerOnOverThreshold violated: IsTruncated=false on over-threshold input")
		}
	})
}

// TestSingleLineHandler_HeadMarkerOnCarryover_Property anchors:
//
//	surface SingleLineHandling (single_line_handler.allium)
//	    @guarantee HeadMarkerOnCarryover — prior carry=true ⇒
//	                                        head marker on next
//	                                        emission, IsTruncated
//	                                        true.
//
// After a process call that truncates (sets carry=true), the
// NEXT process call's emission begins with the truncation marker
// and IsTruncated=true, even when its own content is under
// threshold and its upstream flag is false.
func TestSingleLineHandler_HeadMarkerOnCarryover_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(5, 100).Draw(t, "lineLimit")

		// First input: over threshold to set the carry.
		extra := rapid.IntRange(1, 60).Draw(t, "extra")
		first := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef0123")),
			lineLimit+extra, lineLimit+extra,
		).Draw(t, "first")

		// Second input: under threshold, no upstream flag.
		second := rapid.SliceOfN(
			rapid.SampledFrom([]byte("xyzwv98765")),
			1, lineLimit/2+1,
		).Draw(t, "second")

		emitted := runHandler(lineLimit, []singleLineInput{
			{content: first, upstreamIsTruncated: false},
			{content: second, upstreamIsTruncated: false},
		})
		if len(emitted) != 2 {
			t.Fatalf("expected 2 emissions, got %d", len(emitted))
		}
		e := emitted[1]

		marker := message.TruncatedFlag
		if !bytes.HasPrefix(e.content, marker) {
			t.Fatalf("HeadMarkerOnCarryover violated: no head marker on second emission %q", e.content)
		}
		if bytes.HasSuffix(e.content, marker) {
			t.Fatalf("HeadMarkerOnCarryover violated: unexpected tail marker on under-threshold second emission %q", e.content)
		}
		if !e.isTruncated {
			t.Fatal("HeadMarkerOnCarryover violated: IsTruncated=false on emission with head marker")
		}
	})
}

// TestSingleLineHandler_CarryoverConsumed_Property anchors:
//
//	surface SingleLineHandling (single_line_handler.allium)
//	    @guarantee CarryoverConsumed — carry is determined solely
//	                                    by current input; prior
//	                                    carry does not propagate.
//
// After a process call that sets carry (via over-threshold input),
// a next call that is under threshold and not upstream-flagged
// consumes the carry (emitting with head marker) and the THIRD
// call must NOT carry the marker — verifying the carry reset on
// emission 2.
func TestSingleLineHandler_CarryoverConsumed_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(5, 100).Draw(t, "lineLimit")

		// First input: over threshold to set carry.
		extra := rapid.IntRange(1, 60).Draw(t, "extra")
		first := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef0123")),
			lineLimit+extra, lineLimit+extra,
		).Draw(t, "first")

		// Second + third: under threshold, no upstream flag.
		second := rapid.SliceOfN(
			rapid.SampledFrom([]byte("xyzwv98765")),
			1, lineLimit/2+1,
		).Draw(t, "second")
		third := rapid.SliceOfN(
			rapid.SampledFrom([]byte("pqrstu43210")),
			1, lineLimit/2+1,
		).Draw(t, "third")

		emitted := runHandler(lineLimit, []singleLineInput{
			{content: first, upstreamIsTruncated: false},
			{content: second, upstreamIsTruncated: false},
			{content: third, upstreamIsTruncated: false},
		})
		if len(emitted) != 3 {
			t.Fatalf("expected 3 emissions, got %d", len(emitted))
		}

		marker := message.TruncatedFlag

		// Emission 2 should carry the head marker (HeadMarkerOnCarryover).
		if !bytes.HasPrefix(emitted[1].content, marker) {
			t.Fatalf("CarryoverConsumed precondition: emission 2 should have head marker; got %q", emitted[1].content)
		}

		// The point of this test: emission 3 must NOT carry a
		// head marker — the carry was consumed by emission 2.
		e := emitted[2]
		if bytes.HasPrefix(e.content, marker) {
			t.Fatalf("CarryoverConsumed violated: emission 3 carries head marker after emission 2 should have consumed it; content %q", e.content)
		}
		if bytes.HasSuffix(e.content, marker) {
			t.Fatalf("CarryoverConsumed violated: emission 3 has tail marker on under-threshold input; content %q", e.content)
		}
		if e.isTruncated {
			t.Fatal("CarryoverConsumed violated: emission 3 IsTruncated=true on clean under-threshold input")
		}
	})
}

// TestSingleLineHandler_UpstreamFlagPropagation_Property anchors:
//
//	surface SingleLineHandling (single_line_handler.allium)
//	    @guarantee UpstreamFlagPropagation — upstream IsTruncated
//	                                          true behaves like
//	                                          over-threshold:
//	                                          tail marker, carry
//	                                          set, IsTruncated
//	                                          true.
//
// On a fresh handler, an input whose ParsingExtra.IsTruncated=true
// AND whose content is under threshold emits with the tail marker
// and IsTruncated=true. The next emission (under-threshold, no
// flag) carries the head marker (carry was set by emission 1).
func TestSingleLineHandler_UpstreamFlagPropagation_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(10, 100).Draw(t, "lineLimit")
		// Under threshold; flagged upstream.
		raw := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef0123")),
			1, lineLimit/2,
		).Draw(t, "raw")
		// Second: under threshold, unflagged.
		raw2 := rapid.SliceOfN(
			rapid.SampledFrom([]byte("uvwxy67890")),
			1, lineLimit/2,
		).Draw(t, "raw2")

		emitted := runHandler(lineLimit, []singleLineInput{
			{content: raw, upstreamIsTruncated: true},
			{content: raw2, upstreamIsTruncated: false},
		})
		if len(emitted) != 2 {
			t.Fatalf("expected 2 emissions, got %d", len(emitted))
		}

		marker := message.TruncatedFlag

		// Emission 1: under-threshold but upstream-flagged → tail
		// marker, IsTruncated=true.
		if !bytes.HasSuffix(emitted[0].content, marker) {
			t.Fatalf("UpstreamFlagPropagation violated: no tail marker on emission 1 (upstream flag) %q", emitted[0].content)
		}
		if !emitted[0].isTruncated {
			t.Fatal("UpstreamFlagPropagation violated: emission 1 IsTruncated=false despite upstream flag")
		}

		// Emission 2: under-threshold, unflagged, but carry was
		// set by emission 1 → head marker.
		if !bytes.HasPrefix(emitted[1].content, marker) {
			t.Fatalf("UpstreamFlagPropagation violated: emission 2 missing head marker after upstream-flagged carry; content %q", emitted[1].content)
		}
	})
}

// TestSingleLineHandler_ByteConservation_Property anchors:
//
//	surface SingleLineHandling (single_line_handler.allium)
//	    @guarantee MarkerLiteral — markers are exactly the literal
//	                                bytes message.TruncatedFlag.
//	    @guarantee WhitespaceTrimmedBeforeMarkers — emitted content
//	                                                with markers
//	                                                stripped equals
//	                                                trimmed input.
//
// For arbitrary inputs and an arbitrary line_limit, the emitted
// content stripped of one prepended and one appended marker
// equals the whitespace-trimmed input. This is the strong byte-
// accounting form: every byte of the output is either a
// non-trimmable input byte or part of one of the two marker
// boundary regions.
func TestSingleLineHandler_ByteConservation_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(1, 200).Draw(t, "lineLimit")
		in := genSingleLineInput().Draw(t, "input")

		emitted := runHandler(lineLimit, []singleLineInput{in})
		if len(emitted) != 1 {
			t.Fatalf("expected 1 emission, got %d", len(emitted))
		}
		out := string(emitted[0].content)
		marker := string(message.TruncatedFlag)

		stripped := out
		stripped = strings.TrimPrefix(stripped, marker)
		stripped = strings.TrimSuffix(stripped, marker)

		expected := strings.TrimSpace(string(in.content))
		if stripped != expected {
			t.Fatalf("ByteConservation/MarkerLiteral/WhitespaceTrimmedBeforeMarkers violated: stripped %q != trimmed input %q (full output %q)",
				stripped, expected, out)
		}
	})
}

// TestSingleLineHandler_PerMessageEmission_Property anchors:
//
//	surface SingleLineHandling (single_line_handler.allium)
//	    @guarantee PerMessageEmission — one input ⇒ exactly one
//	                                     outputFn call.
//
// Across an arbitrary sequence of process calls, the handler
// invokes outputFn exactly once per call. No buffering, no
// dropping, no duplication.
func TestSingleLineHandler_PerMessageEmission_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(1, 200).Draw(t, "lineLimit")
		calls := rapid.SliceOfN(genSingleLineInput(), 1, 10).Draw(t, "calls")
		emitted := runHandler(lineLimit, calls)
		if len(emitted) != len(calls) {
			t.Fatalf("PerMessageEmission violated: %d inputs produced %d emissions", len(calls), len(emitted))
		}
	})
}

// TestSingleLineHandler_StatelessFlush_Property anchors:
//
//	surface SingleLineHandling (single_line_handler.allium)
//	    @guarantee StatelessFlush — flush is a no-op; no buffered
//	                                 emission is produced and no
//	                                 observable state changes.
//
// After an arbitrary sequence of process calls, invoking flush
// emits nothing (no additional outputFn calls), and a subsequent
// process call behaves identically to one without the flush in
// between.
func TestSingleLineHandler_StatelessFlush_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(1, 200).Draw(t, "lineLimit")
		calls := rapid.SliceOfN(genSingleLineInput(), 1, 6).Draw(t, "calls")
		probe := genSingleLineInput().Draw(t, "probe")

		// Run A: calls, then probe (no flush in between).
		emittedA := runHandler(lineLimit, append(append([]singleLineInput(nil), calls...), probe))

		// Run B: calls, flush(), then probe.
		var emittedB []singleLineEmission
		h := NewSingleLineHandler(func(m *message.Message) {
			emittedB = append(emittedB, singleLineEmission{
				content:     append([]byte(nil), m.GetContent()...),
				isTruncated: m.ParsingExtra.IsTruncated,
				tags:        append([]string(nil), m.ParsingExtra.Tags...),
			})
		}, lineLimit)
		for _, in := range calls {
			msg := message.NewMessage(append([]byte(nil), in.content...), nil, "", time.Now().UnixNano())
			msg.RawDataLen = len(in.content)
			msg.ParsingExtra.IsTruncated = in.upstreamIsTruncated
			h.process(msg)
		}
		preFlushCount := len(emittedB)
		h.flush()
		if len(emittedB) != preFlushCount {
			t.Fatalf("StatelessFlush violated: flush emitted %d additional messages", len(emittedB)-preFlushCount)
		}
		// Now drive the probe.
		probeMsg := message.NewMessage(append([]byte(nil), probe.content...), nil, "", time.Now().UnixNano())
		probeMsg.RawDataLen = len(probe.content)
		probeMsg.ParsingExtra.IsTruncated = probe.upstreamIsTruncated
		h.process(probeMsg)

		// Runs A and B must produce identical emission sequences:
		// the flush in B is a no-op and does not perturb subsequent
		// state (including the should_truncate carry).
		if len(emittedA) != len(emittedB) {
			t.Fatalf("StatelessFlush violated: emission count differs A=%d B=%d", len(emittedA), len(emittedB))
		}
		for i := range emittedA {
			if !bytes.Equal(emittedA[i].content, emittedB[i].content) {
				t.Fatalf("StatelessFlush violated: emission %d content differs A=%q B=%q", i, emittedA[i].content, emittedB[i].content)
			}
			if emittedA[i].isTruncated != emittedB[i].isTruncated {
				t.Fatalf("StatelessFlush violated: emission %d IsTruncated differs A=%v B=%v", i, emittedA[i].isTruncated, emittedB[i].isTruncated)
			}
		}
	})
}

// TestSingleLineHandler_TagOnTruncation_Property anchors:
//
//	surface SingleLineHandling (single_line_handler.allium)
//	    @guarantee TagOnTruncation — when global
//	                                  logs_config.tag_truncated_logs
//	                                  is true, a truncation-reason
//	                                  tag is added iff IsTruncated
//	                                  is set on the emission.
//	    @guarantee TagReasonSingleLine — tag value is "single_line".
//
// With the global config enabled, every emission whose IsTruncated
// is true carries exactly one ParsingExtra.Tags entry equal to
// message.TruncatedReasonTag("single_line"). Emissions with
// IsTruncated=false carry no tag.
func TestSingleLineHandler_TagOnTruncation_Property(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("logs_config.tag_truncated_logs", true)
	expectedTag := message.TruncatedReasonTag("single_line")

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(1, 200).Draw(t, "lineLimit")
		calls := rapid.SliceOfN(genSingleLineInput(), 1, 10).Draw(t, "calls")
		emitted := runHandler(lineLimit, calls)

		for i, e := range emitted {
			if e.isTruncated {
				if len(e.tags) != 1 || e.tags[0] != expectedTag {
					t.Fatalf("TagOnTruncation/TagReasonSingleLine violated: emission %d IsTruncated=true but tags=%v (want [%q])", i, e.tags, expectedTag)
				}
			} else {
				if len(e.tags) != 0 {
					t.Fatalf("TagOnTruncation violated: emission %d IsTruncated=false but tags=%v (want none)", i, e.tags)
				}
			}
		}
	})
}

// TestSingleLineHandler_InputMessageMutated_Property anchors:
//
//	surface SingleLineHandling (single_line_handler.allium)
//	    @guarantee InputMessageMutated — the input *message.Message
//	                                      is mutated in place
//	                                      before being passed to
//	                                      outputFn; the handler
//	                                      does not construct a new
//	                                      message.
//
// LOAD-BEARING for the refactor safety net. If the refactor
// switches from in-place mutation to allocation of new emission
// messages, observable identity behaviour changes — any caller
// retaining a reference to the input message would no longer see
// the post-process modifications. This test verifies the pointer
// passed to outputFn is the SAME as the input pointer.
func TestSingleLineHandler_InputMessageMutated_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(1, 200).Draw(t, "lineLimit")
		in := genSingleLineInput().Draw(t, "input")

		var inputPtr *message.Message
		var emittedPtr *message.Message
		h := NewSingleLineHandler(func(m *message.Message) {
			emittedPtr = m
		}, lineLimit)

		msg := message.NewMessage(append([]byte(nil), in.content...), nil, "", time.Now().UnixNano())
		msg.RawDataLen = len(in.content)
		msg.ParsingExtra.IsTruncated = in.upstreamIsTruncated
		inputPtr = msg
		h.process(msg)

		if emittedPtr != inputPtr {
			t.Fatalf("InputMessageMutated violated: emitted pointer %p differs from input pointer %p (handler allocated a new message instead of mutating in place)", emittedPtr, inputPtr)
		}
	})
}

// TestSingleLineHandler_TagDisabledNoTag_Property anchors:
//
//	surface SingleLineHandling (single_line_handler.allium)
//	    @guarantee TagOnTruncation — when global
//	                                  logs_config.tag_truncated_logs
//	                                  is false, NO truncation-reason
//	                                  tag is added regardless of
//	                                  IsTruncated.
//
// With the global config disabled, no emission carries a
// truncation-reason tag, even when markers are applied and
// IsTruncated is set.
func TestSingleLineHandler_TagDisabledNoTag_Property(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("logs_config.tag_truncated_logs", false)

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(1, 200).Draw(t, "lineLimit")
		calls := rapid.SliceOfN(genSingleLineInput(), 1, 10).Draw(t, "calls")
		emitted := runHandler(lineLimit, calls)

		for i, e := range emitted {
			if len(e.tags) != 0 {
				t.Fatalf("TagOnTruncation (disabled) violated: emission %d tags=%v (want none)", i, e.tags)
			}
		}
	})
}
