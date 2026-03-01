// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

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
}

// FindFrame implements FrameMatcher. It looks for a complete syslog frame
// at the start of buf. The seen argument indicates how many bytes of buf
// were present on the last call (used to avoid rescanning).
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
		// Stray delimiter between frames — consume one byte, return empty
		// content. The Framer skips zero-length content (the parser never
		// sees it), so this effectively advances past inter-frame junk.
		return buf[:0], 1, false

	default:
		// Unexpected leading byte — consume it to avoid infinite loops.
		return buf[:0], 1, false
	}
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
			// Malformed length — skip this byte to avoid getting stuck.
			return buf[:0], 1, false
		}
		i++
		if i > 10 {
			// Length has too many digits — skip the first byte.
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
