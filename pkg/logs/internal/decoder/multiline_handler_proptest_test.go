// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// Property tests for the MultiLineHandling surface declared in
// multiline_handler.allium. Each test names the spec @guarantee
// it anchors so drift in either direction is easy to spot during
// review.
//
// MultiLineHandler is a deliberate non-fulfiller of the Truncatable
// contract — it conforms to most invariants but DEVIATES on
// UpstreamFlagPropagation. The UpstreamFlagIgnored test below is
// load-bearing for the refactor safety net.

// multiLineEmission captures one emitted message's observable
// state, deep-copied at the callback boundary. The handler mutates
// the message in place, so deep copying here is required to keep
// emissions stable across subsequent process calls.
type multiLineEmission struct {
	content     []byte
	isTruncated bool
	tags        []string
}

// multiLineTestRig bundles a MultiLineHandler with its emission
// collector. send() drives one process call; emissions() returns
// the accumulated captures so far.
type multiLineTestRig struct {
	handler *MultiLineHandler
	emitted []multiLineEmission
}

func newMultiLineTestRig(re *regexp.Regexp, lineLimit int) *multiLineTestRig {
	rig := &multiLineTestRig{}
	rig.handler = NewMultiLineHandler(
		func(m *message.Message) {
			rig.emitted = append(rig.emitted, multiLineEmission{
				content:     append([]byte(nil), m.GetContent()...),
				isTruncated: m.ParsingExtra.IsTruncated,
				tags:        append([]string(nil), m.ParsingExtra.Tags...),
			})
		},
		re,
		// Flush timeout long enough never to fire in unit-test
		// timing. We drive flushes explicitly when needed.
		time.Hour,
		lineLimit,
		false, // telemetryEnabled
		status.NewInfoRegistry(),
		"auto_multiline",
	)
	return rig
}

func (r *multiLineTestRig) send(content string, upstreamIsTruncated bool) {
	msg := message.NewMessage([]byte(content), nil, "", time.Now().UnixNano())
	msg.RawDataLen = len(content)
	msg.ParsingExtra.IsTruncated = upstreamIsTruncated
	r.handler.process(msg)
}

func (r *multiLineTestRig) flush() {
	r.handler.flush()
}

// TestMultiLineHandler_PatternMatchedOnceSafety_Property anchors:
//
//	surface MultiLineHandling (multiline_handler.allium)
//	    @guarantee PatternMatchedOnceSafety — until the pattern
//	                                           has matched at least
//	                                           once, every process
//	                                           call triggers an
//	                                           emission, preventing
//	                                           a misconfigured
//	                                           never-matching pattern
//	                                           from joining all input
//	                                           into one giant message.
//
// With a regex that never matches the generated content, a
// sequence of N process calls + a final flush produces exactly N
// emissions — each input line is emitted individually rather
// than accumulated.
func TestMultiLineHandler_PatternMatchedOnceSafety_Property(t *testing.T) {
	// Pattern that requires a leading "Z" — combined with content
	// drawn from a Z-free alphabet, the pattern never matches.
	re := regexp.MustCompile("^Z")

	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 8).Draw(t, "n")
		rig := newMultiLineTestRig(re, 200)

		for i := 0; i < n; i++ {
			line := string(rapid.SliceOfN(
				rapid.SampledFrom([]byte("abcdef0123")),
				1, 20,
			).Draw(t, "line"))
			rig.send(line, false)
		}
		rig.flush()

		if len(rig.emitted) != n {
			t.Fatalf("PatternMatchedOnceSafety violated: %d inputs produced %d emissions, expected %d", n, len(rig.emitted), n)
		}
	})
}

