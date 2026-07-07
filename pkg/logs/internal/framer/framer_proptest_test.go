// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

import (
	"bytes"
	"testing"

	"pgregory.net/rapid"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Property tests for the FramerTruncation surface declared in
// framer.allium. Each test names the spec @guarantee it anchors so
// drift in either direction is easy to spot during review.
//
// Scope: tests use framings that are easy to construct synthetic
// inputs for — UTF8Newline (enforcing), NoFraming (passthrough),
// UTF8NewlineDatagram (enforcing variant with end-of-Process
// flush). docker_stream and syslog_framing have format-specific
// input requirements (Docker stream headers, RFC 6587 octet
// counting / non-transparent framing) tested elsewhere.
//
// # Input distributions of interest
//
// The generators in this file are shaped to hit the following
// scenarios so that random rapid generation cannot under-cover
// the divergence cases across framings.
//
//	(a) inputs under content_len_limit with valid frames
//	    (PassThroughUnderLimit for all framings).
//	(b) inputs with frames over content_len_limit where the
//	    matcher would normally find them (enforcing framings cut,
//	    non-enforcing framings passthrough).
//	(c) inputs without parseable frames where the buffer fills
//	    past content_len_limit (force-cut uniformly across
//	    framings).
//	(d) datagram-mode inputs combining the above (verify
//	    end-of-Process flush emits remainder with
//	    IsTruncated=false).
//	(e) end-of-stream Flush inputs that exercise the syslog
//	    flush-time truncation path (only syslog_framing has
//	    meaningful FlushFrame content; verify others emit
//	    nothing).

// framerEmission captures one emission's observable state.
type framerEmission struct {
	content     []byte
	rawDataLen  int
	isTruncated bool
}

// newFramerTestRig builds a Framer with a capturing outputFn.
// Returns the framer and a pointer to the captured emissions
// slice; emissions accumulate as Process / Flush are called.
func newFramerTestRig(framing Framing, lineLimit int) (*Framer, *[]framerEmission) {
	var emissions []framerEmission
	fr := NewFramer(
		func(m *message.Message, rawDataLen int) {
			emissions = append(emissions, framerEmission{
				content:     append([]byte(nil), m.GetContent()...),
				rawDataLen:  rawDataLen,
				isTruncated: m.ParsingExtra.IsTruncated,
			})
		},
		framing,
		lineLimit,
	)
	return fr, &emissions
}

func feedFramer(fr *Framer, content []byte) {
	msg := message.NewMessage(content, nil, "", 0)
	fr.Process(msg)
}

// TestFramer_EmissionFromMatcherFound_UTF8Newline_Property anchors:
//
//	surface FramerTruncation (framer.allium)
//	    @guarantee EmissionFromMatcherFound — when the matcher
//	                                           returns non-nil
//	                                           content with
//	                                           wasTruncated=false,
//	                                           the emission has
//	                                           IsTruncated=false.
//
// A newline-terminated line under content_len_limit emits with
// IsTruncated=false and content equal to the input bytes minus
// the terminator.
func TestFramer_EmissionFromMatcherFound_UTF8Newline_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(50, 200).Draw(t, "lineLimit")
		// Content under limit, no embedded newline, then a
		// trailing newline.
		body := rapid.SliceOfN(
			rapid.SampledFrom([]byte("abcdef0123")),
			0, lineLimit/2,
		).Draw(t, "body")
		input := append(append([]byte(nil), body...), '\n')

		fr, emissions := newFramerTestRig(UTF8Newline, lineLimit)
		feedFramer(fr, input)

		if len(*emissions) != 1 {
			t.Fatalf("expected 1 emission, got %d", len(*emissions))
		}
		e := (*emissions)[0]
		if e.isTruncated {
			t.Fatalf("EmissionFromMatcherFound violated: IsTruncated=true on under-threshold newline-terminated input %q", e.content)
		}
		if !bytes.Equal(e.content, body) {
			t.Fatalf("EmissionFromMatcherFound violated: emission content %q != input body %q", e.content, body)
		}
	})
}

