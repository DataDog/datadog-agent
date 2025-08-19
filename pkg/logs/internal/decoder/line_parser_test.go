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
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const header = "HEADER"

type MockLineHandler struct {
	ch chan *message.Message
}

func NewMockLineHandler() *MockLineHandler {
	return &MockLineHandler{
		ch: make(chan *message.Message, 20),
	}
}

func (m *MockLineHandler) process(msg *message.Message) {
	m.ch <- msg
}

func (m *MockLineHandler) flushChan() <-chan time.Time {
	return nil
}
func (m *MockLineHandler) flush() {}

type MockFailingParser struct {
	header []byte
}

func NewMockFailingParser(header string) parsers.Parser {
	return &MockFailingParser{header: []byte(header)}
}

// Parse removes header from line, returns a message if its header matches the Parser header
// or returns an error and flags the line as partial if it does not end up by \n
func (u *MockFailingParser) Parse(input *message.Message) (*message.Message, error) {
	if bytes.HasPrefix(input.GetContent(), u.header) {
		content := bytes.Replace(input.GetContent(), u.header, []byte(""), 1)
		l := len(content)
		if l > 1 && content[l-2] == '\\' && content[l-1] == 'n' {
			msg := message.NewMessage(content[:l-2], nil, "", 0)
			return msg, nil
		}
		msg := message.NewMessage(content, nil, "", 0)
		msg.ParsingExtra = message.ParsingExtra{
			IsPartial: true,
		}
		return msg, nil
	}
	msg := message.NewMessage(input.GetContent(), nil, "", 0)
	return msg, fmt.Errorf("error")
}

func (u *MockFailingParser) SupportsPartialLine() bool {
	return true
}

func TestSingleLineParser(t *testing.T) {
	p := NewMockFailingParser(header)

	lineHandler := NewMockLineHandler()
	lineParser := NewSingleLineParser(lineHandler, p)

	line := header
	logMessage := message.NewMessage([]byte(line), nil, "", 0)

	inputLen := len(logMessage.GetContent()) + 1
	lineParser.process(logMessage, inputLen)
	message := <-lineHandler.ch
	assert.Equal(t, "", string(message.GetContent()))
	assert.Equal(t, inputLen, message.RawDataLen)

	logMessage.SetContent([]byte(line + "one message"))
	inputLen = len(logMessage.GetContent()) + 1
	lineParser.process(logMessage, inputLen)
	message = <-lineHandler.ch
	assert.Equal(t, []byte("one message"), message.GetContent())
	assert.Equal(t, inputLen, message.RawDataLen)
}

func TestSingleLineParserSendsRawInvalidMessages(t *testing.T) {
	p := NewMockFailingParser(header)

	lineHandler := NewMockLineHandler()
	lineParser := NewSingleLineParser(lineHandler, p)

	logMessage := message.NewMessage([]byte("one message"), nil, "", 0)

	lineParser.process(logMessage, 12)
	message := <-lineHandler.ch
	assert.Equal(t, "one message", string(message.GetContent()))
}

func TestMultilineParser(t *testing.T) {
	p := NewMockFailingParser(header)
	timeout := 1000 * time.Millisecond
	contentLenLimit := 256 * 100

	lineHandler := NewMockLineHandler()
	lineParser := NewMultiLineParser(lineHandler, timeout, p, contentLenLimit)

	logMessage := message.NewMessage([]byte(header+"one "), nil, "", 0)

	lineParser.process(logMessage, 11)

	logMessage.SetContent([]byte(header + "long "))
	lineParser.process(logMessage, 12)

	logMessage.SetContent([]byte(header + "line\\n"))
	lineParser.process(logMessage, 14)

	message := <-lineHandler.ch

	assert.Equal(t, "one long line", string(message.GetContent()))
	assert.Equal(t, message.RawDataLen, 11+12+14)
}

func TestMultilineParserTimeout(t *testing.T) {
	p := NewMockFailingParser(header)
	timeout := 100 * time.Millisecond
	contentLenLimit := 256 * 100

	lineHandler := NewMockLineHandler()
	lineParser := NewMultiLineParser(lineHandler, timeout, p, contentLenLimit)

	logMessage := message.NewMessage([]byte(header+"message"), nil, "", 0)

	lineParser.process(logMessage, 14)

	// shouldn't be anything here yet
	select {
	case <-lineHandler.ch:
		panic("shouldn't be a message")
	default:
	}

	lineParser.flush()

	message := <-lineHandler.ch

	assert.Equal(t, "message", string(message.GetContent()))
	assert.Equal(t, message.RawDataLen, 14)
}

func TestMultilineParserLimit(t *testing.T) {
	// Allow buffering to ensure the line_parser does not timeout
	p := NewMockFailingParser(header)
	timeout := 1000 * time.Millisecond
	contentLenLimit := 64
	line := strings.Repeat("a", contentLenLimit)

	lineHandler := NewMockLineHandler()
	lineParser := NewMultiLineParser(lineHandler, timeout, p, contentLenLimit)

	for i := 0; i < 10; i++ {
		logMessage := message.NewMessage([]byte(header+line), nil, "", 0)
		lineParser.process(logMessage, 7+len(line))
	}

	logMessage := message.NewMessage([]byte(header+"aaaa\\n"), nil, "", 0)
	lineParser.process(logMessage, 13)

	for i := 0; i < 10; i++ {
		message := <-lineHandler.ch
		assert.Equal(t, line, string(message.GetContent()))
		assert.Equal(t, message.RawDataLen, 7+len(line))
	}

	message := <-lineHandler.ch
	assert.Equal(t, "aaaa", string(message.GetContent()))
	assert.Equal(t, message.RawDataLen, 13)
}
