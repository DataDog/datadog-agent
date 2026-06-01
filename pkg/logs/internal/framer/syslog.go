// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

import (
	"bytes"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
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

	discardedBytes  *status.CountInfo
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
		return m.emitNonTransparentContinuation(buf)
	case overflowMalformed:
		// If the malformed run ended exactly at the previous chunk boundary,
		// the next byte begins a real frame (or is a stray delimiter): clear
		// overflow and fall through to normal detection.
		if isSyslogFrameStart(buf, 0) || buf[0] == '\n' || buf[0] == '\r' || buf[0] == 0 {
			m.overflow = overflowNone
		} else {
			return m.emitMalformedContinuation(buf)
		}
	}

	b := buf[0]
	switch {
	case b >= '1' && b <= '9':
		return m.findOctetCounted(buf)

	case b == '<':
		return m.findNonTransparent(buf, seen)

	case b == '\n' || b == '\r' || b == 0:
		return buf[:0], 1, false

	default:
		return m.findMalformed(buf)
	}
}

// findMalformed handles bytes that don't start a valid syslog frame. It scans
// forward for the next probable frame start (PRI header or octet-counting
// prefix), newline, or NUL delimiter and emits everything before it as a
// single malformed frame. If no sync point is found and the buffer has not
// yet reached contentLenLimit, returns nil to wait for more data.
//
// When the malformed run exceeds contentLenLimit — or grows past the limit
// before any sync point is found — the matcher emits a bounded chunk, enters
// overflowMalformed, and the remainder is consumed by emitMalformedContinuation.
// Discarded bytes are counted per emitted chunk so a run is never double-counted.
func (m *syslogFrameMatcher) findMalformed(buf []byte) ([]byte, int, bool) {
	for i := 1; i < len(buf); i++ {
		if isSyslogFrameStart(buf, i) || buf[i] == '\n' || buf[i] == 0 {
			content := buf[:i]
			rawDataLen := i
			if buf[i] == '\n' || buf[i] == 0 {
				rawDataLen = i + 1
			}
			if len(content) > m.contentLenLimit {
				m.recordDiscarded(int64(m.contentLenLimit))
				m.recordOversized()
				m.overflow = overflowMalformed
				return buf[:m.contentLenLimit], m.contentLenLimit, true
			}
			m.recordDiscarded(int64(len(content)))
			return content, rawDataLen, false
		}
	}
	// No sync point yet. If the run already fills a chunk, emit it now and
	// enter overflow rather than waiting (which would let the Framer guard
	// chop blindly and re-detect the continuation).
	if len(buf) >= m.contentLenLimit {
		m.recordDiscarded(int64(m.contentLenLimit))
		m.recordOversized()
		m.overflow = overflowMalformed
		return buf[:m.contentLenLimit], m.contentLenLimit, true
	}
	return nil, 0, false
}

