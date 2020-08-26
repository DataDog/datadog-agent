// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inc.

package decoder

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/stretchr/testify/assert"
)

const header = "HEADER"

type MockHandler struct {
	ouputChan chan *Message
}

func (h *MockHandler) Handle(input *Message) {
	h.ouputChan <- input
}

func (h *MockHandler) Start() {
	return
}

func (h *MockHandler) Stop() {
	return
}

type MockFailingParser struct {
	header []byte
}

func NewMockFailingParser(header string) parser.Parser {
	return &MockFailingParser{header: []byte(header)}
}

// Parse removes header from line, returns a message if its header matches the Parser header
// or returns an error and flags the line as partial if it does not end up by \n
func (u *MockFailingParser) Parse(msg []byte) ([]byte, string, string, bool, error) {
	if bytes.HasPrefix(msg, u.header) {
		msg := bytes.Replace(msg, u.header, []byte(""), 1)
		l := len(msg)
		if l > 1 && msg[l-2] == '\\' && msg[l-1] == 'n' {
			return msg[:l-2], "", "", false, nil
		}
		return msg, "", "", true, nil
	}
	return msg, "", "", false, fmt.Errorf("error")
}

func (u *MockFailingParser) SupportsPartialLine() bool {
	return true
}

func TestSingleLineParser(t *testing.T) {
	var message *Message
	h := &MockHandler{make(chan *Message)}
	p := NewMockFailingParser(header)

	lineParser := NewSingleLineParser(p, h)
	lineParser.Start()

	line := header

	lineParser.Handle(&DecodedInput{[]byte(line), 7})
	message = <-h.ouputChan
	assert.Equal(t, "", string(message.Content))
	assert.Equal(t, 7, message.RawDataLen)

	lineParser.Handle(&DecodedInput{[]byte(line + "one message"), 18})
	message = <-h.ouputChan

	assert.Equal(t, "one message", string(message.Content))
	assert.Equal(t, 18, message.RawDataLen)

	lineParser.Stop()
}

func TestSingleLineParserSendsRawInvalidMessages(t *testing.T) {

	h := &MockHandler{make(chan *Message)}
	p := NewMockFailingParser(header)

	lineParser := NewSingleLineParser(p, h)
	lineParser.Start()

	lineParser.Handle(&DecodedInput{[]byte("one message"), 12})
	message := <-h.ouputChan
	assert.Equal(t, "one message", string(message.Content))

	lineParser.Stop()
}

func TestMultilineParser(t *testing.T) {
	h := &MockHandler{make(chan *Message)}
	p := NewMockFailingParser(header)
	timeout := 1000 * time.Millisecond
	contentLenLimit := 256 * 100

	lineParser := NewMultiLineParser(timeout, p, h, contentLenLimit)
	lineParser.Start()

	lineParser.Handle(&DecodedInput{[]byte(header + "one "), 11})
	lineParser.Handle(&DecodedInput{[]byte(header + "long "), 12})
	lineParser.Handle(&DecodedInput{[]byte(header + "line\\n"), 14})

	message := <-h.ouputChan

	assert.Equal(t, "one long line", string(message.Content))
	assert.Equal(t, message.RawDataLen, 11+12+14)

	lineParser.Stop()
}

func TestMultilineParserTimeout(t *testing.T) {
	h := &MockHandler{make(chan *Message)}
	p := NewMockFailingParser(header)
	timeout := 100 * time.Millisecond
	contentLenLimit := 256 * 100

	lineParser := NewMultiLineParser(timeout, p, h, contentLenLimit)
	lineParser.Start()

	lineParser.Handle(&DecodedInput{[]byte(header + "message"), 14})

	message := <-h.ouputChan

	assert.Equal(t, "message", string(message.Content))
	assert.Equal(t, message.RawDataLen, 14)

	lineParser.Stop()
}

func TestMultilineParserLimit(t *testing.T) {
	// Allow buffering to ensure the line_parser does not timeout
	h := &MockHandler{make(chan *Message, 10)}
	p := NewMockFailingParser(header)
	timeout := 1000 * time.Millisecond
	contentLenLimit := 64
	var message *Message
	line := strings.Repeat("a", contentLenLimit)

	lineParser := NewMultiLineParser(timeout, p, h, contentLenLimit)
	lineParser.Start()

	for i := 0; i < 10; i++ {
		lineParser.Handle(&DecodedInput{[]byte(header + line), 7 + len(line)})
	}
	lineParser.Handle(&DecodedInput{[]byte(header + "aaaa\\n"), 13})

	for i := 0; i < 10; i++ {
		message = <-h.ouputChan
		assert.Equal(t, line, string(message.Content))
		assert.Equal(t, message.RawDataLen, 7+len(line))
	}

	message = <-h.ouputChan
	assert.Equal(t, "aaaa", string(message.Content))
	assert.Equal(t, message.RawDataLen, 13)

	lineParser.Stop()
}
