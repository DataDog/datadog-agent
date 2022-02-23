// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package breaker supports efficiently breaking chunks of binary data into lines.
package breaker

import (
	"bytes"
	"sync/atomic"
)

// LineBreaker gets chunks of bytes (via Process(..)) and uses an
// EndLineMatcher to break those into lines, passing the results to its
// outputFn.
type LineBreaker struct {
	// The number of raw lines decoded from the input before they are processed.
	// Needs to be first to ensure 64 bit alignment
	linesDecoded int64

	// outputFn is called with each complete "line"
	outputFn func(content []byte, rawDatatLen int)

	matcher         EndLineMatcher
	lineBuffer      *bytes.Buffer
	contentLenLimit int
	rawDataLen      int
}

// NewLineBreaker initializes a LineBreaker.
//
// The breaker will use the given matcher to match newlines.  Content longer
// than the given limit will be broken into lines at that length, regardless of
// matching a newline.  Each line will be passed to outputFn, including both
// the content of the line itself and the number of raw bytes matched to
// represent that line.
func NewLineBreaker(
	outputFn func(content []byte, rawDataLen int),
	matcher EndLineMatcher,
	contentLenLimit int,
) *LineBreaker {
	return &LineBreaker{
		linesDecoded:    0,
		outputFn:        outputFn,
		matcher:         matcher,
		lineBuffer:      &bytes.Buffer{},
		contentLenLimit: contentLenLimit,
		rawDataLen:      0,
	}
}

// GetLineCount gets the number of lines this breaker has processed.  This is safe to
// call from any goroutine.
func (lb *LineBreaker) GetLineCount() int64 {
	return atomic.LoadInt64(&lb.linesDecoded)
}

// Process handles an incoming chunk of data.  It will call outputFn for any recognized lines.  Partial
// lines are maintained between calls to Process.  The passed buffer is not used after return.
func (lb *LineBreaker) Process(inBuf []byte) {
	i, j := 0, 0
	n := len(inBuf)
	maxj := lb.contentLenLimit - lb.lineBuffer.Len()

	for ; j < n; j++ {
		if j == maxj {
			// send line because it is too long
			lb.lineBuffer.Write(inBuf[i:j])
			lb.rawDataLen += (j - i)
			lb.sendLine()
			i = j
			maxj = i + lb.contentLenLimit
		} else if lb.matcher.Match(lb.lineBuffer.Bytes(), inBuf, i, j) {
			lb.lineBuffer.Write(inBuf[i:j])
			lb.rawDataLen += (j - i)
			lb.rawDataLen++ // account for the matching byte
			lb.sendLine()
			i = j + 1 // skip the last bytes of the matched sequence
			maxj = i + lb.contentLenLimit
		}
	}
	lb.lineBuffer.Write(inBuf[i:j])
	lb.rawDataLen += (j - i)
}

// sendLine copies content from lineBuffer which is passed to lineHandler
func (lb *LineBreaker) sendLine() {
	// Account for longer-than-1-byte line separator
	content := make([]byte, lb.lineBuffer.Len()-(lb.matcher.SeparatorLen()-1))
	copy(content, lb.lineBuffer.Bytes())
	lb.lineBuffer.Reset()
	lb.outputFn(content, lb.rawDataLen)
	lb.rawDataLen = 0
	atomic.AddInt64(&lb.linesDecoded, 1)
}
