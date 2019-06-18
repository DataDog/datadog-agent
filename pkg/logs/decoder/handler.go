// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import (
	"bytes"
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"regexp"
)

// Handler should replace LineHandler.
// Handler defines the methods for handling Lines and form a Output ready Line.
type Handler interface {
	Handle(line *RichLine)
	SendResult()
	Cleanup()
}

// SingleHandler treats the incoming Line independently and prepare the results
// for send.
type SingleHandler struct {
	truncator LineTruncator
}

// NewSingleHandler creates a new instance of SingleHandler.
func NewSingleHandler(truncator LineTruncator) *SingleHandler {
	return &SingleHandler{
		truncator: truncator,
	}
}

// Handle takes a line, truncates accordingly.
func (s *SingleHandler) Handle(line *RichLine) {
	s.truncator.truncate(line)
}

// Cleanup closes the downstream operations.
func (s *SingleHandler) Cleanup() {
	s.truncator.Close()
}

// SendResult does nothing since single line handler doesn't cache the lines.
func (s *SingleHandler) SendResult() {}

// MultiHandler handles multiline logs. It accumulates the multiline logs and
// send it to downstream.
type MultiHandler struct {
	truncator   LineTruncator
	newLogRegex *regexp.Regexp
	buffer      *multiLineBuffer
}

// NewMultiHandler creates a new instance of MultiHandler.
func NewMultiHandler(newLogRegex *regexp.Regexp, truncator LineTruncator) *MultiHandler {
	return &MultiHandler{
		newLogRegex: newLogRegex,
		truncator:   truncator,
		buffer:      newMultiLineBuffer(),
	}
}

// Handle checks line content prefix against a regex to see if it's a start of new log.
// It accumulates lines and merge as one log and then send to next handler.
// If the specified line requires leading or tailing truncation information, treat
// this line "specially" meaning sending them directly to down stream without buffering.
func (m *MultiHandler) Handle(line *RichLine) {
	if m.newLogRegex.Match(line.Content) {
		// it's the start of a new log, handle buffered content.
		m.SendResult()
	}
	m.buffer.write(line)
	// When leading or tailing truncation info is required, the line should be
	// part of large line splitted by upstream.
	// in case of:
	// msg...TRUNCATED... needTailing = true or
	// ...TRUNCATED...msg...TRUNCATED... needLeading = true && needTailing = true
	// both cases above suggest msg is at it's cap length, that no buffering is
	// required.
	if line.needTailing {
		m.SendResult()
	}
}

// Cleanup closes downstreams.
func (m *MultiHandler) Cleanup() {
	m.truncator.Close()
}

// SendResult sends the cached result to downstream.
func (m *MultiHandler) SendResult() {
	line := m.buffer.toLine()
	if line != nil {
		m.truncator.truncate(line)
	}
}

// multiLineBuffer
type multiLineBuffer struct {
	lineBuffer
}

func newMultiLineBuffer() *multiLineBuffer {
	var contentB bytes.Buffer
	return &multiLineBuffer{
		lineBuffer{
			contentBuf: &contentB,
		},
	}
}

// write appends the content of specified line to the end of buffer, if buffer is not empty,
// a `\n` will be append prior to the content of this line.
func (m *multiLineBuffer) write(line *RichLine) {
	if m.contentBuf.Len() > 0 {
		m.contentBuf.Write([]byte(`\n`))
	}
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

func (m *multiLineBuffer) reset() {
	m.lastLeading = false
	m.lastTailing = false
	m.contentBuf.Reset()
}

func (m *multiLineBuffer) toLine() *RichLine {
	defer m.reset()
	// leading and tailing space should be handled up stream.
	c := m.contentBuf.Bytes()
	content := make([]byte, len(c))
	copy(content, c)
	if len(content) <= 0 {
		return nil
	}
	// Not handling the case when after merge multiple lines, content length too large. Let
	// truncator do the job.
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

// LineTruncator truncates a large line into multiple small lines with
// shared Prefix. The results are then sent to outputChan for further
// operations.
type LineTruncator struct {
	outputChan chan *Output
	maxLen     int // max send length
}

// NewLineTruncator creates a new instance of LineTruncator.
func NewLineTruncator(outputChan chan *Output, maxLen int) *LineTruncator {
	return &LineTruncator{
		outputChan: outputChan,
		maxLen:     maxLen,
	}
}

// Close closes the output channel. This method should be called by the sender of
// outputChan, normally the Cleanup method of upstream.
func (l *LineTruncator) Close() {
	close(l.outputChan)
}

// truncate splits the large line to multiple smaller size lines with the same prefix
// and send them to outputChan.
func (l *LineTruncator) truncate(line *RichLine) {
	if l.isLarge(line) {
		lines := l.split(line)
		for _, ln := range lines {
			l.send(ln)
		}
	} else {
		l.send(l.completeContent(line))
	}
}

// split splits the content of a large line to multiple partial contents. The result for
// a line with content "M1M2M3" will be splitted to multiple lines with contents:
// ["M1...TRUNCATED...", "...TRUNCATED...M2...TRUNCATED...", "...TRUNCATED...M3"]
// The first and last content could add leading or tailing "...TRUNCATED..." if specified
// from RichLine.needLeading or RichLine.needTailing.
func (l *LineTruncator) split(line *RichLine) []*parser.Line {
	numOfCompleteLines := line.Size / l.maxLen
	leftOver := line.Size % l.maxLen // the end part which is less than maxLen.
	totalNumOfLines := numOfCompleteLines
	if leftOver > 0 {
		totalNumOfLines++ // include the leftovers.
	}
	lines := make([]*parser.Line, totalNumOfLines)
	if totalNumOfLines <= 0 {
		return lines
	}
	i := 0
	for ; i < totalNumOfLines; i++ {
		start := i * l.maxLen
		end := min((i+1)*l.maxLen, line.Size)
		content := copyContent(line, start, end)
		lines[i] = &parser.Line{
			Prefix:  line.Prefix,
			Content: append(append(truncatedString, content...), truncatedString...),
			Size:    end - start,
		}
	}

	// check first line to apply leading truncation information
	firstLine := lines[0]
	if !line.needLeading {
		firstLine.Content = firstLine.Content[truncatedStringLen:]
	}
	// check last line to apply tailing truncation information
	lastLine := lines[len(lines)-1]
	if !line.needTailing {
		actualLength := len(lastLine.Content)
		lastLine.Content = lastLine.Content[:actualLength-truncatedStringLen]
	}
	return lines
}

func (l *LineTruncator) send(line *parser.Line) {
	if line != nil && line.Size > 0 {
		l.outputChan <- NewOutput(line.Content, line.Status, line.Size, line.Timestamp)
	}
}

func (l *LineTruncator) completeContent(line *RichLine) *parser.Line {
	content := line.Content
	if line.needLeading {
		content = append(truncatedString, content...)
	}
	if line.needTailing {
		content = append(content, truncatedString...)
	}
	return &parser.Line{
		Prefix:  line.Prefix,
		Content: content,
		Size:    line.Size,
	}
}

func (l *LineTruncator) isLarge(line *RichLine) bool {
	return line.Size > l.maxLen
}

func copyContent(line *RichLine, start int, end int) []byte {
	content := make([]byte, end-start)
	copy(content, line.Content[start:end])
	return content
}

func min(a int, b int) int {
	if a > b {
		return b
	}
	return a
}

var truncatedString = []byte("...TRUNCATED...")
var truncatedStringLen = len(truncatedString)
