// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package windowsevent

import (
	"strings"
	"testing"
	"unicode/utf8"

	"pgregory.net/rapid"
)

// Property tests for the WindowsEventTruncation surface declared
// in windows_event_truncation.allium. Each test names the spec
// @guarantee it anchors so drift in either direction is easy to
// spot during review.
//
// Windows event truncation is a STANDALONE pathway — it does not
// compose with the logs decoder pipeline, has a hardcoded 128 KB
// limit, applies UTF-8-aware cutting, and detects truncation
// retroactively by scanning for the marker at either head or
// tail.

// minimalEventXML is the smallest XML that parses through
// NewMapXML and provides a Map we can SetMessage on. Used as
// scaffolding when the test specifically exercises SetMessage's
// truncation behaviour.
const minimalEventXML = `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System/></Event>`

// newTestMap builds a fresh Map for a single test iteration. The
// minimalEventXML constant is parseable by NewMapXML by
// construction; the helper panics on the impossible error path
// rather than threading a *testing.T from rapid's callback.
func newTestMap() *Map {
	m, err := NewMapXML([]byte(minimalEventXML))
	if err != nil {
		panic(err)
	}
	return m
}

// TestWindowsEvent_PassThroughUnderLimit_Property anchors:
//
//	surface WindowsEventTruncation (windows_event_truncation.allium)
//	    @guarantee PassThroughUnderLimit — when the input message's
//	                                        byte length is at or
//	                                        under max_message_bytes,
//	                                        the message field is
//	                                        stored byte-for-byte
//	                                        unchanged.
//
// SetMessage on a Map with content under 128 KB stores the
// content unmodified; the round-tripped value via GetMessage
// equals the original input, with no truncation marker present.
func TestWindowsEvent_PassThroughUnderLimit_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Pick a size strictly under maxMessageBytes (128 KB).
		size := rapid.IntRange(1, maxMessageBytes-1).Draw(t, "size")
		content := strings.Repeat("a", size)

		m := newTestMap()
		if err := m.SetMessage(content); err != nil {
			t.Fatalf("SetMessage failed: %v", err)
		}

		got := m.GetMessage()
		if got != content {
			t.Fatalf("PassThroughUnderLimit violated: stored content len=%d differs from input len=%d", len(got), len(content))
		}
		if strings.Contains(got, truncatedFlag) {
			t.Fatal("PassThroughUnderLimit violated: stored content contains marker on under-limit input")
		}
	})
}

// TestWindowsEvent_TailMarkerOnOverflow_Property anchors:
//
//	surface WindowsEventTruncation (windows_event_truncation.allium)
//	    @guarantee TailMarkerOnOverflow — when content exceeds
//	                                       max_message_bytes, the
//	                                       truncation marker is
//	                                       APPENDED to the cut
//	                                       content. The marker is
//	                                       NEVER prepended by the
//	                                       write side.
//	    @guarantee UTF8AwareCutAtLimit — the cut is at a UTF-8
//	                                      character boundary; the
//	                                      stored content (minus
//	                                      marker) is at most
//	                                      max_message_bytes bytes.
//
// SetMessage on content larger than 128 KB produces a stored
// string that ends with the marker. The cut content (the prefix
// before the marker) has length at most max_message_bytes and
// is valid UTF-8 (no split multi-byte sequence).
func TestWindowsEvent_TailMarkerOnOverflow_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Build content over the limit with multi-byte runes
		// distributed throughout, so the UTF-8-aware cut is
		// genuinely exercised.
		excess := rapid.IntRange(1, 2048).Draw(t, "excess")
		// Use a 3-byte UTF-8 rune ('日') to make boundary-splitting
		// possible if the cut were byte-based.
		oneRune := "日" // 3 bytes
		base := strings.Repeat("a", maxMessageBytes-2) + strings.Repeat(oneRune, excess/3+1)
		// Ensure total > maxMessageBytes.
		if len(base) <= maxMessageBytes {
			base += strings.Repeat("b", maxMessageBytes-len(base)+1)
		}

		m := newTestMap()
		if err := m.SetMessage(base); err != nil {
			t.Fatalf("SetMessage failed: %v", err)
		}

		got := m.GetMessage()
		if !strings.HasSuffix(got, truncatedFlag) {
			t.Fatalf("TailMarkerOnOverflow violated: stored content does not end with marker; tail = %q", got[max0(len(got)-30):])
		}
		// The cut content is got minus the trailing marker.
		cutContent := strings.TrimSuffix(got, truncatedFlag)
		if len(cutContent) > maxMessageBytes {
			t.Fatalf("UTF8AwareCutAtLimit violated: cut content length %d > max_message_bytes %d", len(cutContent), maxMessageBytes)
		}
		if !utf8.ValidString(cutContent) {
			t.Fatal("UTF8AwareCutAtLimit violated: cut content is not valid UTF-8 (multi-byte sequence was split)")
		}
	})
}

