// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/stretchr/testify/assert"
)

// All valid whitespace characters
const whitespace = "\t\n\v\f\r\u0085\u00a0 "

// MockParser mocks the logic of a Parser
type MockParser struct {
	header []byte
}

// MockUnwrapper mocks the logic of LineUnwrapper
type MockUnwrapper struct {
	header []byte
}

// MockTSParser mocks the logic of Timestamps
type MockTSParser struct {
	header []byte
}

type MockFailingParser struct {
	header []byte
}

func NewMockParser(header string) parser.Parser {
	return &MockParser{header: []byte(header)}
}

// Parse removes header from line and returns a message
func (u *MockParser) Parse(msg []byte) (*message.Message, error) {
	return &message.Message{Content: bytes.Replace(msg, u.header, []byte(""), 1)}, nil
}

// Unwrap removes header from line and return also the timestamp
func (u *MockParser) Unwrap(line []byte) ([]byte, string, error) {
	return bytes.Replace(line, u.header, []byte(""), 1), "", nil
}

func NewMockUnwrapper(header string) parser.Parser {
	return &MockUnwrapper{header: []byte(header)}
}

// Parse does nothing for MockUnwrapper
func (u *MockUnwrapper) Parse(msg []byte) (*message.Message, error) {
	return &message.Message{Content: msg}, nil
}

// Unwrap removes header from line and return also the timestamp
func (u *MockUnwrapper) Unwrap(line []byte) ([]byte, string, error) {
	return bytes.Replace(line, u.header, []byte(""), 1), "", nil
}

func NewMockTSParser(header string) parser.Parser {
	return &MockTSParser{header: []byte(header)}
}

// Parse does nothing for MockUnwrapper
func (u *MockTSParser) Parse(msg []byte) (*message.Message, error) {
	components := bytes.SplitN(msg, []byte{' '}, 2)

	return &message.Message{Content: components[1], Timestamp: string(components[0])}, nil
}

// Unwrap removes header from line and return also the timestamp
func (u *MockTSParser) Unwrap(line []byte) ([]byte, string, error) {
	components := bytes.SplitN(line, []byte{' '}, 2)
	return components[1], string(components[0]), nil
}

func NewMockFailingParser(header string) parser.Parser {
	return &MockFailingParser{header: []byte(header)}
}

// Parse removes header from line and returns a message if its header matches the Parser header
// or returns an error
func (u *MockFailingParser) Parse(msg []byte) (*message.Message, error) {
	if bytes.HasPrefix(msg, u.header) {
		return &message.Message{Content: bytes.Replace(msg, u.header, []byte(""), 1)}, nil
	}
	return &message.Message{Content: msg}, fmt.Errorf("error")
}

// Unwrap removes header from line if the header matches the Parser header or returns an error
func (u *MockFailingParser) Unwrap(line []byte) ([]byte, string, error) {
	if bytes.HasPrefix(line, u.header) {
		return bytes.Replace(line, u.header, []byte(""), 1), "", nil
	}
	return line, "", fmt.Errorf("error")
}

func TestSingleLineHandler(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	h := NewSingleLineHandler(outputChan, parser.NoopParser, 100)
	h.Start()

	var output *message.Message
	var line string

	// valid line should be sent
	line = "hello world"
	h.Handle([]byte(line))
	output = <-outputChan
	assert.Equal(t, line, string(output.Content))
	assert.Equal(t, len(line)+1, output.RawDataLen)

	// empty line should be dropped
	h.Handle([]byte(""))
	assert.Equal(t, 0, len(outputChan))

	// too long line should be truncated
	line = strings.Repeat("a", contentLenLimit+10)
	h.Handle([]byte(line))
	output = <-outputChan
	assert.Equal(t, len(line)+len(TRUNCATED), len(output.Content))
	assert.Equal(t, len(line), output.RawDataLen)

	line = strings.Repeat("a", contentLenLimit+10)
	h.Handle([]byte(line))
	output = <-outputChan
	assert.Equal(t, len(TRUNCATED)+len(line)+len(TRUNCATED), len(output.Content))
	assert.Equal(t, len(line), output.RawDataLen)

	line = strings.Repeat("a", 10)
	h.Handle([]byte(line))
	output = <-outputChan
	assert.Equal(t, string(TRUNCATED)+line, string(output.Content))
	assert.Equal(t, len(line)+1, output.RawDataLen)

	h.Stop()
}