// TestFramer_EmissionFromMatcherTruncated_UTF8Newline_Property anchors:
//
//	surface FramerTruncation (framer.allium)
//	    @guarantee EmissionFromMatcherTruncated — when the matcher
//	                                               returns
//	                                               wasTruncated=true,
//	                                               the emission has
//	                                               IsTruncated=true.
//	    @guarantee FramingCategorisation — utf8_newline is
//	                                        ENFORCING: matcher
//	                                        cuts at limit when
//	                                        terminator is found
//	                                        beyond.
//
// A line with a newline BEYOND content_len_limit causes the
// oneByteNewLineMatcher to return wasTruncated=true with content
// cut at content_len_limit bytes. The emission has
// IsTruncated=true.
func TestFramer_EmissionFromMatcherTruncated_UTF8Newline_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(10, 50).Draw(t, "lineLimit")
		// Total bytes before newline > lineLimit.
		preNewline := lineLimit + rapid.IntRange(1, 40).Draw(t, "extra")
		body := make([]byte, preNewline)
		for i := range body {
			body[i] = 'x'
		}
		input := append(body, '\n')

		fr, emissions := newFramerTestRig(UTF8Newline, lineLimit)
		feedFramer(fr, input)

		// The line was cut at lineLimit bytes; expect one
		// truncated emission of exactly lineLimit bytes.
		if len(*emissions) < 1 {
			t.Fatal("expected at least 1 emission, got 0")
		}
		first := (*emissions)[0]
		if !first.isTruncated {
			t.Fatalf("EmissionFromMatcherTruncated violated: IsTruncated=false on cut emission %q", first.content)
		}
		if len(first.content) != lineLimit {
			t.Fatalf("EmissionFromMatcherTruncated violated: emission len %d != lineLimit %d", len(first.content), lineLimit)
		}
	})
}

// TestFramer_ForceCutOnMatcherNil_UTF8Newline_Property anchors:
//
//	surface FramerTruncation (framer.allium)
//	    @guarantee ForceCutOnMatcherNil — when matcher returns nil
//	                                       AND buffer >=
//	                                       content_len_limit, the
//	                                       framer cuts the first
//	                                       content_len_limit bytes
//	                                       and emits with
//	                                       IsTruncated=true.
//
// Content without any newline that exceeds content_len_limit
// forces the matcher to return nil, triggering the framer's
// force-cut safety path. The emission has IsTruncated=true and
// is exactly content_len_limit bytes.
func TestFramer_ForceCutOnMatcherNil_UTF8Newline_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(10, 50).Draw(t, "lineLimit")
		excess := lineLimit + rapid.IntRange(1, 40).Draw(t, "excess")
		// No newline at all → matcher returns nil throughout.
		input := make([]byte, excess)
		for i := range input {
			input[i] = 'y'
		}

		fr, emissions := newFramerTestRig(UTF8Newline, lineLimit)
		feedFramer(fr, input)

		if len(*emissions) < 1 {
			t.Fatal("expected at least 1 emission from force-cut path, got 0")
		}
		first := (*emissions)[0]
		if !first.isTruncated {
			t.Fatalf("ForceCutOnMatcherNil violated: IsTruncated=false on force-cut emission %q", first.content)
		}
		if len(first.content) != lineLimit {
			t.Fatalf("ForceCutOnMatcherNil violated: emission len %d != lineLimit %d (force-cut should produce exactly lineLimit bytes)", len(first.content), lineLimit)
		}
	})
}

// TestFramer_NonEnforcingFramingPassthrough_NoFraming_Property anchors:
//
//	surface FramerTruncation (framer.allium)
//	    @guarantee FramingCategorisation — no_framing is
//	                                        NON-ENFORCING: matcher
//	                                        returns content with
//	                                        wasTruncated=false
//	                                        regardless of size.
//	    @guarantee EmissionFromMatcherFound — emission's
//	                                           IsTruncated mirrors
//	                                           matcher's
//	                                           wasTruncated.
//
// LOAD-BEARING for the refactor safety net. NoFraming with an
// oversized input MUST emit with IsTruncated=false. A refactor
// that began enforcing the limit for NoFraming would break this
// test — exactly the behaviour change we're guarding against.
func TestFramer_NonEnforcingFramingPassthrough_NoFraming_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(10, 50).Draw(t, "lineLimit")
		// Content larger than lineLimit.
		excess := lineLimit + rapid.IntRange(1, 80).Draw(t, "excess")
		input := make([]byte, excess)
		for i := range input {
			input[i] = 'z'
		}

		fr, emissions := newFramerTestRig(NoFraming, lineLimit)
		feedFramer(fr, input)

		if len(*emissions) != 1 {
			t.Fatalf("NoFraming should emit exactly 1 frame (the whole buffer), got %d", len(*emissions))
		}
		e := (*emissions)[0]
		if e.isTruncated {
			t.Fatalf("NonEnforcingFramingPassthrough violated: NoFraming emission has IsTruncated=true on oversized content; size=%d limit=%d", len(e.content), lineLimit)
		}
		if !bytes.Equal(e.content, input) {
			t.Fatalf("NonEnforcingFramingPassthrough violated: emission content len %d != input len %d (whole input should be emitted)", len(e.content), len(input))
		}
	})
}

