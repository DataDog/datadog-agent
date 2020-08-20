// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package decoder

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
)

// defaultContentLenLimit represents the max size for a line,
// if a line is bigger than this limit, it will be truncated.
const defaultContentLenLimit = 256 * 1000

// Input represents a chunk of line.
type Input struct {
	content []byte
}

// NewInput returns a new input.
func NewInput(content []byte) *Input {
	return &Input{
		content: content,
	}
}

// DecodedInput represents a decoded line and the raw length
type DecodedInput struct {
	content    []byte
	rawDataLen int
}

// NewDecodedInput returns a new decoded input.
func NewDecodedInput(content []byte, rawDataLen int) *DecodedInput {
	return &DecodedInput{
		content:    content,
		rawDataLen: rawDataLen,
	}
}

// Message represents a structured line.
type Message struct {
	Content    []byte
	Status     string
	RawDataLen int
	Timestamp  string
}

// NewMessage returns a new output.
func NewMessage(content []byte, status string, rawDataLen int, timestamp string) *Message {
	return &Message{
		Content:    content,
		Status:     status,
		RawDataLen: rawDataLen,
		Timestamp:  timestamp,
	}
}

// Decoder splits raw data into lines and passes them to a lineParser that passes them to
// a lineHandler that emits outputs
// Input->[decoder]->[parser]->[handler]->Output
type Decoder struct {
	InputChan       chan *Input
	OutputChan      chan *Message
	matcher         EndLineMatcher
	lineBuffer      *bytes.Buffer
	lineParser      LineParser
	contentLenLimit int
	rawDataLen      int
}

// InitializeDecoder returns a properly initialized Decoder
func InitializeDecoder(source *config.LogSource, parser parser.Parser) *Decoder {
	return NewDecoderWithEndLineMatcher(source, parser, &newLineMatcher{})
}

// NewDecoderWithEndLineMatcher initialize a decoder with given endline strategy.
func NewDecoderWithEndLineMatcher(source *config.LogSource, parser parser.Parser, matcher EndLineMatcher) *Decoder {
	inputChan := make(chan *Input)
	outputChan := make(chan *Message)
	lineLimit := defaultContentLenLimit
	var lineHandler LineHandler
	var lineParser LineParser

	for _, rule := range source.Config.ProcessingRules {
		if rule.Type == config.MultiLine {
			lineHandler = NewMultiLineHandler(outputChan, rule.Regex, defaultFlushTimeout, lineLimit)
		}
	}
	if lineHandler == nil {
		lineHandler = NewSingleLineHandler(outputChan, lineLimit)
	}

	if parser.SupportsPartialLine() {
		lineParser = NewMultiLineParser(defaultFlushTimeout, parser, lineHandler, lineLimit)
	} else {
		lineParser = NewSingleLineParser(parser, lineHandler)
	}

	return New(inputChan, outputChan, lineParser, lineLimit, matcher)
}

// New returns an initialized Decoder
func New(InputChan chan *Input, OutputChan chan *Message, lineParser LineParser, contentLenLimit int, matcher EndLineMatcher) *Decoder {
	var lineBuffer bytes.Buffer
	return &Decoder{
		InputChan:       InputChan,
		OutputChan:      OutputChan,
		lineBuffer:      &lineBuffer,
		lineParser:      lineParser,
		contentLenLimit: contentLenLimit,
		matcher:         matcher,
	}
}

// Start starts the Decoder
func (d *Decoder) Start() {
	d.lineParser.Start()
	go d.run()
}

// Stop stops the Decoder
func (d *Decoder) Stop() {
	close(d.InputChan)
}

// run lets the Decoder handle data coming from InputChan
func (d *Decoder) run() {
	for data := range d.InputChan {
		d.decodeIncomingData(data.content)
	}
	// finish to stop decoder
	d.lineParser.Stop()
}

// decodeIncomingData splits raw data based on '\n', creates and processes new lines
func (d *Decoder) decodeIncomingData(inBuf []byte) {
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
			i = j + 1 // skip the matching byte.
			maxj = i + d.contentLenLimit
		}
	}
	d.lineBuffer.Write(inBuf[i:j])
	d.rawDataLen += (j - i)
}

// sendLine copies content from lineBuffer which is passed to lineHandler
func (d *Decoder) sendLine() {
	content := make([]byte, d.lineBuffer.Len())
	copy(content, d.lineBuffer.Bytes())
	d.lineBuffer.Reset()
	d.lineParser.Handle(NewDecodedInput(content, d.rawDataLen))
	d.rawDataLen = 0
}