// TestMultiLineHandler_PassThroughUnderThreshold_Property anchors:
//
//	surface MultiLineHandling (multiline_handler.allium)
//	    @guarantee PassThroughUnderThreshold — accumulated buffer
//	                                            under line_limit +
//	                                            no carry +
//	                                            (UpstreamFlagIgnored:
//	                                            inputs may carry
//	                                            upstream flags but
//	                                            they are ignored) ⇒
//	                                            no markers,
//	                                            IsTruncated false.
//
// A pattern-bounded group whose accumulated content stays under
// line_limit emits with no markers and IsTruncated=false. Note
// that under-threshold accumulation never overflows, so the only
// way to get truncation in MultiLineHandler is via buffer
// overflow — upstream flags do NOT propagate (see
// UpstreamFlagIgnored test).
func TestMultiLineHandler_PassThroughUnderThreshold_Property(t *testing.T) {
	re := regexp.MustCompile("^START")

	rapid.Check(t, func(t *rapid.T) {
		// Generate a few short continuation lines (each <= 8
		// bytes from a small alphabet) and ensure their total
		// stays under the line_limit.
		nContinuations := rapid.IntRange(0, 3).Draw(t, "nContinuations")
		lineLimit := 200

		rig := newMultiLineTestRig(re, lineLimit)
		rig.send("START first", false)
		for i := 0; i < nContinuations; i++ {
			line := string(rapid.SliceOfN(
				rapid.SampledFrom([]byte("abcdef")),
				1, 8,
			).Draw(t, "cont"))
			rig.send(line, false)
		}
		rig.send("START second", false) // boundary → emits first group
		rig.flush()                     // emits second group too

		if len(rig.emitted) < 1 {
			t.Fatalf("expected at least 1 emission, got %d", len(rig.emitted))
		}

		marker := message.TruncatedFlag
		first := rig.emitted[0]
		if bytes.Contains(first.content, marker) {
			t.Fatalf("PassThroughUnderThreshold violated: marker found in first emission %q", first.content)
		}
		if first.isTruncated {
			t.Fatalf("PassThroughUnderThreshold violated: IsTruncated=true on under-threshold emission %q", first.content)
		}
	})
}

// TestMultiLineHandler_TailMarkerOnBufferOverflow_Property anchors:
//
//	surface MultiLineHandling (multiline_handler.allium)
//	    @guarantee TailMarkerOnBufferOverflow — accumulated buffer
//	                                             >= line_limit ⇒
//	                                             tail marker
//	                                             appended,
//	                                             is_buffer_truncated
//	                                             true, sendBuffer
//	                                             called within the
//	                                             same process call.
//
// A pattern-bounded group whose accumulated content reaches or
// exceeds line_limit emits with the tail marker appended and
// IsTruncated=true on the SAME process call that caused the
// overflow.
func TestMultiLineHandler_TailMarkerOnBufferOverflow_Property(t *testing.T) {
	re := regexp.MustCompile("^START")

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(8, 30).Draw(t, "lineLimit")
		// Continuation line guaranteed to push the buffer over
		// limit: lineLimit*2 bytes from a single-char alphabet.
		contLen := lineLimit*2 + rapid.IntRange(0, 20).Draw(t, "extra")
		contBytes := bytes.Repeat([]byte("x"), contLen)

		rig := newMultiLineTestRig(re, lineLimit)
		rig.send("START a", false)         // buffer ~7 bytes
		rig.send(string(contBytes), false) // overflow expected on this call

		// At least one emission should have been produced by
		// the overflow path; it should have the tail marker
		// and IsTruncated=true.
		if len(rig.emitted) == 0 {
			t.Fatal("TailMarkerOnBufferOverflow violated: expected emission after overflow, got none")
		}
		e := rig.emitted[len(rig.emitted)-1]
		marker := message.TruncatedFlag
		if !bytes.HasSuffix(e.content, marker) {
			t.Fatalf("TailMarkerOnBufferOverflow violated: no tail marker on overflow emission %q", e.content)
		}
		if !e.isTruncated {
			t.Fatal("TailMarkerOnBufferOverflow violated: IsTruncated=false on overflow emission")
		}
	})
}

