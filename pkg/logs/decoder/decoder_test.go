// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package decoder

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeIncomingDataForSingleLineLogs(t *testing.T) {
	outChan := make(chan *Output, 10)
	d := New(nil, outChan, NewSingleLineHandler(outChan))

	var out *Output

	// multiple messages in one buffer
	d.decodeIncomingData([]byte("helloworld\n"))
	out = <-outChan
	assert.Equal(t, "helloworld", string(out.Content))
	assert.Equal(t, "", d.lineBuffer.String())

	d.decodeIncomingData([]byte("helloworld\nhowayou\ngoodandyou"))
	out = <-outChan
	assert.Equal(t, "helloworld", string(out.Content))
	out = <-outChan
	assert.Equal(t, "howayou", string(out.Content))
	assert.Equal(t, "goodandyou", d.lineBuffer.String())
	d.lineBuffer.Reset()

	// messages overflow in the next buffer
	d.decodeIncomingData([]byte("helloworld\nthisisa"))
	assert.Equal(t, "thisisa", d.lineBuffer.String())
	d.decodeIncomingData([]byte("longinput\nindeed"))
	out = <-outChan
	out = <-outChan
	assert.Equal(t, "thisisalonginput", string(out.Content))
	assert.Equal(t, "indeed", d.lineBuffer.String())
	d.lineBuffer.Reset()

	// edge cases, do not crash
	d.decodeIncomingData([]byte("\n\n"))
	d.decodeIncomingData([]byte(""))
	d.lineBuffer.Reset()

	// buffer overflow
	d.decodeIncomingData([]byte("hello world"))
	d.decodeIncomingData([]byte("!\n"))
	out = <-outChan
	assert.Equal(t, "hello world!", string(out.Content))
	d.lineBuffer.Reset()

	// message too big
	d.decodeIncomingData([]byte(strings.Repeat("a", contentLenLimit+10) + "\n"))
	out = <-outChan
	assert.Equal(t, contentLenLimit+len(TRUNCATED), len(out.Content))
	out = <-outChan
	assert.Equal(t, string(TRUNCATED)+strings.Repeat("a", 10), string(out.Content))
	d.lineBuffer.Reset()

	// message too big, over several calls
	d.decodeIncomingData([]byte(strings.Repeat("a", contentLenLimit-5)))
	d.decodeIncomingData([]byte(strings.Repeat("a", 15) + "\n"))
	out = <-outChan
	assert.Equal(t, contentLenLimit+len(TRUNCATED), len(out.Content))
	out = <-outChan
	assert.Equal(t, string(TRUNCATED)+strings.Repeat("a", 10), string(out.Content))
	d.lineBuffer.Reset()

	// message twice too big
	d.decodeIncomingData([]byte(strings.Repeat("a", 2*contentLenLimit+10) + "\n"))
	out = <-outChan
	assert.Equal(t, contentLenLimit+len(TRUNCATED), len(out.Content))
	out = <-outChan
	assert.Equal(t, len(TRUNCATED)+contentLenLimit+len(TRUNCATED), len(out.Content))
	out = <-outChan
	assert.Equal(t, string(TRUNCATED)+strings.Repeat("a", 10), string(out.Content))

	// message twice too big, over several calls
	d.decodeIncomingData([]byte(strings.Repeat("a", contentLenLimit+5)))
	d.decodeIncomingData([]byte(strings.Repeat("a", contentLenLimit+5) + "\n"))
	out = <-outChan
	assert.Equal(t, contentLenLimit+len(TRUNCATED), len(out.Content))
	out = <-outChan
	assert.Equal(t, len(TRUNCATED)+contentLenLimit+len(TRUNCATED), len(out.Content))
	out = <-outChan
	assert.Equal(t, string(TRUNCATED)+strings.Repeat("a", 10), string(out.Content))
	d.lineBuffer.Reset()
}

