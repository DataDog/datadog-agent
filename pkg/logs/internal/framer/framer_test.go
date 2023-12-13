// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const contentLenLimit = 256000

// brokenLine represents a decoded line and the raw length
type brokenLine struct {
	content    []byte
	rawDataLen int
}

// framerOutput returns an outputFn for use with a Framer, and
// a channel containing the broken lines passed to that outputFn.
func framerOutput() (func(*message.Message, int), chan brokenLine) {
	ch := make(chan brokenLine, 10)
	return func(msg *message.Message, rawDataLen int) { ch <- brokenLine{msg.GetContent(), rawDataLen} }, ch
}

func chunk(input []byte, size int) [][]byte {
	rv := [][]byte{}
	iter := input
	for len(iter) > 0 {
		if size <= len(iter) {
			rv = append(rv, iter)
			break
		} else {
			rv = append(rv, iter[:size])
			iter = iter[size:]
		}
	}
	return rv
}

func TestLineBreaking(t *testing.T) {
	test := func(framing Framing, chunks [][]byte, lines []string, rawLens []int) func(*testing.T) {
		return func(t *testing.T) {
			gotContent := []string{}
			gotLens := []int{}
			outputFn := func(msg *message.Message, rawDataLen int) {
				gotContent = append(gotContent, string(msg.GetContent()))
				gotLens = append(gotLens, rawDataLen)
			}
			framer := NewFramer(outputFn, framing, contentLenLimit)
			for _, chunk := range chunks {
				logMessage := message.NewMessage(chunk, nil, "", 0)
				framer.Process(logMessage)
			}
			require.Equal(t, lines, gotContent)
			require.Equal(t, rawLens, gotLens)
			require.Equal(t, int64(len(lines)), framer.GetFrameCount())
		}
	}

	t.Run("UTF8", func(t *testing.T) {
		utf8 := []byte("line1\nline2\nline3\nline4\n")
		lines := []string{"line1", "line2", "line3", "line4"}
		lens := []int{6, 6, 6, 6}
		framing := UTF8Newline
		t.Run("one chunk", test(framing, chunk(utf8, len(utf8)), lines, lens))
		t.Run("one-line chunks", test(framing, chunk(utf8, 6), lines, lens))
		t.Run("two-line chunks", test(framing, chunk(utf8, 12), lines, lens))
		t.Run("one-byte chunks", test(framing, chunk(utf8, 1), lines, lens))
	})

	t.Run("SHIFTJIS", func(t *testing.T) {
		shiftjis := []byte("line1-\x93\xfa\x96{\nline2-\x93\xfa\x96{\nline3-\x93\xfa\x96{\nline4-\x93\xfa\x96{\n")
		lines := []string{"line1-\x93\xfa\x96{", "line2-\x93\xfa\x96{", "line3-\x93\xfa\x96{", "line4-\x93\xfa\x96{"}
		lens := []int{11, 11, 11, 11}
		framing := SHIFTJISNewline
		t.Run("one chunk", test(framing, chunk(shiftjis, len(shiftjis)), lines, lens))
		t.Run("one-line chunks", test(framing, chunk(shiftjis, 11), lines, lens))
		t.Run("two-line chunks", test(framing, chunk(shiftjis, 22), lines, lens))
		t.Run("one-byte chunks", test(framing, chunk(shiftjis, 1), lines, lens))
		t.Run("two-byte chunks", test(framing, chunk(shiftjis, 2), lines, lens))
		t.Run("three-byte chunks", test(framing, chunk(shiftjis, 3), lines, lens))
	})

	t.Run("UTF-16-LE", func(t *testing.T) {
		utf16 := []byte("l\x00i\x00n\x00e\x001\x00\n\x00l\x00i\x00n\x00e\x002\x00\n\x00l\x00i\x00n\x00e\x003\x00\n\x00l\x00i\x00n\x00e\x004\x00\n\x00")
		lines := []string{"l\x00i\x00n\x00e\x001\x00", "l\x00i\x00n\x00e\x002\x00", "l\x00i\x00n\x00e\x003\x00", "l\x00i\x00n\x00e\x004\x00"}
		lens := []int{12, 12, 12, 12}
		framing := UTF16LENewline
		t.Run("one chunk", test(framing, chunk(utf16, len(utf16)), lines, lens))
		t.Run("one-line chunks", test(framing, chunk(utf16, 12), lines, lens))
		t.Run("three-byte chunks", test(framing, chunk(utf16, 3), lines, lens))
		t.Run("two-byte chunks", test(framing, chunk(utf16, 2), lines, lens))
		t.Run("one-byte chunks", test(framing, chunk(utf16, 1), lines, lens))
	})

	t.Run("UTF-16-BE", func(t *testing.T) {
		utf16 := []byte("\x00l\x00i\x00n\x00e\x001\x00\n\x00l\x00i\x00n\x00e\x002\x00\n\x00l\x00i\x00n\x00e\x003\x00\n\x00l\x00i\x00n\x00e\x004\x00\n")
		lines := []string{"\x00l\x00i\x00n\x00e\x001", "\x00l\x00i\x00n\x00e\x002", "\x00l\x00i\x00n\x00e\x003", "\x00l\x00i\x00n\x00e\x004"}
		lens := []int{12, 12, 12, 12}
		framing := UTF16BENewline
		t.Run("one chunk", test(framing, chunk(utf16, len(utf16)), lines, lens))
		t.Run("one-line chunks", test(framing, chunk(utf16, 12), lines, lens))
		t.Run("three-byte chunks", test(framing, chunk(utf16, 3), lines, lens))
		t.Run("two-byte chunks", test(framing, chunk(utf16, 2), lines, lens))
		t.Run("one-byte chunks", test(framing, chunk(utf16, 1), lines, lens))
	})

	dockerChunk := func(stream byte, data []byte) []byte {
		header := [8]byte{stream}
		binary.BigEndian.PutUint32(header[4:8], uint32(len(data)))
		return append(header[:], data...)
	}

	t.Run("DockerStream(no headers)", func(t *testing.T) {
		input := []byte{}
		lines := []string{}
		lens := []int{}
		for i := 0; i < 15; i++ {
			b := []byte("noheader\n")
			input = append(input, b...)
			lines = append(lines, string(b[:len(b)-1]))
			lens = append(lens, len(b))
		}
		framing := DockerStream
		t.Run("one chunk", test(framing, chunk(input, len(input)), lines, lens))
		for size := 0; size < 20; size++ {
			t.Run(fmt.Sprintf("%d-byte chunks", size), test(framing, chunk(input, size), lines, lens))
		}
	})

	t.Run("DockerStream(headers)", func(t *testing.T) {
		input := []byte{}
		lines := []string{}
		lens := []int{}
		for i := 0; i < 15; i++ {
			b := dockerChunk(1, []byte("has-a-header\n"))
			input = append(input, b...)
			lines = append(lines, string(b[:len(b)-1]))
			lens = append(lens, len(b))
		}
		framing := DockerStream
		t.Run("one chunk", test(framing, chunk(input, len(input)), lines, lens))
		for size := 0; size < 20; size++ {
			t.Run(fmt.Sprintf("%d-byte chunks", size), test(framing, chunk(input, size), lines, lens))
		}
	})

	t.Run("DockerStream(multi-chunk-headers)", func(t *testing.T) {
		lines := []string{}
		lens := []int{}

		// all of these chunks will appear together as the first line
		input := bytes.Join([][]byte{
			dockerChunk(1, []byte("has-")),
			dockerChunk(1, []byte("a-")),
			dockerChunk(1, []byte("header-")),
			dockerChunk(1, []byte("in-")),
			dockerChunk(1, []byte("chunks\n")),
		}, []byte{})
		// ..but without the newline
		firstLine := input[:len(input)-1]
		lines = append(lines, string(firstLine))
		lens = append(lens, len(input))

		another := dockerChunk(2, []byte("another line\n"))
		input = append(input, another...)
		lines = append(lines, string(another[:len(another)-1]))
		lens = append(lens, len(another))

		framing := DockerStream
		t.Run("one chunk", test(framing, chunk(input, len(input)), lines, lens))
		for size := 0; size < 20; size++ {
			t.Run(fmt.Sprintf("%d-byte chunks", size), test(framing, chunk(input, size), lines, lens))
		}
	})

	t.Run("DockerStream(big-multi-chunk-headers)", func(t *testing.T) {
		lines := []string{}
		lens := []int{}

		// all of these chunks will appear together as the first "line"
		input := bytes.Join([][]byte{
			dockerChunk(1, bytes.Repeat([]byte{0x41}, 16384)),
			dockerChunk(1, bytes.Repeat([]byte{0x41}, 16384)),
			dockerChunk(1, bytes.Repeat([]byte{0x41}, 16384)),
			dockerChunk(1, bytes.Repeat([]byte{0x41}, 16384)),
			dockerChunk(1, []byte("the end\n")),
		}, []byte{})
		// ..but without the newline
		firstLine := input[:len(input)-1]
		lines = append(lines, string(firstLine))
		lens = append(lens, len(input))

		another := dockerChunk(2, []byte("another line\n"))
		input = append(input, another...)
		lines = append(lines, string(another[:len(another)-1]))
		lens = append(lens, len(another))

		framing := DockerStream
		t.Run("one chunk", test(framing, chunk(input, len(input)), lines, lens))
		for size := 0; size < 20; size++ {
			t.Run(fmt.Sprintf("%d-byte chunks", size), test(framing, chunk(input, size), lines, lens))
		}
	})
}

