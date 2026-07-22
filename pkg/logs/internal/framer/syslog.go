// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

import (
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// syslogFrameMatcher implements FrameMatcher for syslog TCP streams per
// RFC 6587. Two framing methods are supported with automatic per-frame
// detection:
//
//   - Octet Counting (RFC 6587 §3.4.1 / RFC 5425):
//     SYSLOG-FRAME = MSG-LEN SP SYSLOG-MSG
//     The sender prefixes each message with its byte length as ASCII digits.
//
//   - Non-Transparent Framing (RFC 6587 §3.4.2):
//     Messages are terminated by a TRAILER character (LF or NUL).
//     Trailing LF, CR+LF, and NUL are stripped from the returned frame.
//
// Detection: the first byte of each frame determines the method — a digit
// ('1'-'9') selects octet counting, '<' (start of PRI) selects
// non-transparent framing. Stray whitespace/NUL between frames is consumed.
// overflowKind identifies how the continuation bytes of an oversized frame
// must be consumed. Once a frame exceeds contentLenLimit, its remaining bytes
// are emitted as raw continuation chunks instead of being re-run through frame
// detection — otherwise message content (e.g. an embedded "<134>" or "3 <")
// would be misread as framing and corrupt message boundaries.
type overflowKind int

const (
	overflowNone overflowKind = iota
	// overflowOctet: remaining length is known from MSG-LEN; see octetRemaining.
	overflowOctet
	// overflowNonTransparent: consume raw until the LF/NUL delimiter.
	overflowNonTransparent
	// overflowMalformed: consume raw until the next frame-start sync point.
	overflowMalformed
)

type syslogFrameMatcher struct {
	contentLenLimit int

	// overflow tracks an in-progress oversized frame whose continuation bytes
	// must be emitted raw. octetRemaining holds the number of declared body
	// bytes still owed while overflow == overflowOctet.
	overflow       overflowKind
	octetRemaining int

	malformedBytes  *status.CountInfo
	oversizedFrames *status.CountInfo
}

// FindFrame implements FrameMatcher. It looks for a complete syslog frame
// at the start of buf. The seen argument indicates how many bytes of buf
// were present on the last call (used to avoid rescanning).
//
// When the leading byte is not a valid syslog frame start ('<' or digit),
// the matcher scans forward for the next probable frame start — either a
// PRI header (<[0-9]) or an octet-counting prefix (digit+ SP <digit).
// Everything before that sync point is emitted as a single malformed frame
// so the downstream parser can log it coherently rather than producing one
// empty message per byte.
func (m *syslogFrameMatcher) FindFrame(buf []byte, seen int) ([]byte, int, bool) {
	if len(buf) == 0 {
		return nil, 0, false
	}

	// Continuation of an oversized frame: emit the remaining bytes as raw
	// chunks without re-detecting frame type, so message content cannot be
	// misread as framing.
	switch m.overflow {
	case overflowOctet:
		return m.emitOctetContinuation(buf)
	case overflowNonTransparent:
		return m.scanNonTransparent(buf, 0, true /* continuation */)
	case overflowMalformed:
		// If the malformed run ended exactly at the previous chunk boundary,
		// the next byte begins a real frame (or is a stray delimiter): clear
		// overflow and fall through to normal detection.
		if isSyslogFrameStart(buf, 0) || buf[0] == '\n' || buf[0] == '\r' || buf[0] == 0 {
			m.overflow = overflowNone
		} else {
			return m.scanMalformed(buf, seen, true /* continuation */)
		}
	}

	b := buf[0]
	switch {
	case b >= '1' && b <= '9':
		return m.findOctetCounted(buf, seen)

	case b == '<':
		return m.findNonTransparent(buf, seen)

	case b == '\n' || b == '\r' || b == 0:
		// Stray delimiter between frames: consume it, emit nothing.
		return nil, 1, false

	default:
		return m.scanMalformed(buf, seen, false /* continuation */)
	}
}

// maxFrameStartLookbehind bounds how far scanMalformed rewinds behind the
// already-scanned prefix (seen) before resuming its search for the next frame
// start. It must cover the longest frame-start signature isSyslogFrameStart can
// match across a read boundary so a signature whose tail only arrives in the
// current read is still detected: up to 10 length digits (the cap enforced by
// findOctetCounted) + SP + "<" + digit, i.e. ~13 bytes. 16 adds margin. Any
// "octet header" longer than this is rejected as malformed anyway, so there is
// no valid frame start beyond the look-behind to miss.
const maxFrameStartLookbehind = 16

// scanMalformed consumes bytes that do not start a valid syslog frame. buf[0]
// is known malformed (the FindFrame dispatch handles frame starts and stray
// delimiters), so it scans from index 1 for the next probable frame start (PRI
// header or octet-counting prefix), newline, or NUL and emits everything before
// it as a single malformed frame so the downstream parser can log it coherently
// rather than producing one empty message per byte. If no sync point is found
// and the buffer has not yet reached contentLenLimit, it returns nil to wait
// for more data. Malformed bytes are counted per emitted chunk so a run is
// never double-counted.
//
// A run longer than contentLenLimit is emitted in bounded chunks: the matcher
// emits contentLenLimit bytes, enters (or stays in) overflowMalformed, and the
// remainder returns here on the next call. The continuation flag distinguishes
// the first chunk of a fresh run (counts the oversized frame once) from a
// continuation chunk (already counted). Truncation follows "never the last": a
// bounded chunk emitted before the sync point is truncated, while the chunk
// that reaches the sync point (the final segment of the run) is not.
//
// seen is how many bytes of buf were already present (and scanned) on the
// previous call. The scan resumes at seen minus a bounded look-behind rather
// than restarting at index 1, so a delimiter-less malformed run delivered in
// many small reads is not re-scanned from the start every call (which would be
// quadratic in the run length). The look-behind re-examines the few bytes
// adjacent to the previous boundary so a frame-start signature split across
// reads is still found.
func (m *syslogFrameMatcher) scanMalformed(buf []byte, seen int, continuation bool) ([]byte, int, bool) {
	start := seen - maxFrameStartLookbehind
	if start < 1 {
		start = 1
	}
	for i := start; i < len(buf); i++ {
		if isSyslogFrameStart(buf, i) || buf[i] == '\n' || buf[i] == 0 {
			content := buf[:i]
			rawDataLen := i
			if buf[i] == '\n' || buf[i] == 0 {
				rawDataLen = i + 1
			}
			if len(content) > m.contentLenLimit {
				m.recordMalformed(int64(m.contentLenLimit))
				if !continuation {
					m.recordOversized()
				}
				m.overflow = overflowMalformed
				return buf[:m.contentLenLimit], m.contentLenLimit, true
			}
			m.recordMalformed(int64(len(content)))
			m.overflow = overflowNone
			// Reaching the sync point ends the malformed run, so this is its
			// final segment: never truncated ("never the last"). A whole
			// sub-limit run likewise emits untruncated.
			return content, rawDataLen, false
		}
	}
	// No sync point yet. If the run already fills a chunk, emit it now and
	// enter overflow rather than waiting (which would let the Framer guard
	// chop blindly and re-detect the continuation).
	if len(buf) >= m.contentLenLimit {
		m.recordMalformed(int64(m.contentLenLimit))
		if !continuation {
			m.recordOversized()
		}
		m.overflow = overflowMalformed
		return buf[:m.contentLenLimit], m.contentLenLimit, true
	}
	return nil, 0, false
}

// isSyslogFrameStart returns true if buf[i] looks like the start of a valid
// syslog frame. Two patterns are recognized:
//
//   - Non-transparent PRI header: <[0-9] (e.g. "<134>...")
//   - Octet-counting prefix: [1-9][0-9]* SP <[0-9] (e.g. "62 <134>...")
//
// The octet-counting check requires the full "digits SP <digit" signature
// to avoid false positives on bare digits in non-syslog content (e.g.,
// timestamps like "2026-04-20T12:00:00Z" or JSON values). Previously, any
// digit 1-9 was treated as a sync point, which caused a single JSON line
// to fragment into 13+ entries.
func isSyslogFrameStart(buf []byte, i int) bool {
	b := buf[i]
	if b == '<' && i+1 < len(buf) && buf[i+1] >= '0' && buf[i+1] <= '9' {
		return true
	}
	if b >= '1' && b <= '9' {
		j := i
		for j < len(buf) && buf[j] >= '0' && buf[j] <= '9' {
			j++
		}
		if j < len(buf) && buf[j] == ' ' &&
			j+1 < len(buf) && buf[j+1] == '<' &&
			j+2 < len(buf) && buf[j+2] >= '0' && buf[j+2] <= '9' {
			return true
		}
	}
	return false
}

// findOctetCounted parses MSG-LEN SP SYSLOG-MSG from the beginning of buf.
// Returns nil if the buffer does not yet contain enough data.
//
// The MSG-LEN SP header is transport framing and is stripped from emitted
// content. When the frame cannot be held whole under contentLenLimit — the
// declared body exceeds the limit, or the header pushes an otherwise sub-limit
// body past it before the body fully arrives — the matcher does NOT buffer the
// whole declared body; instead it emits a bounded first chunk, records the
// remainder in octetRemaining, and enters overflowOctet. MSG-LEN stays the
// authoritative boundary (RFC 6587 §3.4.1): the remaining declared bytes are
// emitted as raw continuation, never re-detected.
func (m *syslogFrameMatcher) findOctetCounted(buf []byte, seen int) ([]byte, int, bool) {
	// Parse the decimal length prefix.
	msgLen := 0
	i := 0
	for i < len(buf) {
		b := buf[i]
		if b == ' ' {
			i++ // consume the space
			break
		}
		// A non-digit before the SP terminator, or a prefix longer than any
		// plausible length (>10 digits), means these leading bytes are not an
		// octet-counting header after all — buf[0] was a digit, which is the
		// only reason we are here. Hand the whole run to scanMalformed so it is
		// emitted as one coherent malformed frame and counted once, rather than
		// silently dropping the digit prefix byte-by-byte while the remainder
		// is later emitted as malformed.
		if b < '0' || b > '9' {
			return m.scanMalformed(buf, seen, false /* continuation */)
		}
		i++
		if i > 10 {
			return m.scanMalformed(buf, seen, false /* continuation */)
		}
		msgLen = msgLen*10 + int(b-'0')
	}

	// If we consumed all of buf without finding SP, wait for more data.
	if i == len(buf) && (i == 0 || buf[i-1] != ' ') {
		return nil, 0, false
	}

	if msgLen == 0 {
		// "0 " is not a valid octet-counted frame — skip the prefix, emit nothing.
		return nil, i, false
	}

	headerLen := i // digits + SP
	totalLen := headerLen + msgLen

	// Common case: the whole frame is buffered and its body fits the limit.
	// Strip the MSG-LEN SP header and emit the body whole.
	if msgLen <= m.contentLenLimit && len(buf) >= totalLen {
		return buf[headerLen:totalLen], totalLen, false
	}

	// Otherwise the frame cannot be held whole under the limit: either the
	// declared body genuinely exceeds contentLenLimit, or the buffer filled to
	// the limit before the full body arrived (only reachable in the
	// pathological near-limit band, since real syslog lines are far below the
	// limit). Wait while still under the limit so the first chunk is as large
	// as the limit allows, then emit a bounded first chunk with the header
	// stripped and consume the declared remainder as raw continuation. MSG-LEN
	// stays the authoritative boundary (RFC 6587 §3.4.1). Such frames are
	// flagged truncated; any framing-level truncation signals a severe upstream
	// error, since syslog lines are conventionally far below the framer limit.
	if len(buf) < m.contentLenLimit {
		return nil, 0, false
	}
	emit := len(buf) - headerLen
	if emit > m.contentLenLimit {
		emit = m.contentLenLimit
	}
	m.recordOversized()
	m.octetRemaining = msgLen - emit
	m.overflow = overflowOctet
	return buf[headerLen : headerLen+emit], headerLen + emit, true
}

// emitOctetContinuation emits the next bounded chunk of an octet-counted
// frame's remaining declared body. Continuation chunks carry no header. Every
// chunk is flagged truncated except the one that completes the declared length:
// per the "never the last" contract, the final segment of the split frame is
// not truncated (it carries no trailing marker — MSG-LEN is the authoritative
// boundary, so the matcher knows when the body is complete). The matcher waits
// only for a single chunk's worth of bytes, so the buffer never exceeds the
// limit.
func (m *syslogFrameMatcher) emitOctetContinuation(buf []byte) ([]byte, int, bool) {
	chunk := m.octetRemaining
	if chunk > m.contentLenLimit {
		chunk = m.contentLenLimit
	}
	if len(buf) < chunk {
		return nil, 0, false
	}
	m.octetRemaining -= chunk
	if m.octetRemaining <= 0 {
		m.octetRemaining = 0
		m.overflow = overflowNone
	}
	// Truncated unless this chunk completed the declared body (the last
	// segment is never truncated).
	return buf[:chunk], chunk, m.overflow == overflowOctet
}

// findNonTransparent scans a fresh non-transparent frame for its LF/NUL
// delimiter, skipping the bytes already scanned on a prior call (seen).
func (m *syslogFrameMatcher) findNonTransparent(buf []byte, seen int) ([]byte, int, bool) {
	start := seen
	if start < 0 {
		start = 0
	}
	return m.scanNonTransparent(buf, start, false)
}

// scanNonTransparent finds the LF/NUL delimiter that terminates a
// non-transparent frame and emits the trailer-trimmed content (CR+LF and NUL
// stripped). A non-transparent frame's length is unknown until its delimiter
// arrives, so an oversized frame is emitted in bounded chunks: the matcher
// emits contentLenLimit bytes as soon as the buffer reaches the limit without a
// delimiter (rather than waiting and letting the Framer guard chop blindly),
// enters (or stays in) overflowNonTransparent, and the remainder returns here
// on the next call. start lets a fresh frame skip already-scanned bytes.
//
// The continuation flag distinguishes the first chunk of a fresh frame (counts
// the oversized frame once) from a continuation chunk (already counted, and a
// delimiter that lands exactly at the chunk boundary is consumed without
// emitting an empty frame). Truncation follows "never the last": a chunk
// emitted because the buffer filled before the delimiter arrived is truncated,
// while the chunk that reaches the delimiter (the final segment) is not.
func (m *syslogFrameMatcher) scanNonTransparent(buf []byte, start int, continuation bool) ([]byte, int, bool) {
	for i := start; i < len(buf); i++ {
		if buf[i] == '\n' || buf[i] == 0 {
			content := syslogTrimTrailer(buf[:i])
			if len(content) > m.contentLenLimit {
				// Delimiter is beyond this chunk; emit a bounded raw piece.
				if !continuation {
					m.recordOversized()
				}
				m.overflow = overflowNonTransparent
				return buf[:m.contentLenLimit], m.contentLenLimit, true
			}
			m.overflow = overflowNone
			if continuation && len(content) == 0 {
				// Prior continuation chunks already drained the whole body
				// (body length was an exact multiple of contentLenLimit), so
				// the delimiter now sits at the buffer head. Consume it without
				// emitting an empty frame; otherwise a blank log would be
				// forwarded.
				return nil, i + 1, false
			}
			// The delimiter terminates the frame, so this is its final
			// segment: never truncated ("never the last"). A whole sub-limit
			// frame likewise emits untruncated.
			return content, i + 1, false // include the delimiter
		}
	}
	// No delimiter yet. If the frame already fills a chunk, emit it now and
	// enter overflow rather than waiting.
	if len(buf) >= m.contentLenLimit {
		if !continuation {
			m.recordOversized()
		}
		m.overflow = overflowNonTransparent
		return buf[:m.contentLenLimit], m.contentLenLimit, true
	}
	return nil, 0, false
}

// FlushFrame implements FrameMatcher. At end-of-stream it emits whatever
// unframed bytes remain as a single final frame. The Framer only ever leaves a
// remainder smaller than contentLenLimit buffered (Process emits a bounded
// chunk the moment the buffer reaches the limit), so there is nothing to chunk
// here — flush is always a straight dump of the trailing bytes.
//
// The flushed bytes are always the terminal segment of their logical line, so
// the frame is never reported as truncated ("never the last"): if its frame
// was split during Process, the leading "...TRUNCATED..." marker is added
// downstream by the line handler's carry-over, and no trailing marker belongs
// on the final piece. Trailing LF/CR/NUL delimiters are stripped; a remainder
// that is only delimiters (or empty) emits nothing rather than a blank frame.
// Malformed-overflow bytes are counted as malformed so the telemetry total
// stays accurate.
func (m *syslogFrameMatcher) FlushFrame(buf []byte) ([]byte, int) {
	content := syslogTrimTrailer(buf)
	if len(content) == 0 {
		m.overflow = overflowNone
		return nil, 0
	}
	if m.overflow == overflowMalformed {
		m.recordMalformed(int64(len(content)))
	}
	m.overflow = overflowNone
	return content, len(buf)
}

// recordMalformed increments both the global telemetry counter and the
// per-tailer status counter for malformed bytes.
func (m *syslogFrameMatcher) recordMalformed(n int64) {
	metrics.TlmSyslogMalformedBytes.Add(float64(n))
	if m.malformedBytes != nil {
		m.malformedBytes.Add(n)
	}
}

// recordOversized increments both the global telemetry counter and the
// per-tailer status counter for oversized frame splits.
func (m *syslogFrameMatcher) recordOversized() {
	metrics.TlmSyslogOversizedFrames.Inc()
	if m.oversizedFrames != nil {
		m.oversizedFrames.Add(1)
	}
}

// NewSyslogFramer creates a Framer with RFC 6587 syslog framing and registers
// a "Syslog Malformed Bytes" counter in tailerInfo for status display.
func NewSyslogFramer(
	outputFn func(*message.Message, int),
	contentLenLimit int,
	tailerInfo *status.InfoRegistry,
) *Framer {
	malformedBytes := status.NewCountInfo("Syslog Malformed Bytes")
	tailerInfo.Register(malformedBytes)
	oversizedFrames := status.NewCountInfo("Syslog Oversized Frames")
	tailerInfo.Register(oversizedFrames)

	matcher := &syslogFrameMatcher{
		contentLenLimit: contentLenLimit,
		malformedBytes:  malformedBytes,
		oversizedFrames: oversizedFrames,
	}
	return newFramer(outputFn, matcher, contentLenLimit, false)
}

// syslogTrimTrailer removes trailing non-transparent frame delimiters
// (CR, LF, NUL) from b.
func syslogTrimTrailer(b []byte) []byte {
	n := len(b)
	if n > 0 && (b[n-1] == '\n' || b[n-1] == 0) {
		n--
	}
	if n > 0 && b[n-1] == '\r' {
		n--
	}
	return b[:n]
}
