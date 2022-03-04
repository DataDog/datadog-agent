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

	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const header = "HEADER"

func lineParserChans() (chan *DecodedInput, chan *Message) {
	return make(chan *DecodedInput, 5), make(chan *Message, 5)
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

	inputChan, outputChan := lineParserChans()
	lineParser := NewSingleLineParser(inputChan, outputChan, p)
	lineParser.Start()

	line := header

	inputLen := len(line) + 1
	inputChan <- &DecodedInput{[]byte(line), inputLen}
	message = <-outputChan
	assert.Equal(t, "", string(message.Content))
	assert.Equal(t, inputLen, message.RawDataLen)

	inputLen = len(line+"one message") + 1
	inputChan <- &DecodedInput{[]byte(line + "one message"), inputLen}
	message = <-outputChan
	assert.Equal(t, "one message", string(message.Content))
	assert.Equal(t, inputLen, message.RawDataLen)

	close(inputChan)

	// once the input channel closes, the output channel closes as well
	_, ok := <-outputChan
	require.Equal(t, false, ok)
}

func TestSingleLineParserSendsRawInvalidMessages(t *testing.T) {
	p := NewMockFailingParser(header)

	inputChan, outputChan := lineParserChans()
	lineParser := NewSingleLineParser(inputChan, outputChan, p)
	lineParser.Start()

	inputChan <- &DecodedInput{[]byte("one message"), 12}
	message := <-outputChan
	assert.Equal(t, "one message", string(message.Content))

	close(inputChan)

	// once the input channel closes, the output channel closes as well
	_, ok := <-outputChan
	require.Equal(t, false, ok)
}

func TestMultilineParser(t *testing.T) {
	p := NewMockFailingParser(header)
	timeout := 1000 * time.Millisecond
	contentLenLimit := 256 * 100

	inputChan, outputChan := lineParserChans()
	lineParser := NewMultiLineParser(inputChan, outputChan, timeout, p, contentLenLimit)
	lineParser.Start()

	inputChan <- &DecodedInput{[]byte(header + "one "), 11}
	inputChan <- &DecodedInput{[]byte(header + "long "), 12}
	inputChan <- &DecodedInput{[]byte(header + "line\\n"), 14}

	message := <-outputChan

	assert.Equal(t, "one long line", string(message.Content))
	assert.Equal(t, message.RawDataLen, 11+12+14)

	close(inputChan)

	// once the input channel closes, the output channel closes as well
	_, ok := <-outputChan
	require.Equal(t, false, ok)
}

func TestMultilineParserTimeout(t *testing.T) {
	p := NewMockFailingParser(header)
	timeout := 100 * time.Millisecond
	contentLenLimit := 256 * 100

	inputChan, outputChan := lineParserChans()
	lineParser := NewMultiLineParser(inputChan, outputChan, timeout, p, contentLenLimit)
	lineParser.Start()
	defer close(inputChan)

	inputChan <- &DecodedInput{[]byte(header + "message"), 14}

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

	inputChan, outputChan := lineParserChans()
	lineParser := NewMultiLineParser(inputChan, outputChan, timeout, p, contentLenLimit)
	lineParser.Start()
	defer close(inputChan)

	for i := 0; i < 10; i++ {
		inputChan <- &DecodedInput{[]byte(header + line), 7 + len(line)}
	}
	inputChan <- &DecodedInput{[]byte(header + "aaaa\\n"), 13}

	for i := 0; i < 10; i++ {
		message = <-outputChan
		assert.Equal(t, line, string(message.Content))
		assert.Equal(t, message.RawDataLen, 7+len(line))
	}

	message = <-outputChan
	assert.Equal(t, "aaaa", string(message.Content))
	assert.Equal(t, message.RawDataLen, 13)
}
