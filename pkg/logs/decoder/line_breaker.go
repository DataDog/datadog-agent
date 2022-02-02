package decoder

import (
	"bytes"
	"sync/atomic"
)

// LineBreaker implements an actor which reads chunks of bytes from an input
// channel and uses and EndLineMatcher to break those into lines, passing the
// results to a LineParser.
type LineBreaker struct {
	// The number of raw lines decoded from the input before they are processed.
	// Needs to be first to ensure 64 bit alignment
	linesDecoded int64

	inputChan       chan *Input
	matcher         EndLineMatcher
	lineBuffer      *bytes.Buffer
	lineParser      LineParser
	contentLenLimit int
	rawDataLen      int
}

// NewLineBreaker initializes a LineBreaker
func NewLineBreaker(inputChan chan *Input, matcher EndLineMatcher, lineParser LineParser, contentLenLimit int) *LineBreaker {
	return &LineBreaker{
		linesDecoded:    0,
		inputChan:       inputChan,
		matcher:         matcher,
		lineBuffer:      &bytes.Buffer{},
		lineParser:      lineParser,
		contentLenLimit: contentLenLimit,
		rawDataLen:      0,
	}
}

// Start starts the LineBreaker
func (d *LineBreaker) Start() {
	d.lineParser.Start()
	go d.run()
}

// Stop stops the LineBreaker
func (d *LineBreaker) Stop() {
	close(d.inputChan)
}

// run lets the LineBreaker handle data coming from InputChan
func (d *LineBreaker) run() {
	for data := range d.inputChan {
		d.breakIncomingData(data.content)
	}
	d.lineParser.Stop()
}

// breakIncomingData splits raw data based on '\n', creates and processes new lines
func (d *LineBreaker) breakIncomingData(inBuf []byte) {
	i, j := 0, 0
	n := len(inBuf)
	maxj := d.contentLenLimit - d.lineBuffer.Len()

	for ; j < n; j++ {
		if j == maxj {
			// send line because it is too long
			d.lineBuffer.Write(inBuf[i:j])
			d.rawDataLen += (j - i)
			d.sendLine()
			i = j
			maxj = i + d.contentLenLimit
		} else if d.matcher.Match(d.lineBuffer.Bytes(), inBuf, i, j) {
			d.lineBuffer.Write(inBuf[i:j])
			d.rawDataLen += (j - i)
			d.rawDataLen++ // account for the matching byte
			d.sendLine()
			i = j + 1 // skip the last bytes of the matched sequence
			maxj = i + d.contentLenLimit
		}
	}
	d.lineBuffer.Write(inBuf[i:j])
	d.rawDataLen += (j - i)
}

// sendLine copies content from lineBuffer which is passed to lineHandler
func (d *LineBreaker) sendLine() {
	// Account for longer-than-1-byte line separator
	content := make([]byte, d.lineBuffer.Len()-(d.matcher.SeparatorLen()-1))
	copy(content, d.lineBuffer.Bytes())
	d.lineBuffer.Reset()
	d.lineParser.Handle(NewDecodedInput(content, d.rawDataLen))
	d.rawDataLen = 0
	atomic.AddInt64(&d.linesDecoded, 1)
}