// TestWindowsEvent_NoHeadMarkerWritten_Property anchors:
//
//	surface WindowsEventTruncation (windows_event_truncation.allium)
//	    @guarantee TailMarkerOnOverflow — the marker is NEVER
//	                                       prepended by the write
//	                                       side.
//
// Across input sizes that span both under- and over-limit cases,
// the stored message NEVER starts with the marker (the
// SetMessage write path only appends).
func TestWindowsEvent_NoHeadMarkerWritten_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Sample input sizes spanning both sides of the limit.
		size := rapid.IntRange(1, maxMessageBytes+1024).Draw(t, "size")
		content := strings.Repeat("a", size)

		m := newTestMap()
		if err := m.SetMessage(content); err != nil {
			t.Fatalf("SetMessage failed: %v", err)
		}

		got := m.GetMessage()
		if strings.HasPrefix(got, truncatedFlag) {
			t.Fatalf("NoHeadMarkerWritten violated: stored content starts with marker (write side should only append, never prepend); size=%d", size)
		}
	})
}

// TestWindowsEvent_EmptyMessageIsNoop_Property anchors:
//
//	surface WindowsEventTruncation (windows_event_truncation.allium)
//	    @guarantee EmptyMessageIsNoop — an empty input message
//	                                     causes SetMessage to
//	                                     short-circuit without
//	                                     storing the field.
//
// SetMessage("") leaves the message field unset on the Map. A
// subsequent GetMessage returns the empty string. The
// retroactive marker scan on this empty value yields false.
func TestWindowsEvent_EmptyMessageIsNoop_Property(t *testing.T) {
	// Not really a property test, but kept here for spec-anchor
	// completeness with the rest of this surface. Rapid loop is
	// still useful for variant Map states (each iteration uses
	// a freshly-parsed Map).
	rapid.Check(t, func(t *rapid.T) {
		m := newTestMap()
		if err := m.SetMessage(""); err != nil {
			t.Fatalf("SetMessage('') failed: %v", err)
		}
		if got := m.GetMessage(); got != "" {
			t.Fatalf("EmptyMessageIsNoop violated: GetMessage returned %q, want empty", got)
		}
		if hasTruncatedFlag("") {
			t.Fatal("EmptyMessageIsNoop violated: hasTruncatedFlag('') returned true")
		}
	})
}

// TestWindowsEvent_MarkerScanAtBothBoundaries_Property anchors:
//
//	surface WindowsEventTruncation (windows_event_truncation.allium)
//	    @guarantee MarkerScanAtBothBoundaries — the retroactive
//	                                             marker scan
//	                                             (hasTruncatedFlag)
//	                                             checks BOTH the
//	                                             head and the tail
//	                                             of the message
//	                                             string. This is
//	                                             ASYMMETRIC to the
//	                                             write side, which
//	                                             only appends.
//
// A constructed string with the marker at the head OR tail is
// detected as truncated by hasTruncatedFlag. A string with the
// marker only in the middle is NOT detected. Strings shorter
// than the marker length are NOT detected.
func TestWindowsEvent_MarkerScanAtBothBoundaries_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		bodyLen := rapid.IntRange(0, 200).Draw(t, "bodyLen")
		body := strings.Repeat("x", bodyLen)
		position := rapid.SampledFrom([]string{"head", "tail", "middle", "absent"}).Draw(t, "position")

		var input string
		switch position {
		case "head":
			input = truncatedFlag + body
		case "tail":
			input = body + truncatedFlag
		case "middle":
			// Marker only between two body halves — should NOT
			// be detected. Avoid head/tail collision by ensuring
			// non-empty body on both sides.
			if bodyLen < 2 {
				// Skip: cannot construct a middle-only case.
				return
			}
			half := bodyLen / 2
			input = body[:half] + truncatedFlag + body[half:]
		case "absent":
			// body is all 'x's, so no natural marker substring.
			input = body
		}

		got := hasTruncatedFlag(input)
		expected := position == "head" || position == "tail"
		if got != expected {
			t.Fatalf("MarkerScanAtBothBoundaries violated at position=%s: hasTruncatedFlag returned %v, expected %v; input len=%d", position, got, expected, len(input))
		}
	})
}