// TestFramer_NoMarkerBytesAdded_Property anchors:
//
//	surface FramerTruncation (framer.allium)
//	    @guarantee NoMarkerBytesAdded — the framer never adds
//	                                     `...TRUNCATED...` marker
//	                                     bytes to any emitted
//	                                     frame's content,
//	                                     regardless of which
//	                                     emission path produced
//	                                     the frame.
//
// Across UTF8Newline, NoFraming, UTF8NewlineDatagram framings
// with arbitrary inputs, no emission contains the marker byte
// sequence. The framer's truncation signal is the IsTruncated
// flag only — never marker decoration.
func TestFramer_NoMarkerBytesAdded_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		framing := rapid.SampledFrom([]Framing{UTF8Newline, NoFraming, UTF8NewlineDatagram}).Draw(t, "framing")
		lineLimit := rapid.IntRange(10, 50).Draw(t, "lineLimit")
		// Alphabet excludes '.' so generated input cannot
		// naturally contain the marker substring.
		nChunks := rapid.IntRange(1, 5).Draw(t, "nChunks")

		fr, emissions := newFramerTestRig(framing, lineLimit)
		for i := 0; i < nChunks; i++ {
			chunk := rapid.SliceOfN(
				rapid.SampledFrom([]byte("abcdef0123\n")),
				0, 40,
			).Draw(t, "chunk")
			feedFramer(fr, chunk)
		}
		fr.Flush()

		marker := message.TruncatedFlag
		for i, e := range *emissions {
			if bytes.Contains(e.content, marker) {
				t.Fatalf("NoMarkerBytesAdded violated for framing=%v: emission %d contains marker bytes; content %q", framing, i, e.content)
			}
		}
	})
}

// TestFramer_NoCarryOverBetweenEmissions_Property anchors:
//
//	surface FramerTruncation (framer.allium)
//	    @guarantee NoCarryOverBetweenEmissions — the truncation
//	                                              flag on emission
//	                                              N+1 is
//	                                              independent of
//	                                              the flag on
//	                                              emission N. The
//	                                              framer holds no
//	                                              truncation-specific
//	                                              state across
//	                                              emissions.
//
// A force-cut emission (IsTruncated=true) followed by a clean
// newline-terminated emission produces a second emission with
// IsTruncated=false. The framer does NOT propagate a head
// marker / carry like Truncatable fulfillers do.
func TestFramer_NoCarryOverBetweenEmissions_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(10, 30).Draw(t, "lineLimit")
		// First chunk: oversized with NO newline → force-cut.
		excess := lineLimit + rapid.IntRange(1, 30).Draw(t, "excess")
		firstChunk := make([]byte, excess)
		for i := range firstChunk {
			firstChunk[i] = 'x'
		}
		// Cap with a newline to flush the residual through the
		// matcher's normal path, then a clean small line.
		secondChunkLen := rapid.IntRange(1, lineLimit/2).Draw(t, "shortLen")
		secondChunk := make([]byte, 0, secondChunkLen+2)
		secondChunk = append(secondChunk, '\n')
		small := make([]byte, secondChunkLen)
		for i := range small {
			small[i] = 'a'
		}
		secondChunk = append(secondChunk, small...)
		secondChunk = append(secondChunk, '\n')

		fr, emissions := newFramerTestRig(UTF8Newline, lineLimit)
		feedFramer(fr, firstChunk)
		feedFramer(fr, secondChunk)

		// Find the last emission whose content equals `small`.
		// That emission is the clean post-truncation line.
		var clean *framerEmission
		for i := range *emissions {
			if bytes.Equal((*emissions)[i].content, small) {
				clean = &(*emissions)[i]
			}
		}
		if clean == nil {
			t.Fatalf("precondition: no emission matched the clean line %q; emissions=%v", small, *emissions)
			return
		}
		if clean.isTruncated {
			t.Fatalf("NoCarryOverBetweenEmissions violated: clean line emission has IsTruncated=true (carry from prior truncation should not propagate); content %q", clean.content)
		}
	})
}

