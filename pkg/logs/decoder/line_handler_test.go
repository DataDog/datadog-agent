// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/parser"
	"github.com/stretchr/testify/assert"
)

// All valid whitespace characters
const whitespace = "\t\n\v\f\r\u0085\u00a0 "

// Unwrapper mocks the logic of LineUnwrapper
type MockParser struct {
	header []byte
}

// parser.NewIdentityParser returns a new Unwrapper
func NewMockParser(header string) parser.Parser {
	return &MockParser{header: []byte(header)}
}

func (u *MockParser) Parse(msg []byte) (parser.ParsedLine, error) {
	return parser.ParsedLine{Content: msg}, nil
}

// Unwrap removes header from line
func (u *MockParser) Unwrap(line []byte) ([]byte, error) {
	return bytes.Replace(line, u.header, []byte(""), 1), nil
}

func TestSingleLineHandler(t *testing.T) {
	outputChan := make(chan *Output, 10)
	h := NewSingleLineHandler(outputChan, parser.NewIdentityParser())
	h.Start()

	var output *Output
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
	outputChan := make(chan *Output, 10)
	h := NewSingleLineHandler(outputChan, parser.NewIdentityParser())
	h.Start()

	var output *Output
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
	outputChan := make(chan *Output, 10)
	h := NewMultiLineHandler(outputChan, re, 10*time.Millisecond, parser.NewIdentityParser())
	h.Start()

	var output *Output

	// two lines long message should be sent
	h.Handle([]byte("1. first line"))
	h.Handle([]byte("second line"))

	// one line long message should be sent
	h.Handle([]byte("2. first line"))

	output = <-outputChan
	assert.Equal(t, "1. first line"+"\\n"+"second line", string(output.Content))
	assert.Equal(t, len("1. first line"+"second line")+2, output.RawDataLen)

	output = <-outputChan
	assert.Equal(t, "2. first line", string(output.Content))
	assert.Equal(t, len("2. first line")+1, output.RawDataLen)

	// too long line should be truncated
	h.Handle([]byte(strings.Repeat("a", contentLenLimit+10)))
	output = <-outputChan
	assert.Equal(t, contentLenLimit+len(TRUNCATED)+10, len(output.Content))
	assert.Equal(t, contentLenLimit+10, output.RawDataLen)

	h.Handle([]byte(strings.Repeat("a", 10)))
	output = <-outputChan
	assert.Equal(t, string(TRUNCATED)+strings.Repeat("a", 10), string(output.Content))
	assert.Equal(t, 10+1, output.RawDataLen)

	// twice too long lines should be double truncated
	h.Handle([]byte(strings.Repeat("a", contentLenLimit+10)))
	output = <-outputChan
	assert.Equal(t, contentLenLimit+len(TRUNCATED)+10, len(output.Content))
	assert.Equal(t, contentLenLimit+10, output.RawDataLen)

	h.Handle([]byte(strings.Repeat("a", contentLenLimit+10)))
	output = <-outputChan
	assert.Equal(t, len(TRUNCATED)+contentLenLimit+len(TRUNCATED)+10, len(output.Content))
	assert.Equal(t, contentLenLimit+10, output.RawDataLen)

	h.Handle([]byte(strings.Repeat("a", 10)))
	output = <-outputChan
	assert.Equal(t, string(TRUNCATED)+strings.Repeat("a", 10), string(output.Content))
	assert.Equal(t, 10+1, output.RawDataLen)

	h.Stop()
}

func TestTrimMultiLine(t *testing.T) {
	re := regexp.MustCompile("[0-9]+\\.")
	outputChan := make(chan *Output, 10)
	h := NewMultiLineHandler(outputChan, re, 10*time.Millisecond, parser.NewIdentityParser())
	h.Start()

	var output *Output

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
	outputChan := make(chan *Output, 10)
	h := NewMultiLineHandler(outputChan, re, 10*time.Millisecond, NewMockParser(header))
	h.Start()

	var output *Output

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
