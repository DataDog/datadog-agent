// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

import (
	"bytes"
)

var (
	// Utf16leEOL is the bytes sequence for UTF-16 Little-Endian end-of-line char
	Utf16leEOL = []byte{'\n', 0x00}
	// Utf16beEOL is the bytes sequence for UTF-16 Big-Endian end-of-line char
	Utf16beEOL = []byte{0x00, '\n'}
)

// twoByteNewLineMatcher implements EndLineMatcher for lines ending with a given
// two-byte-aligned two-byte sequence.  It is suitable for two-byte encodings such
// as UTF-16.
type twoByteNewLineMatcher struct {
	// contentLenLimit is the maximum content length that will be returned.
	// Lines longer than this value will be split into multiple frames.  This
	// value will be rounded down to a multiple of 2.
	contentLenLimit int

	// newline is the newline character being matched
	newline []byte
}

// FindFrame implements EndLineMatcher#FindFrame.
func (tb *twoByteNewLineMatcher) FindFrame(buf []byte, seen int) ([]byte, int) {
	var nl int

	// round `seen` down to a 2-byte boundary
	seen &= ^0x1

	for {
		nl = bytes.Index(buf[seen:], tb.newline)
		if nl == -1 {
			return nil, 0
		}

		// check alignment of the match
		if (seen+nl)&0x1 == 0 {
			break
		}
		// advance past this "false match"
		seen += nl + 1
		continue
	}

	// limit the returned line to contentLenLimit bytes
	eol := nl + seen
	cll := tb.contentLenLimit & ^0x1
	if eol > cll {
		return buf[:cll], cll
	}

	// return the content without the newline, but count the newline sequence
	// in the raw length
	return buf[:eol], eol + 2
}
