// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import (
	"bytes"
)

// LineBuffer accumulates lines in buffer escaping all '\n'
// and accumulates the total number of bytes of all lines in line representation (line + '\n') in rawDataLen
type LineBuffer struct {
	buffer     *bytes.Buffer
	rawDataLen int
}

// NewLineBuffer returns a new LineBuffer
func NewLineBuffer() *LineBuffer {
	return &LineBuffer{
		buffer: &bytes.Buffer{},
	}
}

// IsEmpty returns true if buffer does not contain any line
func (l *LineBuffer) IsEmpty() bool {
	return l.rawDataLen == 0
}

// Length returns the length of buffer which might be different from rawDataLen because of escaped '\n'
func (l *LineBuffer) Length() int {
	return l.buffer.Len()
}

// Add stores line in buffer
func (l *LineBuffer) Add(line []byte) {
	l.buffer.Write(line)
	l.rawDataLen += len(line) + 1 // add 1 for '\n'
}

// AddEndOfLine stores an escaped '\n' in buffer
func (l *LineBuffer) AddEndOfLine() {
	l.buffer.Write([]byte(`\n`))
}

// AddIncompleteLine stores a chunck of line in buff
func (l *LineBuffer) AddIncompleteLine(line []byte) {
	l.buffer.Write(line)
	l.rawDataLen += len(line)
}

// AddTruncate stores TRUNCATED in buffer
func (l *LineBuffer) AddTruncate(line []byte) {
	l.buffer.Write(truncatedFlag)
}

// Content returns the content in buffer and the length of the data that enabled to compute this content
func (l *LineBuffer) Content() ([]byte, int) {
	content := make([]byte, l.buffer.Len())
	copy(content, l.buffer.Bytes())
	return content, l.rawDataLen
}

// Reset prepares buffer to receive new lines
func (l *LineBuffer) Reset() {
	l.rawDataLen = 0
	l.buffer.Reset()
}
