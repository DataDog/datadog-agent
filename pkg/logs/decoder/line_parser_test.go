// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inc.

package decoder

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/stretchr/testify/assert"
)

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

// Parse removes header from line and returns a message if its header matches the Parser header
// or returns an error
func (u *MockFailingParser) Parse(msg []byte) ([]byte, string, string, bool, error) {
	if bytes.HasPrefix(msg, u.header) {
		return bytes.Replace(msg, u.header, []byte(""), 1), "", "", false, nil
	}
	return msg, "", "", false, fmt.Errorf("error")
}

func (u *MockFailingParser) SupportsPartialLine() bool {
	return false
}

func TestSingleLineParser(t *testing.T) {
	const header = "HEADER"
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
	const header = "HEADER"

	h := &MockHandler{make(chan *Message)}
	p := NewMockFailingParser(header)

	lineParser := NewSingleLineParser(p, h)
	lineParser.Start()

	lineParser.Handle(&DecodedInput{[]byte("one message"), 12})
	message := <-h.ouputChan
	assert.Equal(t, "one message", string(message.Content))

	lineParser.Stop()
}