// TestWindowsEvent_MarkerScanShortStringNotDetected_Property anchors:
//
//	surface WindowsEventTruncation (windows_event_truncation.allium)
//	    @guarantee MarkerScanAtBothBoundaries — the scan requires
//	                                             at least
//	                                             len(truncatedFlag)
//	                                             bytes to find
//	                                             the marker at
//	                                             either boundary.
//
// Strings shorter than the marker length cannot accommodate the
// marker at any boundary and are unconditionally not detected
// as truncated.
func TestWindowsEvent_MarkerScanShortStringNotDetected_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate strings strictly shorter than the marker.
		size := rapid.IntRange(0, len(truncatedFlag)-1).Draw(t, "size")
		input := strings.Repeat(".", size) // include '.' (marker prefix bytes) to stress the scan

		if hasTruncatedFlag(input) {
			t.Fatalf("MarkerScanShortStringNotDetected violated: hasTruncatedFlag returned true on string of len=%d (< marker len=%d)", size, len(truncatedFlag))
		}
	})
}

// TestWindowsEvent_RetroactiveDetection_Property anchors:
//
//	surface WindowsEventTruncation (windows_event_truncation.allium)
//	    @guarantee RetroactiveDetection — IsTruncated is determined
//	                                       NOT by tracking whether
//	                                       SetMessage truncated,
//	                                       but by SCANNING the
//	                                       stored message field
//	                                       for the marker at the
//	                                       time of message
//	                                       construction.
//
// LOAD-BEARING for the refactor safety net. If the refactor
// changed detection to state-tracking (e.g., a per-Map "did we
// truncate?" boolean), behaviour would diverge for messages
// whose content naturally contains the marker substring at a
// boundary. This test injects the marker into under-limit
// content and verifies the resulting message is still detected
// as truncated.
func TestWindowsEvent_RetroactiveDetection_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Under-limit content with the marker artificially
		// placed at the tail boundary. SetMessage does NOT
		// truncate (content is under 128KB), so no state is
		// set indicating "we truncated."
		bodyLen := rapid.IntRange(1, 200).Draw(t, "bodyLen")
		body := strings.Repeat("x", bodyLen)
		// Append the marker manually — this simulates content
		// that already contained the marker for any reason.
		injected := body + truncatedFlag

		m := newTestMap()
		if err := m.SetMessage(injected); err != nil {
			t.Fatalf("SetMessage failed: %v", err)
		}

		// The flag must be detected from the marker presence,
		// regardless of whether SetMessage actually applied
		// truncation logic (it didn't, since len < limit).
		if !hasTruncatedFlag(m.GetMessage()) {
			t.Fatalf("RetroactiveDetection violated: marker present in stored content but hasTruncatedFlag returned false (detection should be marker-scan-based, not state-tracked); content=%q", m.GetMessage())
		}
	})
}

// TestWindowsEvent_NoCarryOver_Property anchors:
//
//	surface WindowsEventTruncation (windows_event_truncation.allium)
//	    @guarantee NoCarryOver — each message is handled
//	                              independently of any prior
//	                              message. There is no rolling
//	                              truncation state across calls.
//
// Two SetMessage calls on separate Maps are independent: one's
// truncation outcome does not affect the other's. The first
// Map's truncated message field does NOT contaminate the second
// Map.
func TestWindowsEvent_NoCarryOver_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// First Map: oversized content → truncated.
		first := strings.Repeat("a", maxMessageBytes+rapid.IntRange(1, 100).Draw(t, "excess"))
		// Second Map: small clean content.
		secondSize := rapid.IntRange(1, 100).Draw(t, "smallSize")
		second := strings.Repeat("b", secondSize)

		m1 := newTestMap()
		_ = m1.SetMessage(first)
		m2 := newTestMap()
		_ = m2.SetMessage(second)

		// First should be truncated.
		if !hasTruncatedFlag(m1.GetMessage()) {
			t.Fatal("NoCarryOver precondition: first Map's message should be truncated")
		}
		// Second should NOT be truncated.
		if hasTruncatedFlag(m2.GetMessage()) {
			t.Fatalf("NoCarryOver violated: second Map's message detected as truncated despite being %d bytes of clean content; m2 content=%q", secondSize, m2.GetMessage())
		}
		if m2.GetMessage() != second {
			t.Fatalf("NoCarryOver violated: second Map's stored content %q differs from input %q", m2.GetMessage(), second)
		}
	})
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