func TestContentLenLimit(t *testing.T) {
	test := func(contentLenLimit int, chunks [][]byte, lines []string, rawLens []int) func(*testing.T) {
		return func(t *testing.T) {
			gotContent := []string{}
			gotLens := []int{}
			outputFn := func(msg *message.Message, rawDataLen int) {
				gotContent = append(gotContent, string(msg.GetContent()))
				gotLens = append(gotLens, rawDataLen)
			}
			fr := NewFramer(outputFn, UTF8Newline, contentLenLimit)
			for _, chunk := range chunks {
				logMessage := message.NewMessage(chunk, nil, "", 0)
				fr.Process(logMessage)
			}
			require.Equal(t, lines, gotContent)
			require.Equal(t, rawLens, gotLens)
		}
	}

	t.Run("one-byte-less", func(t *testing.T) {
		input := []byte("abcdabcdabcdabcd\nabcdabcdabcdabcd\n") // 16 bytes + newline
		contentLenLimit := 15
		lines := []string{"abcdabcdabcdabc", "d", "abcdabcdabcdabc", "d"}
		lens := []int{15, 2, 15, 2}
		t.Run("one chunk", test(contentLenLimit, chunk(input, len(input)), lines, lens))
		t.Run("two-byte chunks", test(contentLenLimit, chunk(input, 2), lines, lens))
		t.Run("one-byte chunks", test(contentLenLimit, chunk(input, 1), lines, lens))
	})

	t.Run("exact", func(t *testing.T) {
		input := []byte("abcdabcdabcdabcd\nabcdabcdabcdabcd\n") // 16 bytes + newline
		contentLenLimit := 16
		lines := []string{"abcdabcdabcdabcd", "abcdabcdabcdabcd"}
		lens := []int{17, 17}
		t.Run("one chunk", test(contentLenLimit, chunk(input, len(input)), lines, lens))
		t.Run("two-byte chunks", test(contentLenLimit, chunk(input, 2), lines, lens))
		t.Run("one-byte chunks", test(contentLenLimit, chunk(input, 1), lines, lens))
	})

	t.Run("one-byte-more", func(t *testing.T) {
		input := []byte("abcdabcdabcdabcd\nabcdabcdabcdabcd\n") // 16 bytes + newline
		contentLenLimit := 17
		lines := []string{"abcdabcdabcdabcd", "abcdabcdabcdabcd"}
		lens := []int{17, 17}
		t.Run("one chunk", test(contentLenLimit, chunk(input, len(input)), lines, lens))
		t.Run("two-byte chunks", test(contentLenLimit, chunk(input, 2), lines, lens))
		t.Run("one-byte chunks", test(contentLenLimit, chunk(input, 1), lines, lens))
	})
}

