package decoder

import (
	"bytes"
	"sync/atomic"
)

// LineBreaker implements an actor which reads chunks of bytes from an input
// channel and uses and EndLineMatcher to break those into lines, passing the
// results to a LineParser.
//
// After Start(), the actor runs until its input channel is closed.
// After all inputs are processed, the actor closes its output channel.
type LineBreaker struct {
	// The number of raw lines decoded from the input before they are processed.
	// Needs to be first to ensure 64 bit alignment
	linesDecoded int64

	inputChan       chan *Input
	outputChan      chan *DecodedInput
	matcher         EndLineMatcher
	lineBuffer      *bytes.Buffer
	contentLenLimit int
	rawDataLen      int
}

// NewLineBreaker initializes a LineBreaker
func NewLineBreaker(inputChan chan *Input, outputChan chan *DecodedInput, matcher EndLineMatcher, contentLenLimit int) *LineBreaker {
	return &LineBreaker{
		linesDecoded:    0,
		inputChan:       inputChan,
		outputChan:      outputChan,
		matcher:         matcher,
		lineBuffer:      &bytes.Buffer{},
		contentLenLimit: contentLenLimit,
		rawDataLen:      0,
	}
}

// Start starts the LineBreaker
func (lb *LineBreaker) Start() {
	go lb.run()
}

// run lets the LineBreaker handle data coming from InputChan
func (lb *LineBreaker) run() {
	for data := range lb.inputChan {
		lb.breakIncomingData(data.content)
	}
	close(lb.outputChan)
}

// breakIncomingData splits raw data based on '\n', creates and processes new lines
func (lb *LineBreaker) breakIncomingData(inBuf []byte) {
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
	lb.outputChan <- NewDecodedInput(content, lb.rawDataLen)
	lb.rawDataLen = 0
	atomic.AddInt64(&lb.linesDecoded, 1)
}
