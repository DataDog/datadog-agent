// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// FuzzFramerProcess feeds arbitrary bytes to a UTF8Newline framer.
func FuzzFramerProcess(f *testing.F) {
	f.Add([]byte("hello\nworld\n"))
	f.Add([]byte("no newline at all"))
	f.Add([]byte("\n"))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte{})
	f.Add([]byte("partial line"))

	f.Fuzz(func(t *testing.T, data []byte) {
		sink := func(_ *message.Message, _ int) {}
		fr := NewFramer(sink, UTF8Newline, 256*1024)
		fr.Process(message.NewMessage(data, nil, "", 0))
	})
}

// FuzzFramerDockerStream feeds arbitrary bytes to a DockerStream framer.
// This is the highest-risk mode: matchHeader() does complex 4-offset arithmetic,
// and checkByte() uses two-span index logic.
func FuzzFramerDockerStream(f *testing.F) {
	// Well-formed stdout frame: {1,0,0,0, size(4 bytes big-endian), payload, '\n'}
	f.Add([]byte{1, 0, 0, 0, 0, 0, 0, 5, 'h', 'e', 'l', 'l', 'o', '\n'})
	// Well-formed stderr frame
	f.Add([]byte{2, 0, 0, 0, 0, 0, 0, 6, 's', 't', 'd', 'e', 'r', 'r', '\n'})
	// Header with '\n' (0x0a) in each size byte position
	f.Add([]byte{1, 0, 0, 0, 10, 0, 0, 0, 't', 'e', 's', 't', '\n'})
	f.Add([]byte{1, 0, 0, 0, 0, 10, 0, 0, 't', 'e', 's', 't', '\n'})
	f.Add([]byte{1, 0, 0, 0, 0, 0, 10, 0, 't', 'e', 's', 't', '\n'})
	f.Add([]byte{1, 0, 0, 0, 0, 0, 0, 10, 't', '\n'})
	// Partial header only
	f.Add([]byte{1, 0, 0, 0})
	// Just a newline
	f.Add([]byte{'\n'})
	// Raw text (no docker header)
	f.Add([]byte("raw text\n"))
	// Empty
	f.Add([]byte{})
	// Multiple frames
	f.Add([]byte{1, 0, 0, 0, 0, 0, 0, 2, 'a', '\n', 1, 0, 0, 0, 0, 0, 0, 2, 'b', '\n'})

	f.Fuzz(func(t *testing.T, data []byte) {
		sink := func(_ *message.Message, _ int) {}
		fr := NewFramer(sink, DockerStream, 256*1024)
		fr.Process(message.NewMessage(data, nil, "", 0))
	})
}

// FuzzFramerTwoByteLE feeds arbitrary bytes to a UTF16LENewline framer.
// UTF-16LE newline is the two-byte sequence {0x0a, 0x00}.
func FuzzFramerTwoByteLE(f *testing.F) {
	// Well-formed UTF-16LE line: "hi\n"
	f.Add([]byte{'h', 0x00, 'i', 0x00, 0x0a, 0x00})
	// Just the two-byte newline
	f.Add([]byte{0x0a, 0x00})
	// Single byte (misaligned)
	f.Add([]byte{0x0a})
	// Empty
	f.Add([]byte{})
	// Misaligned: newline byte in odd position
	f.Add([]byte{0x00, 0x0a, 0x00})
	// BOM + content
	f.Add([]byte{0xff, 0xfe, 'A', 0x00, 0x0a, 0x00})
	// Three bytes
	f.Add([]byte{0x0a, 0x00, 0x41})

	f.Fuzz(func(t *testing.T, data []byte) {
		sink := func(_ *message.Message, _ int) {}
		fr := NewFramer(sink, UTF16LENewline, 256*1024)
		fr.Process(message.NewMessage(data, nil, "", 0))
	})
}

// FuzzFramerMultiCall splits a byte slice at an arbitrary point and calls
// Process() twice, exercising partial-frame buffering and normalizeBuffer().
func FuzzFramerMultiCall(f *testing.F) {
	f.Add([]byte("hello\nworld\n"), uint8(50))
	f.Add([]byte{1, 0, 0, 0, 0, 0, 0, 5, 'h', 'e', 'l', 'l', 'o', '\n'}, uint8(30))
	f.Add([]byte("partial line"), uint8(0))
	f.Add([]byte("partial line"), uint8(255))
	f.Add([]byte{}, uint8(128))
	f.Add([]byte("a\nb\nc\n"), uint8(100))

	f.Fuzz(func(t *testing.T, data []byte, splitPct uint8) {
		sink := func(_ *message.Message, _ int) {}
		fr := NewFramer(sink, UTF8Newline, 256*1024)

		split := 0
		if len(data) > 0 {
			split = int(splitPct) * len(data) / 255
			if split > len(data) {
				split = len(data)
			}
		}

		fr.Process(message.NewMessage(data[:split], nil, "", 0))
		fr.Process(message.NewMessage(data[split:], nil, "", 0))
	})
}