// TestMultiLineHandler_HeadMarkerOnCarryover_Property anchors:
//
//	surface MultiLineHandling (multiline_handler.allium)
//	    @guarantee HeadMarkerOnCarryover — after a process call
//	                                        that triggered overflow
//	                                        (setting should_truncate
//	                                        for the next call), the
//	                                        next NON-pattern-
//	                                        matching call prepends
//	                                        the truncation marker
//	                                        to its accumulated
//	                                        content.
//
// Sequence: pattern + overflow continuation → emission with tail
// marker (carry set). Next NON-pattern line is accumulated with
// the head marker prepended; flushing emits content beginning
// with the marker.
func TestMultiLineHandler_HeadMarkerOnCarryover_Property(t *testing.T) {
	re := regexp.MustCompile("^START")

	rapid.Check(t, func(t *rapid.T) {
		// lineLimit large enough that the post-carry accumulation
		// (head marker 15 bytes + 1-3 content bytes = 16-18 bytes)
		// does NOT itself overflow, so it survives until flush.
		lineLimit := rapid.IntRange(25, 60).Draw(t, "lineLimit")
		// Overflow content: lineLimit + 5 bytes guarantees the
		// call 2 accumulation ("START first" + sep + this content)
		// exceeds lineLimit.
		overflowBytes := bytes.Repeat([]byte("x"), lineLimit+5)
		shortLen := rapid.IntRange(1, 3).Draw(t, "shortLen")
		shortBytes := bytes.Repeat([]byte("a"), shortLen)

		rig := newMultiLineTestRig(re, lineLimit)
		rig.send("START first", false)
		rig.send(string(overflowBytes), false) // emits tail-marker, sets carry
		rig.send(string(shortBytes), false)    // accumulates with head marker
		rig.flush()                            // emits the carry-marked content

		if len(rig.emitted) < 2 {
			t.Fatalf("HeadMarkerOnCarryover violated: expected at least 2 emissions, got %d", len(rig.emitted))
		}
		// The last emission (from flush) should start with the
		// head marker (carried over from the overflow above).
		e := rig.emitted[len(rig.emitted)-1]
		marker := message.TruncatedFlag
		if !bytes.HasPrefix(e.content, marker) {
			t.Fatalf("HeadMarkerOnCarryover violated: no head marker on emission after carry; content %q", e.content)
		}
		if !e.isTruncated {
			t.Fatal("HeadMarkerOnCarryover violated: IsTruncated=false on emission with head marker")
		}
	})
}

// TestMultiLineHandler_CarryoverConsumed_Property anchors:
//
//	surface MultiLineHandling (multiline_handler.allium)
//	    @guarantee CarryoverConsumed — a process call that consumes
//	                                    carry=true but does NOT
//	                                    itself overflow leaves
//	                                    should_truncate=false at
//	                                    call exit; the next emission
//	                                    does NOT inherit a head
//	                                    marker.
//
// Three emissions: overflow (tail marker), consume-carry (head
// marker), fresh accumulation (no markers). The third emission's
// lack of head marker proves the carry was consumed and not
// propagated.
func TestMultiLineHandler_CarryoverConsumed_Property(t *testing.T) {
	re := regexp.MustCompile("^START")

	rapid.Check(t, func(t *rapid.T) {
		// Same lineLimit considerations as HeadMarkerOnCarryover:
		// post-carry accumulation (head marker + short content)
		// must stay under lineLimit so it survives until the
		// flush that produces emission 2.
		lineLimit := rapid.IntRange(25, 60).Draw(t, "lineLimit")
		overflowBytes := bytes.Repeat([]byte("x"), lineLimit+5)
		shortBytes := bytes.Repeat([]byte("a"), rapid.IntRange(1, 3).Draw(t, "shortLen"))
		freshBytes := bytes.Repeat([]byte("b"), rapid.IntRange(1, 3).Draw(t, "freshLen"))

		rig := newMultiLineTestRig(re, lineLimit)
		rig.send("START first", false)
		rig.send(string(overflowBytes), false) // emission 1: tail marker, carry set
		rig.send(string(shortBytes), false)    // accumulate with head marker
		rig.flush()                            // emission 2: head marker
		// Now carry is consumed. Start a fresh group.
		rig.send("START second", false)     // sendBuffer (empty), then accumulate
		rig.send(string(freshBytes), false) // accumulates more (no markers)
		rig.flush()                         // emission 3: no markers

		if len(rig.emitted) < 3 {
			t.Fatalf("CarryoverConsumed setup: expected at least 3 emissions, got %d", len(rig.emitted))
		}
		marker := message.TruncatedFlag

		// Precondition: emission 2 has head marker.
		if !bytes.HasPrefix(rig.emitted[1].content, marker) {
			t.Fatalf("CarryoverConsumed precondition: emission 2 missing head marker; got %q", rig.emitted[1].content)
		}

		// The point: emission 3 has NO markers.
		e := rig.emitted[2]
		if bytes.Contains(e.content, marker) {
			t.Fatalf("CarryoverConsumed violated: emission 3 contains a marker — the carry was not consumed; content %q", e.content)
		}
		if e.isTruncated {
			t.Fatalf("CarryoverConsumed violated: emission 3 IsTruncated=true on clean accumulation; content %q", e.content)
		}
	})
}