func TestLineBreakIncomingData(t *testing.T) {
	outputFn, outputChan := framerOutput()
	framer := NewFramer(outputFn, UTF8Newline, contentLenLimit)

	var line brokenLine

	// one line in one raw should be sent
	logMessage := message.NewMessage([]byte("helloworld\n"), nil, "", 0)
	framer.Process(logMessage)
	line = <-outputChan
	assert.Equal(t, "helloworld", string(line.content))
	assert.Equal(t, len("helloworld\n"), line.rawDataLen)
	assert.Equal(t, "", framer.buffer.String())

	// multiple lines in one raw should be sent
	logMessage.SetContent([]byte("helloworld\nhowayou\ngoodandyou"))
	framer.Process(logMessage)
	l := 0
	line = <-outputChan
	l += line.rawDataLen
	assert.Equal(t, "helloworld", string(line.content))
	line = <-outputChan
	l += line.rawDataLen
	assert.Equal(t, "howayou", string(line.content))
	assert.Equal(t, len("helloworld\nhowayou\n"), l)

	framer.reset()
	l = 0

	// multiple lines in multiple rows should be sent
	logMessage.SetContent([]byte("helloworld\nthisisa"))
	framer.Process(logMessage)
	line = <-outputChan
	l += line.rawDataLen
	assert.Equal(t, "helloworld", string(line.content))
	logMessage.SetContent([]byte("longinput\nindeed"))
	framer.Process(logMessage)
	line = <-outputChan
	l += line.rawDataLen
	assert.Equal(t, "thisisalonginput", string(line.content))
	assert.Equal(t, len("helloworld\nthisisalonginput\n"), l)

	framer.reset()

	// one line in multiple rows should be sent
	logMessage.SetContent([]byte("hello world"))
	framer.Process(logMessage)
	logMessage.SetContent([]byte("!\n"))
	framer.Process(logMessage)
	line = <-outputChan
	assert.Equal(t, "hello world!", string(line.content))
	assert.Equal(t, len("hello world!\n"), line.rawDataLen)

	// excessively long line in one row should be sent by chunks
	logMessage.SetContent([]byte(strings.Repeat("a", contentLenLimit+10) + "\n"))
	framer.Process(logMessage)
	line = <-outputChan
	assert.Equal(t, contentLenLimit, len(line.content))
	assert.Equal(t, contentLenLimit, line.rawDataLen)
	line = <-outputChan
	assert.Equal(t, strings.Repeat("a", 10), string(line.content))
	assert.Equal(t, 11, line.rawDataLen)

	// excessively long line in multiple rows should be sent by chunks
	logMessage.SetContent([]byte(strings.Repeat("a", contentLenLimit-5)))
	framer.Process(logMessage)
	logMessage.SetContent([]byte(strings.Repeat("a", 15) + "\n"))
	framer.Process(logMessage)
	line = <-outputChan
	assert.Equal(t, contentLenLimit, len(line.content))
	assert.Equal(t, contentLenLimit, line.rawDataLen)
	line = <-outputChan
	assert.Equal(t, strings.Repeat("a", 10), string(line.content))
	assert.Equal(t, 11, line.rawDataLen)

	// empty lines should be sent
	logMessage.SetContent([]byte("\n"))
	framer.Process(logMessage)
	line = <-outputChan
	assert.Equal(t, "", string(line.content))
	assert.Equal(t, 1, line.rawDataLen)

	// empty message should not change anything
	logMessage.SetContent([]byte(""))
	framer.Process(logMessage)
	select {
	case <-outputChan:
		assert.Fail(t, "should not have produced a frame")
	default:
	}
}

