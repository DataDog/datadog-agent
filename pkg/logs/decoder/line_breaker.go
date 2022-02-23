package decoder

import (
	"bytes"
	"sync/atomic"
)

// LineBreaker gets chunks of bytes (via process(..)) and uses an
// EndLineMatcher to break those into lines, passing the results to its
// outputFn.
type LineBreaker struct {
	// The number of raw lines decoded from the input before they are processed.
	// Needs to be first to ensure 64 bit alignment
	linesDecoded int64

	// outputFn is called with each complete "line"
	outputFn func(*DecodedInput)

	matcher         EndLineMatcher
	lineBuffer      *bytes.Buffer
	contentLenLimit int
	rawDataLen      int
}

// NewLineBreaker initializes a LineBreaker
func NewLineBreaker(outputFn func(*DecodedInput), matcher EndLineMatcher, contentLenLimit int) *LineBreaker {
	return &LineBreaker{
		linesDecoded:    0,
		outputFn:        outputFn,
		matcher:         matcher,
		lineBuffer:      &bytes.Buffer{},
		contentLenLimit: contentLenLimit,
		rawDataLen:      0,
	}
}

// process splits raw data based on '\n', creates and processes new lines
func (lb *LineBreaker) process(inBuf []byte) {
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
	lb.outputFn(NewDecodedInput(content, lb.rawDataLen))
	lb.rawDataLen = 0
	atomic.AddInt64(&lb.linesDecoded, 1)
}
