// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// test that m matches a line ending at offset endLineAt, and nowhere else,
// with any distribution of bytes between `existing` and `appender`.
func testMatchAt(t *testing.T, m EndLineMatcher, data []byte, nextLineStartsAt int) {
	// offset is the location we will look for the next line to begin
	for offset := 0; offset <= len(data); offset++ {
		// ex is the number of existing bytes
		for ex := 0; ex < offset-1; ex++ {
			existing := data[:ex]
			// ap is the beginning of the appender slice, which can contain
			// existing bytes as well, as offset by `start`
			for ap := 0; ap < ex; ap++ {
				appender := data[ap:]
				start := ex - ap
				end := offset - ap - 1 // account for "end+1" instead of "end"

				require.Equal(t,
					offset == nextLineStartsAt, m.Match(existing, appender, start, end),
					"match(%#v, %#v, %d, %d)", existing, appender, start, end)
			}
		}
	}
}

func TestNewLineMatcher_Match(t *testing.T) {
	testMatchAt(t, &NewLineMatcher{}, []byte("abcd\n1234"), 5)
}

func TestNewLineMatcher_SeparatorLen(t *testing.T) {
	nlm := &NewLineMatcher{}
	require.Equal(t, 1, nlm.SeparatorLen())
}

func TestBytesSequenceMatcher_Match(t *testing.T) {
	input := []byte{
		0xff, 0xfe, // BOM
		0x32, 0x00, 0x30, 0x00, 0x32, 0x00, 0x31, 0x00, 0x0a, 0x00, // "2021\n"
		0x41, 0x00, 0x40, 0x00, 0x44, 0x00, // "BAD" (no newline)
	}
	testMatchAt(t, NewBytesSequenceMatcher([]byte{0x0a, 0x00}, 2), input, 12)
}

func TestBytesSequenceMatcher_Match_OneByte(t *testing.T) {
	testMatchAt(t, NewBytesSequenceMatcher([]byte("\n"), 1), []byte("abcd\n1234"), 5)
}

func TestBytesSequenceMatcher_Match_Misaligned(t *testing.T) {
	input := []byte{
		0x42, 0x00, // B
		0x41, 0x00, // A
		0x44, 0x00, // D
		0x70, 0x0a, // ਇ  // {0x0a, 0x00} is here, at an odd offset
		0x00, 0x01, // Ā
		0x44, 0x00, // D
		0x4f, 0x00, // O
		0x47, 0x00, // G
		0x0a, 0x00, // \n
	}
	testMatchAt(t, NewBytesSequenceMatcher([]byte{0x0a, 0x00}, 2), input, 18)
}

func TestBytesSequenceMatcher_SeparatorLen(t *testing.T) {
	nlm := NewBytesSequenceMatcher([]byte{0x0a, 0x00}, 2)
	require.Equal(t, 2, nlm.SeparatorLen())
}
