// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"bytes"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Property tests for the MultiLineParserTruncation surface declared
// in line_parser.allium. Each test names the spec @guarantee it
// anchors so drift in either direction is easy to spot during
// review.
//
// MultiLineParser is a flag-only accumulator — it propagates the
// IsTruncated flag but never adds the `...TRUNCATED...` marker
// bytes. The EarlierContributorFlagsLostWithinCycle test below is
// load-bearing for the refactor safety net: it locks the current
// behaviour that within a partial-accumulation cycle, only the
// LAST input's upstream flag survives to the emission.

// lineParserEmission captures one emission's observable state,
// deep-copied at the callback boundary.
type lineParserEmission struct {
	content     []byte
	isTruncated bool
}

// capturingLineHandler implements LineHandler by appending each
// emitted message's content + IsTruncated state to a slice.
type capturingLineHandler struct {
	emitted []lineParserEmission
}

func (h *capturingLineHandler) process(m *message.Message) {
	h.emitted = append(h.emitted, lineParserEmission{
		content:     append([]byte(nil), m.GetContent()...),
		isTruncated: m.ParsingExtra.IsTruncated,
	})
}

func (h *capturingLineHandler) flushChan() <-chan time.Time { return nil }
func (h *capturingLineHandler) flush()                      {}

// newTestMultiLineParser builds a MultiLineParser with the
// noop parser and a capturing line handler. The noop parser
// preserves IsPartial / IsTruncated on input messages, letting
// tests drive the accumulator directly via the input flags.
func newTestMultiLineParser(lineLimit int) (*MultiLineParser, *capturingLineHandler) {
	h := &capturingLineHandler{}
	p := NewMultiLineParser(h, time.Hour, noop.New(), lineLimit)
	return p, h
}

func sendLineParser(p *MultiLineParser, content string, isPartial bool, isTruncated bool) {
	msg := message.NewMessage([]byte(content), nil, "", time.Now().UnixNano())
	msg.ParsingExtra.IsPartial = isPartial
	msg.ParsingExtra.IsTruncated = isTruncated
	p.process(msg, len(content))
}

// TestMultiLineParser_FlagOnlyAccumulator_Property anchors:
//
//	surface MultiLineParserTruncation (line_parser.allium)
//	    @guarantee FlagOnlyAccumulator — the MultiLineParser adds
//	                                      no `...TRUNCATED...`
//	                                      marker bytes and cuts no
//	                                      content; only the
//	                                      IsTruncated flag is
//	                                      produced.
//
// Across arbitrary input sequences, no emission's content
// contains the truncation marker byte sequence. The parser is
// purely a flag accumulator.
func TestMultiLineParser_FlagOnlyAccumulator_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(10, 100).Draw(t, "lineLimit")
		n := rapid.IntRange(1, 8).Draw(t, "n")

		p, h := newTestMultiLineParser(lineLimit)
		for i := 0; i < n; i++ {
			content := string(rapid.SliceOfN(
				rapid.SampledFrom([]byte("abcdef0123")),
				0, 30,
			).Draw(t, "content"))
			isPartial := rapid.Bool().Draw(t, "isPartial")
			isTruncated := rapid.Bool().Draw(t, "isTruncated")
			sendLineParser(p, content, isPartial, isTruncated)
		}
		p.flush()

		marker := message.TruncatedFlag
		for i, e := range h.emitted {
			if bytes.Contains(e.content, marker) {
				t.Fatalf("FlagOnlyAccumulator violated: emission %d contains marker bytes; content %q", i, e.content)
			}
		}
	})
}

// TestMultiLineParser_BufferOverflowMarksTruncated_Property anchors:
//
//	surface MultiLineParserTruncation (line_parser.allium)
//	    @guarantee BufferOverflowMarksTruncated — when accumulated
//	                                               buffer reaches
//	                                               or exceeds
//	                                               line_limit,
//	                                               is_buffer_truncated
//	                                               is set true.
//	    @guarantee BufferOverflowForcesEmission — buffer overflow
//	                                               forces emission
//	                                               in the same
//	                                               process call.
//
// When IsPartial=true inputs accumulate until the buffer crosses
// lineLimit, the emission produced by the overflowing call
// carries IsTruncated=true.
func TestMultiLineParser_BufferOverflowMarksTruncated_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(10, 40).Draw(t, "lineLimit")
		// Send a single oversized chunk while IsPartial=true:
		// guaranteed to overflow on this call, forcing emission.
		overflowLen := lineLimit + rapid.IntRange(1, 20).Draw(t, "extra")
		content := string(make([]byte, overflowLen))
		// Fill with non-marker bytes.
		buf := []byte(content)
		for i := range buf {
			buf[i] = 'x'
		}

		p, h := newTestMultiLineParser(lineLimit)
		sendLineParser(p, string(buf), true /* IsPartial */, false /* IsTruncated */)

		if len(h.emitted) != 1 {
			t.Fatalf("BufferOverflowForcesEmission violated: expected 1 emission on overflow, got %d", len(h.emitted))
		}
		e := h.emitted[0]
		if !e.isTruncated {
			t.Fatalf("BufferOverflowMarksTruncated violated: IsTruncated=false on overflow emission; content len=%d limit=%d", len(e.content), lineLimit)
		}
	})
}

