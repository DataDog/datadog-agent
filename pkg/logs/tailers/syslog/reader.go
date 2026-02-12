// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

// Two framing methods are supported with automatic per-message detection:
//
//   - Octet Counting (RFC 6587 §3.4.1 / RFC 5425):
//     SYSLOG-FRAME = MSG-LEN SP SYSLOG-MSG
//     The sender prefixes each message with its byte length as ASCII digits.
//
//   - Non-Transparent Framing (RFC 6587 §3.4.2):
//     Messages are terminated by a TRAILER character. Two trailer types are
//     supported:
//       - LF (%d10) — the standard trailer per RFC 6587
//       - NUL (%d00) — a common legacy alternative noted in RFC 6587 §3.4.2
//     Trailing LF, CR+LF, and NUL are stripped from the returned frame.
//
// Detection: peek at the first byte of each frame — if it is a digit ('1'-'9'),
// octet counting is assumed; if it is '<' (start of PRI), non-transparent framing
// is assumed. This heuristic works because RFC 5424 messages always start with '<'
// and octet-count lengths always start with a nonzero digit.
//
// The returned frame slice from ReadFrame is valid until the next ReadFrame call.
// It references the internal bufio buffer (zero-copy) when possible.

import (
	"bufio"
	"errors"
	"io"
)

// Maximum message size we'll accept (64 KiB).
const maxMsgLen = 64 * 1024

// Pre-allocated errors.
var (
	errFrameEmpty     = errors.New("framing: empty frame")
	errBadOctetLen    = errors.New("framing: invalid octet-count length")
	errOctetTooLong   = errors.New("framing: octet-count length exceeds maximum")
	errOctetNoSP      = errors.New("framing: expected SP after octet-count length")
	errOctetShort     = errors.New("framing: message shorter than octet-count length")
	errUnexpectedByte = errors.New("framing: unexpected leading byte (not '<' or digit)")
	errLineTooLong    = errors.New("framing: non-transparent line exceeds buffer")
)

// Reader reads syslog frames from a TCP stream, auto-detecting the framing
// method for each message. It is not safe for concurrent use.
//
// The returned frame from ReadFrame references internal buffers and is only
// valid until the next call to ReadFrame.
type Reader struct {
	br  *bufio.Reader
	buf []byte // fallback buffer for octet-counted messages that don't fit in bufio peek
}

// NewReader wraps an io.Reader (typically a net.Conn) for syslog frame reading.
func NewReader(r io.Reader) *Reader {
	return &Reader{
		br: bufio.NewReaderSize(r, maxMsgLen),
	}
}

// ReadFrame reads the next syslog message from the stream.
// The returned slice is valid until the next call to ReadFrame.
//
// Returns io.EOF when the stream is closed cleanly.
func (fr *Reader) ReadFrame() ([]byte, error) {
	for {
		// Peek at the first byte to decide framing method.
		peek, err := fr.br.Peek(1)
		if err != nil {
			return nil, err // io.EOF or read error
		}

		b := peek[0]
		switch {
		case b >= '1' && b <= '9':
			return fr.readOctetCounted()

		case b == '<':
			return fr.readNonTransparent()

		case b == '\n' || b == '\r' || b == 0:
			// Stray newline or NUL between frames — skip it.
			fr.br.ReadByte() //nolint:errcheck
			continue

		default:
			fr.br.ReadByte() //nolint:errcheck
			return nil, errUnexpectedByte
		}
	}
}

// readOctetCounted reads MSG-LEN SP SYSLOG-MSG.
// MSG-LEN = NONZERO-DIGIT *DIGIT (decimal byte count of SYSLOG-MSG).
//
// Uses Peek to return a zero-copy slice when the message fits in the bufio
// buffer; falls back to a reusable scratch buffer otherwise.
func (fr *Reader) readOctetCounted() ([]byte, error) {
	// Read the length prefix: digits then SP.
	msgLen := 0
	digits := 0
	for {
		b, err := fr.br.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == ' ' {
			break
		}
		if b < '0' || b > '9' {
			return nil, errBadOctetLen
		}
		digits++
		if digits > 10 {
			return nil, errOctetTooLong
		}
		msgLen = msgLen*10 + int(b-'0')
	}
	if digits == 0 {
		return nil, errOctetNoSP
	}
	if msgLen == 0 {
		return nil, errFrameEmpty
	}
	if msgLen > maxMsgLen {
		return nil, errOctetTooLong
	}

	// Fast path: try Peek to get a zero-copy view of the message directly
	// from the bufio buffer. This avoids allocating or copying into fr.buf.
	if peeked, err := fr.br.Peek(msgLen); err == nil && len(peeked) == msgLen {
		// Advance the reader past the peeked bytes.
		fr.br.Discard(msgLen) //nolint:errcheck
		return peeked, nil
	}

	// Slow path: message spans bufio boundaries; read into scratch buffer.
	if cap(fr.buf) < msgLen {
		fr.buf = make([]byte, msgLen)
	}
	fr.buf = fr.buf[:msgLen]
	_, err := io.ReadFull(fr.br, fr.buf)
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, errOctetShort
		}
		return nil, err
	}
	return fr.buf, nil
}

// readNonTransparent reads a non-transparent-framed syslog message.
//
// The frame is terminated by LF (%d10) or NUL (%d00) per RFC 6587 §3.4.2.
// Scans the bufio buffer for the first occurrence of either delimiter using
// Peek (non-consuming), then Discards up to and including the delimiter.
// The returned slice references the internal bufio buffer (zero-copy) and
// is valid until the next ReadFrame call.
func (fr *Reader) readNonTransparent() ([]byte, error) {
	searched := 0
	for {
		// Trigger a fill if the buffer doesn't have data beyond what we've scanned.
		// Peek(searched+1) requests one more byte, which causes bufio to read a
		// full chunk from the underlying reader (typically filling most of the 64KB buffer).
		_, peekErr := fr.br.Peek(searched + 1)

		// Peek(Buffered()) returns ALL buffered data without triggering another fill.
		// This lets us scan the full buffer in one pass instead of byte-by-byte.
		buf, _ := fr.br.Peek(fr.br.Buffered())

		// Scan new bytes for LF or NUL delimiter.
		for i := searched; i < len(buf); i++ {
			if buf[i] == '\n' || buf[i] == 0 {
				frame := trimTrailer(buf[:i])
				fr.br.Discard(i + 1) //nolint:errcheck
				if len(frame) == 0 {
					return nil, errFrameEmpty
				}
				return frame, nil
			}
		}
		searched = len(buf)

		if peekErr != nil {
			if peekErr == bufio.ErrBufferFull || searched >= maxMsgLen {
				return nil, errLineTooLong
			}
			// EOF or I/O error. Return any remaining data as a final partial frame.
			if searched > 0 {
				frame := trimTrailer(buf)
				fr.br.Discard(searched) //nolint:errcheck
				if len(frame) == 0 {
					if peekErr == io.EOF {
						return nil, io.EOF
					}
					return nil, errFrameEmpty
				}
				return frame, nil
			}
			return nil, peekErr
		}
	}
}

// trimTrailer removes trailing non-transparent frame delimiters from b.
// Handles LF, CR+LF, and NUL trailers.
func trimTrailer(b []byte) []byte {
	n := len(b)
	if n > 0 && (b[n-1] == '\n' || b[n-1] == 0) {
		n--
	}
	if n > 0 && b[n-1] == '\r' {
		n--
	}
	return b[:n]
}
