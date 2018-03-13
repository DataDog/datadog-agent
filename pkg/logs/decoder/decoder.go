// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package decoder

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// contentLenLimit represents the length limit above which we want to truncate the output content
var contentLenLimit = 256 * 1000

// Input represents a list of bytes consumed by the Decoder
type Input struct {
	content []byte
}

// NewInput returns a new input
func NewInput(content []byte) *Input {
	return &Input{content}
}

// Output represents a list of bytes produced by the Decoder
type Output struct {
	Content    []byte
	RawDataLen int
}

// NewOutput returns a new decoder output
func NewOutput(content []byte, rawDataLen int) *Output {
	return &Output{
		Content:    content,
		RawDataLen: rawDataLen,
	}
}

// Decoder splits raw data into lines and passes them to a lineHandler that emits outputs
type Decoder struct {
	InputChan  chan *Input
	OutputChan chan *Output

	lineBuffer  *bytes.Buffer
	lineHandler LineHandler
}

// InitializeDecoder returns a properly initialized Decoder
func InitializeDecoder(source *config.LogSource) *Decoder {
	inputChan := make(chan *Input)
	outputChan := make(chan *Output)

	var lineHandler LineHandler
	for _, rule := range source.Config.ProcessingRules {
		switch rule.Type {
		case config.MultiLine:
			var lineUnwrapper LineUnwrapper
			switch source.Config.Type {
			case config.DockerType:
				lineUnwrapper = NewDockerUnwrapper()
			default:
				lineUnwrapper = NewUnwrapper()
			}
			lineHandler = NewMultiLineHandler(outputChan, rule.Reg, defaultFlushTimeout, lineUnwrapper)
			break
		}
	}
	if lineHandler == nil {
		lineHandler = NewSingleLineHandler(outputChan)
	}

	return New(inputChan, outputChan, lineHandler)
}

// New returns an initialized Decoder
func New(InputChan chan *Input, OutputChan chan *Output, lineHandler LineHandler) *Decoder {
	var lineBuffer bytes.Buffer
	return &Decoder{
		InputChan:   InputChan,
		OutputChan:  OutputChan,
		lineBuffer:  &lineBuffer,
		lineHandler: lineHandler,
	}
}

// Start starts the Decoder
func (d *Decoder) Start() {
	d.lineHandler.Start()
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
	d.lineHandler.Stop()
}

// decodeIncomingData splits raw data based on '\n', creates and processes new lines
func (d *Decoder) decodeIncomingData(inBuf []byte) {
	i, j := 0, 0
	n := len(inBuf)
	maxj := contentLenLimit - d.lineBuffer.Len()

	for ; j < n; j++ {
		if j == maxj {
			// send line because it is too long
			d.lineBuffer.Write(inBuf[i:j])
			d.sendLine()
			i = j
			maxj = i + contentLenLimit
		} else if inBuf[j] == '\n' {
			d.lineBuffer.Write(inBuf[i:j])
			d.sendLine()
			i = j + 1 // +1 as we skip the `\n`
			maxj = i + contentLenLimit
		}
	}
	d.lineBuffer.Write(inBuf[i:j])
}

// sendLine copies content from lineBuffer which is passed to lineHandler
func (d *Decoder) sendLine() {
	content := make([]byte, d.lineBuffer.Len())
	copy(content, d.lineBuffer.Bytes())
	d.lineBuffer.Reset()
	d.lineHandler.Handle(content)
}