// emitMalformedContinuation consumes the continuation of an oversized malformed
// run. buf[0] is known to be malformed (the FindFrame dispatch cleared overflow
// otherwise), so it scans from index 1 for the next sync point or delimiter.
// All emitted chunks are flagged truncated; discarded bytes are counted per
// chunk so the total matches the run length exactly.
func (m *syslogFrameMatcher) emitMalformedContinuation(buf []byte) ([]byte, int, bool) {
	for i := 1; i < len(buf); i++ {
		if isSyslogFrameStart(buf, i) || buf[i] == '\n' || buf[i] == 0 {
			content := buf[:i]
			rawDataLen := i
			if buf[i] == '\n' || buf[i] == 0 {
				rawDataLen = i + 1
			}
			if len(content) > m.contentLenLimit {
				m.recordDiscarded(int64(m.contentLenLimit))
				return buf[:m.contentLenLimit], m.contentLenLimit, true
			}
			m.recordDiscarded(int64(len(content)))
			m.overflow = overflowNone
			return content, rawDataLen, true
		}
	}
	if len(buf) >= m.contentLenLimit {
		m.recordDiscarded(int64(m.contentLenLimit))
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
// content. When the declared body exceeds contentLenLimit, the matcher does
// NOT buffer the whole declared body (which could be arbitrarily large);
// instead it emits a bounded first chunk as soon as one is available, records
// the remainder in octetRemaining, and enters overflowOctet. MSG-LEN stays the
// authoritative boundary (RFC 6587 §3.4.1): the remaining declared bytes are
// emitted as raw continuation, never re-detected.
func (m *syslogFrameMatcher) findOctetCounted(buf []byte) ([]byte, int, bool) {
	// Parse the decimal length prefix.
	msgLen := 0
	i := 0
	for i < len(buf) {
		b := buf[i]
		if b == ' ' {
			i++ // consume the space
			break
		}
		if b < '0' || b > '9' {
			m.recordDiscarded(1)
			return buf[:0], 1, false
		}
		i++
		if i > 10 {
			m.recordDiscarded(1)
			return buf[:0], 1, false
		}
		msgLen = msgLen*10 + int(b-'0')
	}

	// If we consumed all of buf without finding SP, wait for more data.
	if i == len(buf) && (i == 0 || buf[i-1] != ' ') {
		return nil, 0, false
	}

	if msgLen == 0 {
		// "0 " is not a valid octet-counted frame — skip the prefix.
		return buf[:0], i, false
	}

	headerLen := i // digits + SP

	if msgLen > m.contentLenLimit {
		bodyAvail := len(buf) - headerLen
		emit := bodyAvail
		if emit > m.contentLenLimit {
			emit = m.contentLenLimit
		}
		// Wait only while we are still under the limit (bounded by the header
		// plus contentLenLimit). Once len(buf) reaches the limit we must emit
		// so the Framer guard never chops mid-frame.
		if emit < m.contentLenLimit && len(buf) < m.contentLenLimit {
			return nil, 0, false
		}
		m.recordOversized()
		m.octetRemaining = msgLen - emit
		m.overflow = overflowOctet
		return buf[headerLen : headerLen+emit], headerLen + emit, true
	}

	totalLen := headerLen + msgLen

	// Not oversized: wait for the full (bounded) body, then emit it whole.
	if len(buf) < totalLen {
		return nil, 0, false
	}

	return buf[headerLen:totalLen], totalLen, false
}

// emitOctetContinuation emits the next bounded chunk of an oversized
// octet-counted frame's remaining declared body. Continuation chunks carry no
// header and are flagged truncated. The matcher waits only for a single
// chunk's worth of bytes, so the buffer never exceeds contentLenLimit.
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
	return buf[:chunk], chunk, true
}

// findNonTransparent scans for a LF or NUL delimiter starting from seen.
// Trailing CR+LF and NUL are stripped from the returned content.
//
// A non-transparent frame's length is unknown until its delimiter is found,
// so an oversized frame is frequently detected before the delimiter arrives.
// To hold the memory boundary, the matcher emits a bounded chunk and enters
// overflowNonTransparent as soon as the buffer reaches contentLenLimit without
// a delimiter (rather than waiting and letting the Framer guard chop blindly).
// The remainder is consumed raw by emitNonTransparentContinuation up to the
// delimiter.
func (m *syslogFrameMatcher) findNonTransparent(buf []byte, seen int) ([]byte, int, bool) {
	start := seen
	if start < 0 {
		start = 0
	}

	for i := start; i < len(buf); i++ {
		if buf[i] == '\n' || buf[i] == 0 {
			content := syslogTrimTrailer(buf[:i])
			if len(content) > m.contentLenLimit {
				m.recordOversized()
				m.overflow = overflowNonTransparent
				return buf[:m.contentLenLimit], m.contentLenLimit, true
			}
			return content, i + 1, false // include the delimiter
		}
	}

	// No delimiter yet. If the frame already fills a chunk, emit it now and
	// enter overflow rather than waiting.
	if len(buf) >= m.contentLenLimit {
		m.recordOversized()
		m.overflow = overflowNonTransparent
		return buf[:m.contentLenLimit], m.contentLenLimit, true
	}
	return nil, 0, false
}

// emitNonTransparentContinuation consumes the continuation of an oversized
// non-transparent frame, scanning for the LF/NUL delimiter. Intermediate
// chunks are emitted raw; the final chunk (the one containing the delimiter)
// is trailer-trimmed, mirroring the normal path. All chunks are flagged
// truncated. The matcher emits at the chunk boundary even before the delimiter
// arrives, so the buffer never exceeds contentLenLimit.
func (m *syslogFrameMatcher) emitNonTransparentContinuation(buf []byte) ([]byte, int, bool) {
	for i := 0; i < len(buf); i++ {
		if buf[i] == '\n' || buf[i] == 0 {
			content := syslogTrimTrailer(buf[:i])
			if len(content) > m.contentLenLimit {
				// Delimiter is beyond this chunk; emit a bounded raw piece.
				return buf[:m.contentLenLimit], m.contentLenLimit, true
			}
			m.overflow = overflowNone
			return content, i + 1, true // include the delimiter
		}
	}
	if len(buf) >= m.contentLenLimit {
		return buf[:m.contentLenLimit], m.contentLenLimit, true
	}
	return nil, 0, false
}

// FlushFrame implements FrameMatcher. At end-of-stream, emit any remaining
// bytes in bounded chunks. The caller (Framer.Flush) loops until the buffer
// is drained. The returned wasTruncated flag is true when the chunk is part
// of an oversized frame — either an overflow in progress from FindFrame, or a
// remainder discovered to be oversized here.
func (m *syslogFrameMatcher) FlushFrame(buf []byte) ([]byte, int, bool) {
	if len(buf) == 0 {
		return nil, 0, false
	}

	// Drain an overflow that was in progress when the stream ended.
	switch m.overflow {
	case overflowOctet:
		chunk := m.octetRemaining
		if chunk > m.contentLenLimit {
			chunk = m.contentLenLimit
		}
		if chunk > len(buf) {
			chunk = len(buf) // sender under-sent the declared length
		}
		m.octetRemaining -= chunk
		if m.octetRemaining <= 0 {
			m.octetRemaining = 0
			m.overflow = overflowNone
		}
		if chunk == 0 {
			return nil, 0, false
		}
		return buf[:chunk], chunk, true
	case overflowNonTransparent, overflowMalformed:
		content := syslogTrimTrailer(buf)
		if len(content) > m.contentLenLimit {
			return buf[:m.contentLenLimit], m.contentLenLimit, true
		}
		m.overflow = overflowNone
		if len(content) == 0 {
			return nil, 0, false
		}
		return content, len(buf), true
	}

	// No active overflow: drain the trailing remainder, chunking if oversized.
	content := syslogTrimTrailer(buf)
	if len(content) == 0 {
		return nil, 0, false
	}
	if len(content) > m.contentLenLimit {
		m.recordOversized()
		m.overflow = overflowNonTransparent
		return content[:m.contentLenLimit], m.contentLenLimit, true
	}
	return content, len(buf), false
}

// recordDiscarded increments both the global telemetry counter and the
// per-tailer status counter for discarded bytes.
func (m *syslogFrameMatcher) recordDiscarded(n int64) {
	telemetry.GetStatsTelemetryProvider().Count("logs_syslog_framer.discarded_bytes", float64(n), nil)
	if m.discardedBytes != nil {
		m.discardedBytes.Add(n)
	}
}

// recordOversized increments both the global telemetry counter and the
// per-tailer status counter for oversized frame splits.
func (m *syslogFrameMatcher) recordOversized() {
	telemetry.GetStatsTelemetryProvider().Count("logs_syslog_framer.oversized_frames", 1, nil)
	if m.oversizedFrames != nil {
		m.oversizedFrames.Add(1)
	}
}

// NewSyslogFramer creates a Framer with RFC 6587 syslog framing and registers
// a "Syslog Discarded Bytes" counter in tailerInfo for status display.
func NewSyslogFramer(
	outputFn func(*message.Message, int),
	contentLenLimit int,
	tailerInfo *status.InfoRegistry,
) *Framer {
	discardedBytes := status.NewCountInfo("Syslog Discarded Bytes")
	tailerInfo.Register(discardedBytes)
	oversizedFrames := status.NewCountInfo("Syslog Oversized Frames")
	tailerInfo.Register(oversizedFrames)

	matcher := &syslogFrameMatcher{
		contentLenLimit: contentLenLimit,
		discardedBytes:  discardedBytes,
		oversizedFrames: oversizedFrames,
	}
	return &Framer{
		frames:          atomic.NewInt64(0),
		outputFn:        outputFn,
		matcher:         matcher,
		buffer:          bytes.Buffer{},
		contentLenLimit: contentLenLimit,
	}
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
