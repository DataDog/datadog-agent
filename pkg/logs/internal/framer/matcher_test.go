// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// test that m matches a line with content of length contentLen and rawDataLen as given,
// for all values of `seen` up to `rawDataLen-1`.
func testFindFrame(t *testing.T, m FrameMatcher, data []byte, contentLen, rawDataLen int) {
	expContent := data[:contentLen]
	for seen := 0; seen < rawDataLen; seen++ {
		gotContent, gotRawDataLen := m.FindFrame(data, seen)
		assert.Equal(t, expContent, gotContent, "for seen=%d", seen)
		assert.Equal(t, rawDataLen, gotRawDataLen, "for seen=%d", seen)
	}
}

func TestOneByteNewLineMatcher_FindFrame(t *testing.T) {
	testFindFrame(t, &oneByteNewLineMatcher{contentLenLimit: 100}, []byte("abcd\n1234"), 4, 5)
}

func TestOneByteNewLineMatcher_FindFrame_none(t *testing.T) {
	content, rawDataLen := (&oneByteNewLineMatcher{contentLenLimit: 100}).FindFrame([]byte("abcd1234"), 0)
	assert.Nil(t, content)
	assert.Equal(t, 0, rawDataLen)
}

func TestOneByteNewLineMatcher_FindFrame_cll(t *testing.T) {
	content, rawDataLen := (&oneByteNewLineMatcher{contentLenLimit: 10}).FindFrame([]byte("abcd1234abcd1234\n"), 0)
	assert.Equal(t, []byte("abcd1234ab"), content)
	assert.Equal(t, 10, rawDataLen)
}

func TestTwoByteNewlineMatcher_FindFrame_UTF16LE(t *testing.T) {
	input := []byte{
		0xff, 0xfe, // BOM
		0x32, 0x00, 0x30, 0x00, 0x32, 0x00, 0x31, 0x00, 0x0a, 0x00, // "2021\n"
		0x41, 0x00, 0x40, 0x00, 0x44, 0x00, // "BAD" (no newline)
	}
	testFindFrame(t, &twoByteNewLineMatcher{contentLenLimit: 100, newline: Utf16leEOL}, input, 10, 12)
}

func TestTwoByteNewlineMatcher_FindFrame_UTF16BE(t *testing.T) {
	input := []byte{
		0xfe, 0xff, // BOM
		0x00, 0x32, 0x00, 0x30, 0x00, 0x32, 0x00, 0x31, 0x00, 0x0a, // "2021\n"
		0x00, 0x41, 0x00, 0x40, 0x00, 0x44, // "BAD" (no newline)
	}
	testFindFrame(t, &twoByteNewLineMatcher{contentLenLimit: 100, newline: Utf16beEOL}, input, 10, 12)
}

func TestTwoByteNewlineMatcher_FindFrame_cll(t *testing.T) {
	input := []byte{
		0xfe, 0xff, // BOM
		0x00, 0x32, 0x00, 0x30, 0x00, 0x32, 0x00, 0x31, 0x00, 0x0a, // "2021\n"
		0x00, 0x41, 0x00, 0x40, 0x00, 0x44, // "BAD" (no newline)
	}
	m := &twoByteNewLineMatcher{contentLenLimit: 6, newline: Utf16beEOL}
	content, rawDataLen := m.FindFrame(input, 0)
	assert.Equal(t, input[:6], content)
	assert.Equal(t, 6, rawDataLen)
}

func TestTwoByteNewlineMatcher_FindFrame_cll_odd(t *testing.T) {
	input := []byte{
		0xfe, 0xff, // BOM
		0x00, 0x32, 0x00, 0x30, 0x00, 0x32, 0x00, 0x31, 0x00, 0x0a, // "2021\n"
		0x00, 0x41, 0x00, 0x40, 0x00, 0x44, // "BAD" (no newline)
	}
	m := &twoByteNewLineMatcher{contentLenLimit: 7, newline: Utf16beEOL}
	content, rawDataLen := m.FindFrame(input, 0)
	assert.Equal(t, input[:6], content) // 7 rounded down to 6
	assert.Equal(t, 6, rawDataLen)
}

func TestTwoByteNewlineMatcher_FindFrame_Misaligned(t *testing.T) {
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
	testFindFrame(t, &twoByteNewLineMatcher{contentLenLimit: 100, newline: Utf16leEOL}, input, 16, 18)
}