func TestDecodeIncomingDataForMultiLineLogs(t *testing.T) {
	inChan := make(chan *Input, 10)
	outChan := make(chan *Output, 10)
	re := regexp.MustCompile("[0-9]+\\.")
	d := New(inChan, outChan, NewMultiLineLineHandler(outChan, re))

	var out *Output

	// pending message in one raw data
	d.decodeIncomingData([]byte("1. Hello world!"))
	assert.Equal(t, "1. Hello world!", string(d.lineBuffer.Bytes()))
	d.decodeIncomingData([]byte("\n"))
	out = <-outChan
	assert.Equal(t, "1. Hello world!", string(out.Content))

	go d.Start()

	// two lines message in one raw data
	inChan <- NewInput([]byte("1. Hello\nworld!\n"))
	out = <-outChan
	assert.Equal(t, "1. Hello\\nworld!", string(out.Content))

	// multiple messages in one raw data
	inChan <- NewInput([]byte("1. Hello\nworld!\n2. How are you\n"))
	out = <-outChan
	assert.Equal(t, "1. Hello\\nworld!", string(out.Content))
	out = <-outChan
	assert.Equal(t, "2. How are you", string(out.Content))

	// two lines message over two raw data
	inChan <- NewInput([]byte("1. Hello\n"))
	inChan <- NewInput([]byte("world!\n"))
	out = <-outChan
	assert.Equal(t, "1. Hello\\nworld!", string(out.Content))

	// multiple messages over two raw data
	inChan <- NewInput([]byte("1. Hello\n"))
	inChan <- NewInput([]byte("world!\n2. How are you\n"))
	out = <-outChan
	assert.Equal(t, "1. Hello\\nworld!", string(out.Content))
	out = <-outChan
	assert.Equal(t, "2. How are you", string(out.Content))

	// single-line message in one raw data
	inChan <- NewInput([]byte("1. Hello world!\n"))
	out = <-outChan
	assert.Equal(t, "1. Hello world!", string(out.Content))

	// multiple single-line messages in one raw data
	inChan <- NewInput([]byte("1. Hello world!\n2. How are you\n"))
	out = <-outChan
	assert.Equal(t, "1. Hello world!", string(out.Content))
	out = <-outChan
	assert.Equal(t, "2. How are you", string(out.Content))

	// two lines too big message in one raw data
	inChan <- NewInput([]byte("12345678.\n" + strings.Repeat("a", contentLenLimit+10) + "\n"))
	out = <-outChan
	assert.Equal(t, 11+contentLenLimit+len(TRUNCATED), len(out.Content))
	out = <-outChan
	assert.Equal(t, string(TRUNCATED)+strings.Repeat("a", 10), string(out.Content))

	// two lines too big message in two raw data
	inChan <- NewInput([]byte("12345678.\n"))
	inChan <- NewInput([]byte(strings.Repeat("a", contentLenLimit+10) + "\n"))
	out = <-outChan
	assert.Equal(t, 11+contentLenLimit+len(TRUNCATED), len(out.Content))
	out = <-outChan
	assert.Equal(t, string(TRUNCATED)+strings.Repeat("a", 10), string(out.Content))

	// single-line too big message over two raw data
	inChan <- NewInput([]byte(strings.Repeat("a", contentLenLimit)))
	inChan <- NewInput([]byte(strings.Repeat("a", 10) + "\n"))
	out = <-outChan
	assert.Equal(t, contentLenLimit+len(TRUNCATED), len(out.Content))
	out = <-outChan
	assert.Equal(t, string(TRUNCATED)+strings.Repeat("a", 10), string(out.Content))

	// single-line too big message in one raw data
	inChan <- NewInput([]byte(strings.Repeat("a", contentLenLimit+10) + "\n"))
	out = <-outChan
	assert.Equal(t, contentLenLimit+len(TRUNCATED), len(out.Content))
	out = <-outChan
	assert.Equal(t, string(TRUNCATED)+strings.Repeat("a", 10), string(out.Content))

	// message twice too big in one raw data
	inChan <- NewInput([]byte(strings.Repeat("a", 2*contentLenLimit+10) + "\n"))
	out = <-outChan
	assert.Equal(t, contentLenLimit+len(TRUNCATED), len(out.Content))
	out = <-outChan
	assert.Equal(t, len(TRUNCATED)+contentLenLimit+len(TRUNCATED), len(out.Content))
	out = <-outChan
	assert.Equal(t, string(TRUNCATED)+strings.Repeat("a", 10), string(out.Content))

	// message twice too big over two raw data
	inChan <- NewInput([]byte(strings.Repeat("a", contentLenLimit+5)))
	inChan <- NewInput([]byte(strings.Repeat("a", contentLenLimit+5) + "\n"))
	out = <-outChan
	assert.Equal(t, len(TRUNCATED)+contentLenLimit, len(out.Content))
	out = <-outChan
	assert.Equal(t, len(TRUNCATED)+contentLenLimit+len(TRUNCATED), len(out.Content))
	out = <-outChan
	assert.Equal(t, string(TRUNCATED)+strings.Repeat("a", 10), string(out.Content))
}

func TestSingleLineDecoderLifecycle(t *testing.T) {
	inChan := make(chan *Input, 10)
	outChan := make(chan *Output, 10)
	d := New(inChan, outChan, NewSingleLineHandler(outChan))
	d.Start()

	d.Stop()
	out := <-outChan
	assert.Equal(t, reflect.TypeOf(out), reflect.TypeOf(newStopOutput()))
}

func TestMultiLineDecoderLifecycle(t *testing.T) {
	inChan := make(chan *Input, 10)
	outChan := make(chan *Output, 10)
	d := New(inChan, outChan, NewMultiLineLineHandler(outChan, nil))
	d.Start()

	d.Stop()
	out := <-outChan
	assert.Equal(t, reflect.TypeOf(out), reflect.TypeOf(newStopOutput()))
}
