// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

import (
	"bytes"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var tlmSyslogDiscardedBytes = telemetry.NewCounter(
	"logs_syslog_framer", "discarded_bytes",
	nil, "Bytes discarded by the syslog framer due to non-conformant leading bytes",
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
type syslogFrameMatcher struct {
	contentLenLimit int
	discardedBytes  *status.CountInfo
}

// FindFrame implements FrameMatcher. It looks for a complete syslog frame
// at the start of buf. The seen argument indicates how many bytes of buf
// were present on the last call (used to avoid rescanning).
//
// When the leading byte is not a valid syslog frame start ('<' or digit),
// the matcher scans forward for the next probable PRI header — the two-byte
// sequence <[0-9]. Everything before that sync point is emitted as a single
// malformed frame so the downstream parser can log it coherently rather than
// producing one empty message per byte.
func (m *syslogFrameMatcher) FindFrame(buf []byte, seen int) ([]byte, int, bool) {
	if len(buf) == 0 {
		return nil, 0, false
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
// forward for the next probable PRI header (<[0-9]) or newline delimiter and
// emits everything before it as a single malformed frame. If no sync point is
// found, returns nil to wait for more data.
func (m *syslogFrameMatcher) findMalformed(buf []byte) ([]byte, int, bool) {
	for i := 1; i < len(buf); i++ {
		if isSyslogFrameStart(buf, i) || buf[i] == '\n' || buf[i] == 0 {
			content := buf[:i]
			rawDataLen := i
			if buf[i] == '\n' || buf[i] == 0 {
				rawDataLen = i + 1
			}
			m.recordDiscarded(int64(len(content)))
			wasTruncated := false
			if len(content) > m.contentLenLimit {
				content = content[:m.contentLenLimit]
				wasTruncated = true
			}
			return content, rawDataLen, wasTruncated
		}
	}
	return nil, 0, false
}

// isSyslogFrameStart returns true if buf[i] looks like the start of a valid
// syslog frame: either a digit (octet counting) or '<' followed by a digit
// (PRI header). The two-byte check for '<' avoids false positives on stray
// angle brackets in text.
func isSyslogFrameStart(buf []byte, i int) bool {
	b := buf[i]
	if b >= '1' && b <= '9' {
		return true
	}
	if b == '<' && i+1 < len(buf) && buf[i+1] >= '0' && buf[i+1] <= '9' {
		return true
	}
	return false
}

// findOctetCounted parses MSG-LEN SP SYSLOG-MSG from the beginning of buf.
// Returns nil if the buffer does not yet contain a complete frame.
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
	totalLen := headerLen + msgLen

	// Not enough data yet for the full message body.
	if len(buf) < totalLen {
		return nil, 0, false
	}

	content := buf[headerLen:totalLen]
	wasTruncated := false
	if len(content) > m.contentLenLimit {
		content = content[:m.contentLenLimit]
		wasTruncated = true
	}

	return content, totalLen, wasTruncated
}

// findNonTransparent scans for a LF or NUL delimiter starting from seen.
// Trailing CR+LF and NUL are stripped from the returned content.
func (m *syslogFrameMatcher) findNonTransparent(buf []byte, seen int) ([]byte, int, bool) {
	start := seen
	if start < 0 {
		start = 0
	}

	for i := start; i < len(buf); i++ {
		if buf[i] == '\n' || buf[i] == 0 {
			content := syslogTrimTrailer(buf[:i])
			rawDataLen := i + 1 // include the delimiter

			wasTruncated := false
			if len(content) > m.contentLenLimit {
				content = content[:m.contentLenLimit]
				wasTruncated = true
			}

			return content, rawDataLen, wasTruncated
		}
	}

	// No delimiter found yet — wait for more data.
	return nil, 0, false
}

// FlushFrame implements FrameMatcher. At end-of-stream, emit the buffer if it
// looks like a non-transparent frame (starts with '<') whose trailing delimiter
// was never sent. Partial octet-counted frames are genuinely incomplete, so
// those are discarded.
func (m *syslogFrameMatcher) FlushFrame(buf []byte) ([]byte, int) {
	if len(buf) == 0 {
		return nil, 0
	}
	if buf[0] != '<' {
		return nil, 0
	}
	content := syslogTrimTrailer(buf)
	if len(content) == 0 {
		return nil, 0
	}
	return content, len(buf)
}

// recordDiscarded increments both the global telemetry counter and the
// per-tailer status counter for discarded bytes.
func (m *syslogFrameMatcher) recordDiscarded(n int64) {
	tlmSyslogDiscardedBytes.Add(float64(n))
	if m.discardedBytes != nil {
		m.discardedBytes.Add(n)
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

	matcher := &syslogFrameMatcher{
		contentLenLimit: contentLenLimit,
		discardedBytes:  discardedBytes,
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