func TestTrimSingleLine(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	h := NewSingleLineHandler(outputChan, parser.NoopParser, 100)
	h.Start()

	var output *message.Message
	var line string

	// All leading and trailing whitespace characters should be trimmed
	line = whitespace + "foo" + whitespace + "bar" + whitespace
	h.Handle([]byte(line))
	output = <-outputChan
	assert.Equal(t, "foo"+whitespace+"bar", string(output.Content))
	assert.Equal(t, len(line)+1, output.RawDataLen)

	h.Stop()
}

func TestMultiLineHandler(t *testing.T) {
	re := regexp.MustCompile("[0-9]+\\.")
	outputChan := make(chan *message.Message, 10)
	h := NewMultiLineHandler(outputChan, re, 10*time.Millisecond, parser.NoopParser, 20)
	h.Start()

	var output *message.Message

	// two lines long message should be sent
	h.Handle([]byte("1.first"))
	h.Handle([]byte("second"))

	// one line long message should be sent
	h.Handle([]byte("2. first line"))

	output = <-outputChan
	var expectedContent = "1.first\\nsecond"
	assert.Equal(t, expectedContent, string(output.Content))
	assert.Equal(t, len(expectedContent), output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "2. first line", string(output.Content))
	assert.Equal(t, len("2. first line")+1, output.RawDataLen)

	// too long line should be truncated
	h.Handle([]byte("3. stringssssssize20"))
	h.Handle([]byte("con"))

	output = <-outputChan
	assert.Equal(t, "3. stringssssssize20...TRUNCATED...", string(output.Content))
	assert.Equal(t, len("3. stringssssssize20"), output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...con", string(output.Content))
	assert.Equal(t, 4, output.RawDataLen)

	// second line + TRUNCATED too long
	h.Handle([]byte("4. stringssssssize20"))
	h.Handle([]byte("continue"))

	output = <-outputChan
	assert.Equal(t, "4. stringssssssize20...TRUNCATED...", string(output.Content))
	assert.Equal(t, len("4. stringssssssize20"), output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...continue...TRUNCATED...", string(output.Content))
	assert.Equal(t, 8, output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...", string(output.Content))
	assert.Equal(t, 0, output.RawDataLen)

	// continuous too long lines
	h.Handle([]byte("5. stringssssssize20"))
	longLineTracingSpaces := "continu             "
	h.Handle([]byte(longLineTracingSpaces))
	h.Handle([]byte("end"))
	shortLineTracingSpaces := "6. next line      "
	h.Handle([]byte(shortLineTracingSpaces))

	output = <-outputChan
	assert.Equal(t, "5. stringssssssize20...TRUNCATED...", string(output.Content))
	assert.Equal(t, len("5. stringssssssize20"), output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...continu             ...TRUNCATED...", string(output.Content))
	assert.Equal(t, len(longLineTracingSpaces), output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "...TRUNCATED...end", string(output.Content))
	assert.Equal(t, len("end\n"), output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "6. next line", string(output.Content))
	assert.Equal(t, len(shortLineTracingSpaces) + 1, output.RawDataLen)

	h.Stop()
}

func TestTrimMultiLine(t *testing.T) {
	re := regexp.MustCompile("[0-9]+\\.")
	outputChan := make(chan *message.Message, 10)
	h := NewMultiLineHandler(outputChan, re, 10*time.Millisecond, parser.NoopParser, 100)
	h.Start()

	var output *message.Message

	// All leading and trailing whitespace characters should be trimmed
	h.Handle([]byte(whitespace + "foo" + whitespace + "bar" + whitespace))
	output = <-outputChan
	assert.Equal(t, "foo"+whitespace+"bar", string(output.Content))
	assert.Equal(t, len(whitespace+"foo"+whitespace+"bar"+whitespace)+1, output.RawDataLen)

	// With line break
	h.Handle([]byte(whitespace + "foo" + whitespace))
	h.Handle([]byte("bar" + whitespace))
	output = <-outputChan
	assert.Equal(t, "foo"+whitespace+"\\n"+"bar", string(output.Content))
	assert.Equal(t, len(whitespace+"foo"+whitespace)+1+len("bar"+whitespace)+1, output.RawDataLen)

	h.Stop()
}

func TestUnwrapMultiLine(t *testing.T) {
	const header = "HEADER"

	re := regexp.MustCompile("[0-9]+\\.")
	outputChan := make(chan *message.Message, 10)
	h := NewMultiLineHandler(outputChan, re, 10*time.Millisecond, NewMockUnwrapper(header), 100)
	h.Start()

	var output *message.Message

	// Only the header of the first line of each Output should be kept
	h.Handle([]byte(header + "1. first line"))
	h.Handle([]byte(header + "second line"))
	h.Handle([]byte(header + "2. first line"))
	h.Handle([]byte(header + "3. first line"))

	output = <-outputChan
	assert.Equal(t, header+"1. first line"+"\\n"+"second line", string(output.Content))
	output = <-outputChan
	assert.Equal(t, header+"2. first line", string(output.Content))
	output = <-outputChan
	assert.Equal(t, header+"3. first line", string(output.Content))

	// The header of the malformed content should remain
	h.Handle([]byte(header + "malformed first line"))
	h.Handle([]byte(header + "second line"))
	h.Handle([]byte(header + "4. first line"))

	output = <-outputChan
	assert.Equal(t, header+"malformed first line\\nsecond line", string(output.Content))
	output = <-outputChan
	assert.Equal(t, header+"4. first line", string(output.Content))

	h.Stop()
}

func TestUnwrapTSMultiLineTimestamp(t *testing.T) {
	const header = "HEADER"

	re := regexp.MustCompile("Exception")
	outputChan := make(chan *message.Message, 10)
	h := NewMultiLineHandler(outputChan, re, 10*time.Microsecond, NewMockTSParser(header), 200)
	h.Start()

	var output *message.Message

	// Only the header of the first line of each Output should be kept
	h.Handle([]byte("2019-04-02T15:08:32.320682273Z" + " HelloWorld"))
	h.Handle([]byte("2019-04-02T15:08:32.321763516Z" + " Exception"))
	h.Handle([]byte("2019-04-02T15:08:32.321777174Z" + " HelloWorld.m1(HelloWorld.java:3)"))
	h.Handle([]byte("2019-04-02T15:08:32.322102976Z" + " HelloWorld.main(HelloWorld.java:20)"))

	output = <-outputChan
	assert.Equal(t, "HelloWorld", string(output.Content))
	assert.Equal(t, "2019-04-02T15:08:32.321763516Z", string(output.Timestamp))

	output = <-outputChan
	assert.Equal(t, "Exception"+"\\n"+"HelloWorld.m1(HelloWorld.java:3)"+"\\n"+"HelloWorld.main(HelloWorld.java:20)", string(output.Content))
	assert.Equal(t, "2019-04-02T15:08:32.322102976Z", string(output.Timestamp))

	h.Stop()
}

func TestSingleLineHandlerDropsEmptyMessages(t *testing.T) {
	const header = "HEADER"
	outputChan := make(chan *message.Message, 10)
	h := NewSingleLineHandler(outputChan, NewMockParser(header), 100)
	h.Start()

	line := header
	h.Handle([]byte(line))
	h.Handle([]byte(line + "one message"))

	var output *message.Message

	output = <-outputChan
	assert.Equal(t, "one message", string(output.Content))
}

func TestMultiLineHandlerDropsEmptyMessages(t *testing.T) {
	const header = "HEADER"
	outputChan := make(chan *message.Message, 10)
	re := regexp.MustCompile("[0-9]+\\.")
	h := NewMultiLineHandler(outputChan, re, 10*time.Millisecond, NewMockParser(header), 100)
	h.Start()

	h.Handle([]byte(header))

	h.Handle([]byte(header + "1.third line"))
	h.Handle([]byte("fourth line"))

	var output *message.Message

	output = <-outputChan
	assert.Equal(t, "1.third line\\nfourth line", string(output.Content))
}

func TestSingleLineHandlerSendsRawInvalidMessages(t *testing.T) {
	const header = "HEADER"
	outputChan := make(chan *message.Message, 10)
	h := NewSingleLineHandler(outputChan, NewMockFailingParser(header), 100)
	h.Start()

	h.Handle([]byte("one message"))

	var output *message.Message

	output = <-outputChan
	assert.Equal(t, "one message", string(output.Content))
}

func TestMultiLineHandlerSendsRawInvalidMessages(t *testing.T) {
	const header = "HEADER"
	outputChan := make(chan *message.Message, 10)
	re := regexp.MustCompile("[0-9]+\\.")
	h := NewMultiLineHandler(outputChan, re, 10*time.Millisecond, NewMockFailingParser(header), 100)
	h.Start()

	h.Handle([]byte("1.third line"))
	h.Handle([]byte("fourth line"))

	var output *message.Message

	output = <-outputChan
	assert.Equal(t, "1.third line\\nfourth line", string(output.Content))
}