// TestMultiLineHandler_UpstreamFlagIgnored_Property anchors:
//
//	surface MultiLineHandling (multiline_handler.allium)
//	    @guarantee UpstreamFlagIgnored — input
//	                                      ParsingExtra.IsTruncated
//	                                      is NOT read and does NOT
//	                                      influence any truncation
//	                                      decision; deliberate
//	                                      DEVIATION from the
//	                                      Truncatable contract's
//	                                      UpstreamFlagPropagation.
//
// This is the LOAD-BEARING test for the refactor safety net.
// Inputs with upstream IsTruncated=true that do NOT cause buffer
// overflow MUST emit with IsTruncated=false. A refactor that
// began honouring the upstream flag here would break this test
// — exactly the behaviour change we want to prevent
// inadvertently.
func TestMultiLineHandler_UpstreamFlagIgnored_Property(t *testing.T) {
	re := regexp.MustCompile("^START")

	rapid.Check(t, func(t *rapid.T) {
		// Large limit so buffer never overflows.
		lineLimit := 1000
		shortBytes := bytes.Repeat([]byte("a"), rapid.IntRange(1, 8).Draw(t, "shortLen"))

		rig := newMultiLineTestRig(re, lineLimit)
		rig.send("START first", false)
		// All continuation lines are upstream-flagged but stay
		// well under the buffer limit.
		nFlagged := rapid.IntRange(1, 4).Draw(t, "nFlagged")
		for i := 0; i < nFlagged; i++ {
			rig.send(string(shortBytes), true)
		}
		rig.send("START second", false) // triggers emission of the prior group

		if len(rig.emitted) < 1 {
			t.Fatalf("UpstreamFlagIgnored: expected at least 1 emission, got %d", len(rig.emitted))
		}
		e := rig.emitted[0]
		marker := message.TruncatedFlag
		if bytes.Contains(e.content, marker) {
			t.Fatalf("UpstreamFlagIgnored violated: emission contains truncation marker — upstream flag was honoured; content %q", e.content)
		}
		if e.isTruncated {
			t.Fatalf("UpstreamFlagIgnored violated: emission IsTruncated=true despite no buffer overflow — upstream flag was propagated; content %q", e.content)
		}
	})
}

// TestMultiLineHandler_TagOnTruncation_Property anchors:
//
//	surface MultiLineHandling (multiline_handler.allium)
//	    @guarantee TagOnTruncation — when global
//	                                  logs_config.tag_truncated_logs
//	                                  is true, a truncation-reason
//	                                  tag is appended iff
//	                                  is_buffer_truncated.
//	    @guarantee TagReasonMultilineRegex — tag value is
//	                                          "multiline_regex".
//
// With the global config enabled, every emission whose
// IsTruncated is true carries the truncation-reason tag with
// value "multiline_regex" (distinct from the "single_line"
// reason used by SingleLineHandler and the per-line aggregators).
// Emissions with IsTruncated=false do not carry it.
func TestMultiLineHandler_TagOnTruncation_Property(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.tag_truncated_logs", true, pkgconfigmodel.SourceAgentRuntime)
	expectedTag := message.TruncatedReasonTag("multiline_regex")
	re := regexp.MustCompile("^START")

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(8, 30).Draw(t, "lineLimit")
		overflowBytes := bytes.Repeat([]byte("x"), lineLimit*2)

		rig := newMultiLineTestRig(re, lineLimit)
		rig.send("START a", false)
		rig.send(string(overflowBytes), false) // overflow → truncated emission

		for i, e := range rig.emitted {
			if e.isTruncated {
				hasTag := false
				for _, tag := range e.tags {
					if tag == expectedTag {
						hasTag = true
						break
					}
				}
				if !hasTag {
					t.Fatalf("TagOnTruncation/TagReasonMultilineRegex violated at emission %d: IsTruncated=true but no %q in tags=%v", i, expectedTag, e.tags)
				}
			} else {
				for _, tag := range e.tags {
					if tag == expectedTag {
						t.Fatalf("TagOnTruncation violated at emission %d: IsTruncated=false but tags contain %q (tags=%v)", i, expectedTag, e.tags)
					}
				}
			}
		}
	})
}

// TestMultiLineHandler_TagDisabledNoTag_Property anchors:
//
//	surface MultiLineHandling (multiline_handler.allium)
//	    @guarantee TagOnTruncation — when global
//	                                  logs_config.tag_truncated_logs
//	                                  is false, NO truncation-reason
//	                                  tag is added regardless of
//	                                  truncation state.
//
// With the global config disabled, no emission carries the
// "multiline_regex" truncation-reason tag, even when buffer
// overflow has applied markers and IsTruncated=true.
func TestMultiLineHandler_TagDisabledNoTag_Property(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.tag_truncated_logs", false, pkgconfigmodel.SourceAgentRuntime)
	suppressedTag := message.TruncatedReasonTag("multiline_regex")
	re := regexp.MustCompile("^START")

	rapid.Check(t, func(t *rapid.T) {
		lineLimit := rapid.IntRange(8, 30).Draw(t, "lineLimit")
		overflowBytes := bytes.Repeat([]byte("x"), lineLimit*2)

		rig := newMultiLineTestRig(re, lineLimit)
		rig.send("START a", false)
		rig.send(string(overflowBytes), false) // overflow

		for i, e := range rig.emitted {
			for _, tag := range e.tags {
				if tag == suppressedTag {
					t.Fatalf("TagOnTruncation (disabled) violated at emission %d: found %q in tags=%v", i, suppressedTag, e.tags)
				}
			}
		}
	})
}

