// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import "github.com/DataDog/datadog-agent/pkg/logs/parser"

// Handler should replace LineHandler.
type Handler interface {
	Handle(line *RichLine)
	SendResult()
	Cleanup()
}

type SingleHandler struct {
	truncator LineTruncator
}

func NewSingleHandler(truncator LineTruncator) *SingleHandler {
	return &SingleHandler{
		truncator: truncator,
	}
}

func (s *SingleHandler) Handle(line *RichLine) {
	s.truncator.truncate(line)
}

func (s *SingleHandler) Cleanup() {
	s.truncator.Close()
}

// SendResult does nothing since single line handler doesn't cache the lines.
func (s *SingleHandler) SendResult() {}

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
		totalNumOfLines += 1 // include the leftovers.
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
	} else {
		return a
	}
}

var truncatedString = []byte("...TRUNCATED...")
var truncatedStringLen = len(truncatedString)
