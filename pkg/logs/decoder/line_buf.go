// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import (
	"bytes"
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
)

// lineBuffer defines the fields keep tracking the status of each transition in decoding
// process.
type lineBuffer struct {
	// lastLeading is the needLeading status from the previous line.
	lastLeading bool
	// lastTailing is the needTailing status from the previous line.
	lastTailing bool
	// contentBuf keeps the contents which can not be sent to the next pipe.
	contentBuf *bytes.Buffer
	// lastPrefix is the Prefix of previous line.
	lastPrefix parser.Prefix
}

func newLineBuffer() *lineBuffer {
	var contentB bytes.Buffer
	return &lineBuffer{
		contentBuf: &contentB,
	}
}

func (l *lineBuffer) appendContent(content []byte) {
	l.contentBuf.Write(content)
}

func (l *lineBuffer) contentBytes() []byte {
	return l.contentBuf.Bytes()
}

func (l *lineBuffer) len() int {
	return l.contentBuf.Len()
}

func (l *lineBuffer) reset() {
	l.contentBuf.Reset()
}

func (l *lineBuffer) copyContent() []byte {
	c := l.contentBuf.Bytes()
	finalC := make([]byte, len(c))
	copy(finalC, c)
	return finalC
}

// IsFull indicates if the buffer should receive more content based on the metadata
// lastTailing.
func (l *lineBuffer) IsFull() bool {
	return l.lastTailing
}

// MultiLineBuffer encapsulates lineBuffer and provide more multiline specific buffering
// functions.
type MultiLineBuffer struct {
	lineBuffer
}

// NewMultiLineBuffer creates a new instance of MultiLineBuffer
func NewMultiLineBuffer() *MultiLineBuffer {
	return &MultiLineBuffer{
		*newLineBuffer(),
	}
}

// Append appends a specified delimiter to buffered content.
func (m *MultiLineBuffer) Append(delimiter string) {
	m.contentBuf.WriteString(delimiter)
}

// Write appends the specified line to buffered content and update the metadata status.
func (m *MultiLineBuffer) Write(line *RichLine) {
	m.contentBuf.Write(line.Content)
	// update extra information
	m.lastPrefix = line.Prefix

	// m.lastLeading true means the cached content is the last piece of the multiline log, this
	// incoming line is a non-first line of multiline log.
	if !m.lastLeading {
		m.lastLeading = line.needLeading
	}
	// m.lastTailing is replaced directly by the incoming line's flag.
	m.lastTailing = line.needTailing
}

// ToLine forms a RichLine from the buffered content and kept metadata. Note that this
// method doesn't reset buffer. Call Reset for this purpose.
func (m *MultiLineBuffer) ToLine() *RichLine {
	content := m.copyContent()
	if len(content) <= 0 {
		return nil
	}
	return &RichLine{
		Line: parser.Line{
			Prefix:  m.lastPrefix,
			Content: content,
			Size:    len(content),
		},
		needTailing: m.lastTailing,
		needLeading: m.lastLeading,
	}
}

// Reset resets buffer and metadata.
func (m *MultiLineBuffer) Reset() {
	m.lastLeading = false
	m.lastTailing = false
	m.reset()
}
