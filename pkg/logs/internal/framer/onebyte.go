// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

import "bytes"

// oneByteNewLineMatcher implements EndLineMatcher for lines ending with the
// byte 0x0a ('\n').  It is suitable for single-byte encodings and multibyte
// encodings where 0x0a never appears in a multibyte sequence.  This includes
// ASCII, UTF-8, and Shift-JIS.
type oneByteNewLineMatcher struct {
	// contentLenLimit is the maximum content length that will be returned.
	// Lines longer than this value will be split into multiple frames.
	contentLenLimit int

	// flushPartial, when true, causes FlushFrame to emit any remaining
	// buffered bytes as a final frame. Used by stream-oriented transports
	// (UTF8NewlineStream) where the connection close is a legitimate
	// boundary for the final message.
	flushPartial bool
}

// FlushFrame implements FrameMatcher. By default, partial newline-delimited
// lines are not emitted at end-of-stream. When flushPartial is set, the
// buffered remainder is emitted as the final frame. The Framer chops the
// buffer into contentLenLimit-sized frames during Process, so the remainder is
// always under the limit and is the terminal segment of its line — never
// truncated.
func (ob *oneByteNewLineMatcher) FlushFrame(buf []byte) ([]byte, int) {
	if !ob.flushPartial || len(buf) == 0 {
		return nil, 0
	}
	return buf, len(buf)
}

// FindFrame implements FrameMatcher#FindFrame.
func (ob *oneByteNewLineMatcher) FindFrame(buf []byte, seen int) ([]byte, int, bool) {
	nl := bytes.IndexByte(buf[seen:], '\n')
	if nl == -1 {
		return nil, 0, false
	}

	// limit the returned line to contentLenLimit bytes
	eol := nl + seen
	if eol > ob.contentLenLimit {
		// Newline found beyond the limit - truncate and mark as truncated
		return buf[:ob.contentLenLimit], ob.contentLenLimit, true
	}

	// return the content without the newline, but count the newline in the raw
	// length
	return buf[:eol], eol + 1, false
}
