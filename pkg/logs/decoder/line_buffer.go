// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"bytes"
)

// LineBuffer accumulates lines in buffer escaping all '\n'
// and accumulates the total number of bytes of all lines in line representation (line + '\n') in contentLen
// to form and forward outputs to outputChan
type LineBuffer struct {
	outputChan chan *Output
	buffer     *bytes.Buffer
	contentLen int
}

// NewLineBuffer returns a new LineBuffer
func NewLineBuffer(outputChan chan *Output) *LineBuffer {
	buffer := bytes.Buffer{}
	return &LineBuffer{
		outputChan: outputChan,
		buffer:     &buffer,
	}
}

// IsEmpty returns true if buffer does not contain any line
func (l *LineBuffer) IsEmpty() bool {
	return l.contentLen == 0
}

// Length returns the length of buffer which might be different from contentLen because of escaped '\n'
func (l *LineBuffer) Length() int {
	return l.buffer.Len()
}

// Add stores line in buffer
func (l *LineBuffer) Add(line *Line) {
	l.buffer.Write(line.content)
	l.contentLen += len(line.content) + 1 // add 1 for '\n'
}

// AddEndOfLine stores an escaped '\n' in buffer
func (l *LineBuffer) AddEndOfLine() {
	l.buffer.Write([]byte(`\n`))
}

// AddIncompleteLine stores a chunck of line in buff
func (l *LineBuffer) AddIncompleteLine(line *Line) {
	l.buffer.Write(line.content)
	l.contentLen += len(line.content)
}

// AddTruncate stores TRUNCATED in buffer
func (l *LineBuffer) AddTruncate(line *Line) {
	l.buffer.Write(TRUNCATED)
}

// Flush creates a new output from content in buffer and sends it to outputChan
func (l *LineBuffer) Flush() {
	defer l.reset()
	content := make([]byte, l.buffer.Len())
	copy(content, l.buffer.Bytes())
	if len(content) > 0 {
		output := NewOutput(content, l.contentLen)
		l.outputChan <- output
	}
}

// Stop forwards stop event to outputChan
func (l *LineBuffer) Stop() {
	l.outputChan <- newStopOutput()
}

// reset prepares buffer to receive new lines
func (l *LineBuffer) reset() {
	l.contentLen = 0
	l.buffer.Reset()
}
