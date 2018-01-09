// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package decoder

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// All valid whitespace characters
const whitespace = "\t\n\v\f\r\u0085\u00a0 "

// Unwrapper mocks the logic of LineUnwrapper
type MockUnwrapper struct {
	header []byte
}

// NewUnwrapper returns a new Unwrapper
func NewMockUnwrapper(header string) LineUnwrapper {
	return &MockUnwrapper{[]byte(header)}
}

// Unwrap removes header from line
func (u MockUnwrapper) Unwrap(line []byte) []byte {
	return bytes.Replace(line, u.header, []byte(""), 1)
}

func TestTrimSingleLine(t *testing.T) {
	outChan := make(chan *Output, 10)
	h := NewSingleLineHandler(outChan)

	var out *Output

	// All leading and trailing whitespace characters should be trimmed
	h.Handle([]byte(whitespace + "foo" + whitespace + "bar" + whitespace))
	out = <-outChan
	assert.Equal(t, "foo"+whitespace+"bar", string(out.Content))
}

func TestTrimMultiLine(t *testing.T) {
	re := regexp.MustCompile("[0-9]+\\.")
	outChan := make(chan *Output, 10)
	h := NewMultiLineHandler(outChan, re, NewUnwrapper())

	var out *Output

	// All leading and trailing whitespace characters should be trimmed
	h.Handle([]byte(whitespace + "foo" + whitespace + "bar" + whitespace))
	out = <-outChan
	assert.Equal(t, "foo"+whitespace+"bar", string(out.Content))

	// With line break
	h.Handle([]byte(whitespace + "foo" + whitespace))
	h.Handle([]byte("bar" + whitespace))
	out = <-outChan
	assert.Equal(t, "foo"+whitespace+"\\n"+"bar", string(out.Content))
}

func TestMultiLine(t *testing.T) {
	const header = "HEADER"

	re := regexp.MustCompile("[0-9]+\\.")
	outChan := make(chan *Output, 10)
	h := NewMultiLineHandler(outChan, re, NewMockUnwrapper(header))

	var out *Output

	// Only the header of the first line of each Output should be kept
	h.Handle([]byte(header + "1. first line"))
	h.Handle([]byte(header + "second line"))
	h.Handle([]byte(header + "2. first line"))
	h.Handle([]byte(header + "3. first line"))

	out = <-outChan
	assert.Equal(t, header+"1. first line"+"\\n"+"second line", string(out.Content))
	out = <-outChan
	assert.Equal(t, header+"2. first line", string(out.Content))
	out = <-outChan
	assert.Equal(t, header+"3. first line", string(out.Content))

	// The header of the malformed content should remain
	h.Handle([]byte(header + "malformed first line"))
	h.Handle([]byte(header + "second line"))
	h.Handle([]byte(header + "4. first line"))

	out = <-outChan
	assert.Equal(t, header+"malformed first line\\nsecond line", string(out.Content))
	out = <-outChan
	assert.Equal(t, header+"4. first line", string(out.Content))
}
