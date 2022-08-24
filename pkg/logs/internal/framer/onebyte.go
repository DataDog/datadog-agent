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
}

// FindFrame implements EndLineMatcher#FindFrame.
func (ob *oneByteNewLineMatcher) FindFrame(buf []byte, seen int) ([]byte, int) {
	nl := bytes.IndexByte(buf[seen:], '\n')
	if nl == -1 {
		return nil, 0
	}

	// limit the returned line to contentLenLimit bytes
	eol := nl + seen
	if eol > ob.contentLenLimit {
		return buf[:ob.contentLenLimit], ob.contentLenLimit
	}

	// return the content without the newline, but count the newline in the raw
	// length
	return buf[:eol], eol + 1
}