func TestFramerInputNotDockerHeader(t *testing.T) {
	outputFn, outputChan := framerOutput()
	framer := NewFramer(outputFn, UTF8Newline, 100)

	input := []byte("hello")
	input = append(input, []byte{1, 0, 0, 0, 0, 10, 0, 0}...) // docker header
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
	logMessage := message.NewMessage(input, nil, "", 0)
	framer.Process(logMessage)

	var output brokenLine
	output = <-outputChan
	expected1 := append([]byte("hello"), []byte{1, 0, 0, 0, 0}...)
	assert.Equal(t, expected1, output.content)
	assert.Equal(t, len(expected1)+1, output.rawDataLen)

	output = <-outputChan
	expected2 := append([]byte{0, 0}, []byte("2018-06-14T18:27:03.246999277Z app logs")...)
	assert.Equal(t, expected2, output.content)
	assert.Equal(t, len(expected2)+1, output.rawDataLen)
}

func TestStructuredContent(t *testing.T) {
	// structured log messages must not be processed by the framer
	outputFn, outputChan := framerOutput()
	framer := NewFramer(outputFn, UTF8Newline, 100)

	content := append(
		[]byte{1, 0, 0, 0, 0, 10, 0, 0},
		[]byte("2018-06-14T18:27:03.246999277Z\n app logs\n")...,
	)

	entry := message.BasicStructuredContent{
		Data: make(map[string]interface{}),
	}
	entry.SetContent(content)
	message := message.NewStructuredMessage(&entry, nil, "", 111)

	framer.Process(message)

	// only one output and it must contain everything
	assert.Len(t, outputChan, 1)
	output := <-outputChan
	assert.Equal(t, output.content, content)
	assert.Equal(t, output.rawDataLen, len(content))

	assert.Len(t, outputChan, 0)
}
