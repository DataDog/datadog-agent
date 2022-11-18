// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package decoder

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
)

const header = "HEADER"

func lineParserChans() (func(*Message), chan *Message) {
	ch := make(chan *Message, 20)
	return func(m *Message) { ch <- m }, ch
}

type MockFailingParser struct {
	header []byte
}

func NewMockFailingParser(header string) parsers.Parser {
	return &MockFailingParser{header: []byte(header)}
}

// Parse removes header from line, returns a message if its header matches the Parser header
// or returns an error and flags the line as partial if it does not end up by \n
func (u *MockFailingParser) Parse(msg []byte) (parsers.Message, error) {
	if bytes.HasPrefix(msg, u.header) {
		msg := bytes.Replace(msg, u.header, []byte(""), 1)
		l := len(msg)
		if l > 1 && msg[l-2] == '\\' && msg[l-1] == 'n' {
			return parsers.Message{Content: msg[:l-2]}, nil
		}
		return parsers.Message{
			Content:   msg,
			IsPartial: true,
		}, nil
	}
	return parsers.Message{Content: msg}, fmt.Errorf("error")
}

func (u *MockFailingParser) SupportsPartialLine() bool {
	return true
}

func TestSingleLineParser(t *testing.T) {
	var message *Message
	p := NewMockFailingParser(header)

	outputFn, outputChan := lineParserChans()
	lineParser := NewSingleLineParser(outputFn, p)

	line := header

	inputLen := len(line) + 1
	lineParser.process([]byte(line), inputLen)
	message = <-outputChan
	assert.Equal(t, "", string(message.Content))
	assert.Equal(t, inputLen, message.RawDataLen)

	inputLen = len(line+"one message") + 1
	lineParser.process([]byte(line+"one message"), inputLen)
	message = <-outputChan
	assert.Equal(t, "one message", string(message.Content))
	assert.Equal(t, inputLen, message.RawDataLen)
}

func TestSingleLineParserSendsRawInvalidMessages(t *testing.T) {
	p := NewMockFailingParser(header)

	outputFn, outputChan := lineParserChans()
	lineParser := NewSingleLineParser(outputFn, p)

	lineParser.process([]byte("one message"), 12)
	message := <-outputChan
	assert.Equal(t, "one message", string(message.Content))
}

func TestMultilineParser(t *testing.T) {
	p := NewMockFailingParser(header)
	timeout := 1000 * time.Millisecond
	contentLenLimit := 256 * 100

	outputFn, outputChan := lineParserChans()
	lineParser := NewMultiLineParser(outputFn, timeout, p, contentLenLimit)

	lineParser.process([]byte(header+"one "), 11)
	lineParser.process([]byte(header+"long "), 12)
	lineParser.process([]byte(header+"line\\n"), 14)

	message := <-outputChan

	assert.Equal(t, "one long line", string(message.Content))
	assert.Equal(t, message.RawDataLen, 11+12+14)
}

func TestMultilineParserTimeout(t *testing.T) {
	p := NewMockFailingParser(header)
	timeout := 100 * time.Millisecond
	contentLenLimit := 256 * 100

	outputFn, outputChan := lineParserChans()
	lineParser := NewMultiLineParser(outputFn, timeout, p, contentLenLimit)

	lineParser.process([]byte(header+"message"), 14)

	// shouldn't be anything here yet
	select {
	case <-outputChan:
		panic("shouldn't be a message")
	default:
	}

	lineParser.flush()

	message := <-outputChan

	assert.Equal(t, "message", string(message.Content))
	assert.Equal(t, message.RawDataLen, 14)
}

func TestMultilineParserLimit(t *testing.T) {
	// Allow buffering to ensure the line_parser does not timeout
	p := NewMockFailingParser(header)
	timeout := 1000 * time.Millisecond
	contentLenLimit := 64
	var message *Message
	line := strings.Repeat("a", contentLenLimit)

	outputFn, outputChan := lineParserChans()
	lineParser := NewMultiLineParser(outputFn, timeout, p, contentLenLimit)

	for i := 0; i < 10; i++ {
		lineParser.process([]byte(header+line), 7+len(line))
	}
	lineParser.process([]byte(header+"aaaa\\n"), 13)

	for i := 0; i < 10; i++ {
		message = <-outputChan
		assert.Equal(t, line, string(message.Content))
		assert.Equal(t, message.RawDataLen, 7+len(line))
	}

	message = <-outputChan
	assert.Equal(t, "aaaa", string(message.Content))
	assert.Equal(t, message.RawDataLen, 13)
}
