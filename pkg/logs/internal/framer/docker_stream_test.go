// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// This file contains additional tests for line-breaking specifically relating
// to DockerStream, and came with the move of this functionality from
// ./pkg/logs/tailers/docker/matcher.go.  Some are redundant with
// other tests in this package.

func getDummyHeader(i int) []byte {
	hdr := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	hdr[i] = 10
	return hdr
}

func dummyHeaderContent(tenPos int, data []byte) []byte {
	hdr := getDummyHeader(tenPos)
	hdr = append(hdr, data...)
	return hdr
}

func TestDetectDockerHeader(t *testing.T) {
	gotContent := []string{}
	gotLens := []int{}
	outputFn := func(msg *message.Message, rawDataLen int) {
		gotContent = append(gotContent, string(msg.GetContent()))
		gotLens = append(gotLens, rawDataLen)
	}

	fr := NewFramer(outputFn, DockerStream, 256000)

	for i := 4; i < 8; i++ {
		input := []byte("hello\n")
		input = append(input, getDummyHeader(i)...) // docker header
		input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
		msg := message.NewMessage(input, nil, "", 0)
		fr.Process(msg)
	}
	assert.Equal(t, []string{
		"hello",
		string(dummyHeaderContent(4, []byte("2018-06-14T18:27:03.246999277Z app logs"))),
		"hello",
		string(dummyHeaderContent(5, []byte("2018-06-14T18:27:03.246999277Z app logs"))),
		"hello",
		string(dummyHeaderContent(6, []byte("2018-06-14T18:27:03.246999277Z app logs"))),
		"hello",
		string(dummyHeaderContent(7, []byte("2018-06-14T18:27:03.246999277Z app logs"))),
	}, gotContent)
	assert.Equal(t, []int{6, 48, 6, 48, 6, 48, 6, 48}, gotLens)
}

func TestDetectMultipleDockerHeader(t *testing.T) {
	gotContent := []string{}
	gotLens := []int{}
	outputFn := func(msg *message.Message, rawDataLen int) {
		gotContent = append(gotContent, string(msg.GetContent()))
		gotLens = append(gotLens, rawDataLen)
	}

	fr := NewFramer(outputFn, DockerStream, 256000)

	var input []byte
	for i := 0; i < 100; i++ {
		input = append(input, getDummyHeader(4+i%4)...) // docker header
		input = append(input, []byte(fmt.Sprintf("2018-06-14T18:27:03.246999277Z app logs %d\n", i))...)
	}
	msg := message.NewMessage(input, nil, "", 0)
	fr.Process(msg)

	for i := 0; i < 100; i++ {
		data := []byte(fmt.Sprintf("2018-06-14T18:27:03.246999277Z app logs %d", i))
		assert.Equal(t, dummyHeaderContent(4+i%4, data), []byte(gotContent[i]))
		assert.Equal(t, len(data)+8+1, gotLens[i])
	}
}

func TestDetectMultipleDockerHeaderOnAChunkedLine(t *testing.T) {
	gotContent := []string{}
	gotLens := []int{}
	outputFn := func(message *message.Message, rawDataLen int) {
		gotContent = append(gotContent, string(message.GetContent()))
		gotLens = append(gotLens, rawDataLen)
	}

	fr := NewFramer(outputFn, DockerStream, 256000)

	var input []byte
	longestChunk := strings.Repeat("A", 16384)
	input = append(input, getDummyHeader(5)...)
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z "+longestChunk)...)
	input = append(input, getDummyHeader(6)...)
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z "+longestChunk)...)
	input = append(input, getDummyHeader(7)...)
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z the end\n")...)
	l1 := len(input)
	input = append(input, getDummyHeader(5)...)
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z "+longestChunk)...)
	input = append(input, getDummyHeader(6)...)
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z "+longestChunk)...)
	input = append(input, getDummyHeader(7)...)
	input = append(input, []byte("2018-06-14T18:27:03.246999277Z the very end\n")...)
	l2 := len(input)

	logMessage := message.NewMessage(input, nil, "", 0)
	fr.Process(logMessage)

	assert.Equal(t, []string{
		string(input[:l1-1]),
		string(input[l1 : l2-1]),
	}, gotContent)
	assert.Equal(t, []int{l1, l2 - l1}, gotLens)
}

func TestDecoderNoNewLineBeforeDockerHeader(t *testing.T) {
	gotContent := []string{}
	gotLens := []int{}
	outputFn := func(msg *message.Message, rawDataLen int) {
		gotContent = append(gotContent, string(msg.GetContent()))
		gotLens = append(gotLens, rawDataLen)
	}

	fr := NewFramer(outputFn, DockerStream, 256000)

	for i := 4; i < 8; i++ {
		input := []byte("hello")
		input = append(input, getDummyHeader(i)...) // docker header
		input = append(input, []byte("2018-06-14T18:27:03.246999277Z app logs\n")...)
		logMessage := message.NewMessage(input, nil, "", 0)
		fr.Process(logMessage)
		assert.Equal(t, string(input[:len(input)-1]), gotContent[i-4])
		assert.Equal(t, len(input), gotLens[i-4])
	}
}