// TestFramer_IsTruncatedOnEmittedFrame_Property anchors:
//
//	surface FramerTruncation (framer.allium)
//	    @guarantee IsTruncatedOnEmittedFrame — the IsTruncated
//	                                            flag is set on the
//	                                            EMITTED frame's
//	                                            ParsingExtra (a new
//	                                            *message.Message
//	                                            constructed by
//	                                            emitFrame). The
//	                                            original input
//	                                            message's
//	                                            IsTruncated flag
//	                                            is not modified.
//
// LOAD-BEARING for the refactor safety net. If the refactor
// changes the framer to mutate the input message in place
// instead of constructing a new emission message, observable
// identity behaviour changes — any caller retaining the input
// would suddenly see truncation flags it didn't set. This test
// pins the current "new message per frame" semantics.
func TestFramer_IsTruncatedOnEmittedFrame_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(10, 50).Draw(t, "lineLimit")
		// Oversized input forces force-cut → emission with
		// IsTruncated=true.
		excess := lineLimit + rapid.IntRange(1, 40).Draw(t, "excess")
		body := make([]byte, excess)
		for i := range body {
			body[i] = 'y'
		}

		var emittedPtrs []*message.Message
		fr := NewFramer(
			func(m *message.Message, _ int) {
				emittedPtrs = append(emittedPtrs, m)
			},
			UTF8Newline,
			lineLimit,
		)

		input := message.NewMessage(body, nil, "", 0)
		// Input deliberately starts with IsTruncated=false.
		input.ParsingExtra.IsTruncated = false
		fr.Process(input)

		// Input's flag must be unchanged.
		if input.ParsingExtra.IsTruncated {
			t.Fatal("IsTruncatedOnEmittedFrame violated: input message's IsTruncated flipped to true (framer should not mutate the input)")
		}
		// At least one emission expected (force-cut).
		if len(emittedPtrs) == 0 {
			t.Fatal("expected at least 1 emission from oversized input, got 0")
		}
		// The emitted frame is a new message — different pointer
		// from input — with IsTruncated=true.
		first := emittedPtrs[0]
		if first == input {
			t.Fatal("IsTruncatedOnEmittedFrame violated: emitted frame is the SAME pointer as input (framer should construct a new message)")
		}
		if !first.ParsingExtra.IsTruncated {
			t.Fatal("IsTruncatedOnEmittedFrame violated: emitted frame's IsTruncated=false on oversized input")
		}
	})
}

// TestFramer_DatagramEndOfProcessFlush_Property anchors:
//
//	surface FramerTruncation (framer.allium)
//	    @guarantee DatagramEndOfProcessFlush — when
//	                                            flush_after_process
//	                                            is true and Process
//	                                            completes with a
//	                                            non-empty un-framed
//	                                            buffer, the framer
//	                                            emits the remaining
//	                                            buffer as a single
//	                                            frame with
//	                                            IsTruncated=false.
//
// UTF8NewlineDatagram framing on input WITHOUT a terminating
// newline AND under content_len_limit emits the full input as
// one frame with IsTruncated=false at the end of Process().
// Datagram framings flush residuals automatically.
func TestFramer_DatagramEndOfProcessFlush_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(20, 100).Draw(t, "lineLimit")
		bodyLen := rapid.IntRange(1, lineLimit/2).Draw(t, "bodyLen")
		body := make([]byte, bodyLen)
		for i := range body {
			body[i] = byte('a' + (i % 6))
		}

		fr, emissions := newFramerTestRig(UTF8NewlineDatagram, lineLimit)
		feedFramer(fr, body)

		if len(*emissions) != 1 {
			t.Fatalf("DatagramEndOfProcessFlush violated: expected 1 end-of-Process emission, got %d", len(*emissions))
		}
		e := (*emissions)[0]
		if e.isTruncated {
			t.Fatalf("DatagramEndOfProcessFlush violated: end-of-Process emission has IsTruncated=true on under-limit content %q", e.content)
		}
		if !bytes.Equal(e.content, body) {
			t.Fatalf("DatagramEndOfProcessFlush violated: emission content %q != input body %q", e.content, body)
		}
	})
}