// TestMultiLineHandler_LineSeparator_Property anchors:
//
//	surface MultiLineHandling (multiline_handler.allium)
//	    @guarantee LineSeparator — between consecutive input lines
//	                                accumulated into the same
//	                                buffer, the handler writes the
//	                                EscapedLineFeed marker.
//
// A pattern-bounded group containing 2+ input lines emits content
// where the EscapedLineFeed separator appears between adjacent
// line contents. Separator bytes count toward buffered_content_len
// for the overflow check.
func TestMultiLineHandler_LineSeparator_Property(t *testing.T) {
	re := regexp.MustCompile("^START")

	rapid.Check(t, func(t *rapid.T) {
		// Large limit so no overflow.
		nContinuations := rapid.IntRange(1, 3).Draw(t, "nContinuations")
		rig := newMultiLineTestRig(re, 1000)

		// Build a sequence of distinct continuation lines so we
		// can find them in the emission and verify a separator
		// appears between them.
		conts := make([]string, nContinuations)
		for i := 0; i < nContinuations; i++ {
			conts[i] = "cont" + string([]byte{byte('A' + i)})
		}

		rig.send("START leader", false)
		for _, c := range conts {
			rig.send(c, false)
		}
		rig.send("START next", false) // triggers emission

		if len(rig.emitted) < 1 {
			t.Fatal("expected at least 1 emission")
		}
		content := string(rig.emitted[0].content)
		separator := string(message.EscapedLineFeed)

		// Every pair of consecutive expected pieces (leader,
		// conts[0], conts[1], ...) should have the separator
		// between them.
		pieces := append([]string{"START leader"}, conts...)
		for i := 0; i < len(pieces)-1; i++ {
			pair := pieces[i] + separator + pieces[i+1]
			if !strings.Contains(content, pair) {
				t.Fatalf("LineSeparator violated: expected %q (separator between %q and %q) in emission content %q", pair, pieces[i], pieces[i+1], content)
			}
		}
	})
}

// TestMultiLineHandler_MultiLineSourceTag_Property anchors:
//
//	surface MultiLineHandling (multiline_handler.allium)
//	    @guarantee MultiLineSourceTag — when lines_combined > 1 at
//	                                     sendBuffer time AND
//	                                     logs_config.tag_multi_line_logs
//	                                     is true, the multi-line
//	                                     source tag with the
//	                                     constructor-supplied tag
//	                                     value is appended.
//
// A pattern-bounded group containing 2+ input lines emits with
// the multi-line source tag. A group of exactly 1 line does NOT
// receive the tag.
func TestMultiLineHandler_MultiLineSourceTag_Property(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.Set("logs_config.tag_multi_line_logs", true, pkgconfigmodel.SourceAgentRuntime)
	multiLineTag := message.MultiLineSourceTag("auto_multiline")
	re := regexp.MustCompile("^START")

	rapid.Check(t, func(t *rapid.T) {
		nContinuations := rapid.IntRange(1, 4).Draw(t, "nContinuations")
		rig := newMultiLineTestRig(re, 1000) // large, no overflow
		rig.send("START first", false)
		for i := 0; i < nContinuations; i++ {
			rig.send("more", false)
		}
		rig.send("START second", false) // triggers emission of the multi-line group

		if len(rig.emitted) < 1 {
			t.Fatalf("expected at least 1 emission, got %d", len(rig.emitted))
		}
		// First emission contains 1 + nContinuations lines (>= 2).
		// It should carry the multi-line source tag.
		first := rig.emitted[0]
		hasTag := false
		for _, tag := range first.tags {
			if tag == multiLineTag {
				hasTag = true
				break
			}
		}
		if !hasTag {
			t.Fatalf("MultiLineSourceTag violated: %d-line group emission missing tag %q; tags=%v", 1+nContinuations, multiLineTag, first.tags)
		}
	})
}