// TestMultiLineParser_BufferOverflowForcesEmissionDespitePartial_Property anchors:
//
//	surface MultiLineParserTruncation (line_parser.allium)
//	    @guarantee BufferOverflowForcesEmission — emission is
//	                                               forced even
//	                                               when the input's
//	                                               IsPartial flag
//	                                               would normally
//	                                               continue
//	                                               accumulation.
//
// A sequence of IsPartial=true inputs with no overflow does NOT
// emit, but adding one input whose own content alone exceeds
// lineLimit forces emission on that same call even though
// IsPartial is still true.
func TestMultiLineParser_BufferOverflowForcesEmissionDespitePartial_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(20, 60).Draw(t, "lineLimit")

		p, h := newTestMultiLineParser(lineLimit)
		// First: short IsPartial input → no emission.
		sendLineParser(p, "a", true, false)
		if len(h.emitted) != 0 {
			t.Fatalf("precondition: short partial input should not emit, got %d emissions", len(h.emitted))
		}
		// Second: massive IsPartial input → overflow forces emit.
		massive := make([]byte, lineLimit+rapid.IntRange(1, 30).Draw(t, "extra"))
		for i := range massive {
			massive[i] = 'y'
		}
		sendLineParser(p, string(massive), true, false)

		if len(h.emitted) != 1 {
			t.Fatalf("BufferOverflowForcesEmission violated: expected 1 emission after overflow with IsPartial=true, got %d", len(h.emitted))
		}
	})
}

// TestMultiLineParser_EmissionPropagatesLastInputFlag_Property anchors:
//
//	surface MultiLineParserTruncation (line_parser.allium)
//	    @guarantee EmissionPropagatesLastInputAndBufferFlag — the
//	                                                          emission's
//	                                                          IsTruncated
//	                                                          is the
//	                                                          OR of the
//	                                                          last input's
//	                                                          upstream
//	                                                          flag and
//	                                                          is_buffer_truncated.
//
// When the terminating (IsPartial=false) input has
// IsTruncated=true upstream and the buffer stays under
// lineLimit, the emission's IsTruncated is true via the last-
// input source. This is the non-degenerate side of
// EmissionPropagatesLastInputAndBufferFlag — the upstream-flag
// half of the OR.
func TestMultiLineParser_EmissionPropagatesLastInputFlag_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := 200 // large, no overflow
		// Accumulate some partial content, then terminate with
		// an IsTruncated=true input.
		nPartials := rapid.IntRange(0, 3).Draw(t, "nPartials")

		p, h := newTestMultiLineParser(lineLimit)
		for i := 0; i < nPartials; i++ {
			sendLineParser(p, "a", true, false)
		}
		sendLineParser(p, "z", false /* terminator */, true /* upstream flag */)

		if len(h.emitted) != 1 {
			t.Fatalf("expected 1 emission, got %d", len(h.emitted))
		}
		if !h.emitted[0].isTruncated {
			t.Fatal("EmissionPropagatesLastInputAndBufferFlag violated: emission IsTruncated=false despite last input's upstream flag=true")
		}
	})
}

// TestMultiLineParser_EarlierContributorFlagsLostWithinCycle_Property anchors:
//
//	surface MultiLineParserTruncation (line_parser.allium)
//	    @guarantee EarlierContributorFlagsLostWithinCycle — within
//	                                                         a
//	                                                         partial-
//	                                                         accumulation
//	                                                         cycle,
//	                                                         only the
//	                                                         LAST
//	                                                         input's
//	                                                         upstream
//	                                                         flag
//	                                                         survives;
//	                                                         earlier
//	                                                         flagged
//	                                                         contributors'
//	                                                         flags are
//	                                                         LOST if not
//	                                                         also flagged
//	                                                         on the
//	                                                         terminator.
//
// LOAD-BEARING for the refactor safety net. A cycle where
// EARLIER contributors arrived with IsTruncated=true upstream
// but the FINAL (terminating) contributor's flag is false and
// the buffer doesn't overflow MUST emit with IsTruncated=false.
// This is the current bufferedMsg-overwrite behaviour — a
// refactor that switched to OR-across-all-contributors would
// break this test, exactly the inadvertent behaviour change
// we're guarding against.
func TestMultiLineParser_EarlierContributorFlagsLostWithinCycle_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := 200 // large, no overflow
		nFlaggedEarlier := rapid.IntRange(1, 3).Draw(t, "nFlaggedEarlier")

		p, h := newTestMultiLineParser(lineLimit)
		// EARLIER contributors: IsPartial=true AND
		// IsTruncated=true upstream.
		for i := 0; i < nFlaggedEarlier; i++ {
			sendLineParser(p, "a", true /* partial */, true /* upstream flagged */)
		}
		// TERMINATOR: IsPartial=false, IsTruncated=false.
		sendLineParser(p, "z", false, false)

		if len(h.emitted) != 1 {
			t.Fatalf("expected 1 emission, got %d", len(h.emitted))
		}
		if h.emitted[0].isTruncated {
			t.Fatalf("EarlierContributorFlagsLostWithinCycle violated: emission IsTruncated=true despite terminator unflagged — earlier contributors' flags should have been LOST; content %q", h.emitted[0].content)
		}
	})
}

// TestMultiLineParser_NoContentCutting_Property anchors:
//
//	surface MultiLineParserTruncation (line_parser.allium)
//	    @guarantee NoContentCutting — the emitted message's
//	                                   content is the FULL
//	                                   accumulated buffer
//	                                   byte-for-byte, regardless
//	                                   of whether
//	                                   buffered_content_len
//	                                   exceeded line_limit.
//
// When accumulated buffer crosses lineLimit, the resulting
// emission contains the FULL accumulated bytes (not cut to
// lineLimit). MultiLineParser does NOT enforce a hard byte
// limit on emitted content — it only flags it.
func TestMultiLineParser_NoContentCutting_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(10, 30).Draw(t, "lineLimit")
		// Build a content that's strictly larger than lineLimit.
		overflowLen := lineLimit + rapid.IntRange(5, 50).Draw(t, "extra")
		content := make([]byte, overflowLen)
		for i := range content {
			content[i] = byte('a' + (i % 6))
		}

		p, h := newTestMultiLineParser(lineLimit)
		sendLineParser(p, string(content), true /* partial — but overflow forces emit */, false)

		if len(h.emitted) != 1 {
			t.Fatalf("expected 1 emission, got %d", len(h.emitted))
		}
		if !bytes.Equal(h.emitted[0].content, content) {
			t.Fatalf("NoContentCutting violated: emission content len=%d != input len=%d (full content was NOT preserved)", len(h.emitted[0].content), len(content))
		}
	})
}

// TestMultiLineParser_FlushDrainsBuffer_Property anchors:
//
//	surface MultiLineParserTruncation (line_parser.allium)
//	    @guarantee FlushDrainsBuffer — flush() is equivalent to a
//	                                    forced sendLine: any
//	                                    buffered emission is
//	                                    produced.
//
// After IsPartial=true inputs accumulate (no emission yet),
// calling flush() produces exactly one emission containing the
// accumulated content. A second flush is a no-op.
func TestMultiLineParser_FlushDrainsBuffer_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := 200 // large, no overflow
		nPartials := rapid.IntRange(1, 4).Draw(t, "nPartials")

		p, h := newTestMultiLineParser(lineLimit)
		for i := 0; i < nPartials; i++ {
			sendLineParser(p, "a", true /* partial */, false)
		}
		// No emission yet (all partial, no overflow).
		if len(h.emitted) != 0 {
			t.Fatalf("precondition: %d partial sends should produce no emissions, got %d", nPartials, len(h.emitted))
		}
		p.flush()
		if len(h.emitted) != 1 {
			t.Fatalf("FlushDrainsBuffer violated: expected 1 emission from flush, got %d", len(h.emitted))
		}
		// Second flush: no-op.
		p.flush()
		if len(h.emitted) != 1 {
			t.Fatalf("FlushDrainsBuffer violated: second flush produced %d additional emissions, expected 0", len(h.emitted)-1)
		}
	})
}

// TestMultiLineParser_NoCarryOverBetweenEmissions_Property anchors:
//
//	surface MultiLineParserTruncation (line_parser.allium)
//	    @guarantee NoCarryOverBetweenEmissions — the
//	                                              is_buffer_truncated
//	                                              flag, the
//	                                              buffered_content_len
//	                                              value, and the
//	                                              last_input_upstream_truncated
//	                                              value are all
//	                                              reset on every
//	                                              sendLine.
//
// After an emission whose IsTruncated=true (via buffer
// overflow), a SECOND fresh cycle that stays under lineLimit
// and has no upstream flags emits with IsTruncated=false.
// Truncation state does NOT carry across emissions.
func TestMultiLineParser_NoCarryOverBetweenEmissions_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(15, 30).Draw(t, "lineLimit")

		p, h := newTestMultiLineParser(lineLimit)
		// Cycle 1: oversized input → emission with IsTruncated=true.
		massive := make([]byte, lineLimit+10)
		for i := range massive {
			massive[i] = 'x'
		}
		sendLineParser(p, string(massive), false /* terminator */, false)

		if len(h.emitted) != 1 || !h.emitted[0].isTruncated {
			t.Fatalf("precondition: first cycle should emit 1 truncated message, got %d emissions; first.isTruncated=%v", len(h.emitted), h.emitted[0].isTruncated)
		}

		// Cycle 2: short clean input → emission with IsTruncated=false.
		sendLineParser(p, "y", false /* terminator */, false)

		if len(h.emitted) != 2 {
			t.Fatalf("expected 2 total emissions after cycle 2, got %d", len(h.emitted))
		}
		if h.emitted[1].isTruncated {
			t.Fatalf("NoCarryOverBetweenEmissions violated: cycle 2 emission inherited IsTruncated=true from cycle 1; content %q", h.emitted[1].content)
		}
	})
}
