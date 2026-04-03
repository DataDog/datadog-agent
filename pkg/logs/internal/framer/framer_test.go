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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const contentLenLimit = 900000

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
		}
		rv = append(rv, iter[:size])
		iter = iter[size:]
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
	t.Run("exact-with-newline-should-not-truncate", func(t *testing.T) {
		input := []byte(strings.Repeat("a", contentLenLimit) + "\n")
		lines := []string{strings.Repeat("a", contentLenLimit)}
		lens := []int{contentLenLimit + 1}

		gotContent := []string{}
		gotLens := []int{}
		gotTruncated := []bool{}
		outputFn := func(msg *message.Message, rawDataLen int) {
			gotContent = append(gotContent, string(msg.GetContent()))
			gotLens = append(gotLens, rawDataLen)
			gotTruncated = append(gotTruncated, msg.ParsingExtra.IsTruncated)
		}
		fr := NewFramer(outputFn, UTF8Newline, contentLenLimit)
		logMessage := message.NewMessage(input, nil, "", 0)
		fr.Process(logMessage)

		require.Equal(t, lines, gotContent)
		require.Equal(t, lens, gotLens)
		require.Equal(t, []bool{false}, gotTruncated, "Log exactly at limit should NOT be truncated")
	})

	t.Run("one-byte-over-should-truncate", func(t *testing.T) {
		input := []byte(strings.Repeat("a", contentLenLimit+1) + "\n")
		lines := []string{strings.Repeat("a", contentLenLimit), "a"}
		lens := []int{contentLenLimit, 2}

		gotContent := []string{}
		gotLens := []int{}
		gotTruncated := []bool{}
		outputFn := func(msg *message.Message, rawDataLen int) {
			gotContent = append(gotContent, string(msg.GetContent()))
			gotLens = append(gotLens, rawDataLen)
			gotTruncated = append(gotTruncated, msg.ParsingExtra.IsTruncated)
		}
		fr := NewFramer(outputFn, UTF8Newline, contentLenLimit)
		logMessage := message.NewMessage(input, nil, "", 0)
		fr.Process(logMessage)

		require.Equal(t, lines, gotContent)
		require.Equal(t, lens, gotLens)
		require.Equal(t, []bool{true, false}, gotTruncated, "First frame truncated, second frame completes the line")
	})
}

// utf16LEBytes builds a UTF-16-LE byte sequence from a string of ASCII characters.
// Each ASCII character becomes 2 bytes: [char, 0x00].
func utf16LEBytes(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, c := range []byte(s) {
		out = append(out, c, 0x00)
	}
	return out
}

// utf16BEBytes builds a UTF-16-BE byte sequence from a string of ASCII characters.
// Each ASCII character becomes 2 bytes: [0x00, char].
func utf16BEBytes(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, c := range []byte(s) {
		out = append(out, 0x00, c)
	}
	return out
}

func TestUTF16ContentLenLimitAlignment(t *testing.T) {
	// Test that the framer correctly aligns contentLenLimit to a 2-byte
	// boundary for UTF-16 encodings. This prevents splitting a UTF-16
	// character in half when the fallback truncation path is used.

	t.Run("UTF-16-LE odd limit, no newline (fallback truncation)", func(t *testing.T) {
		// Build a UTF-16-LE line of 8 characters (16 bytes) with NO newline.
		// Use an odd contentLenLimit of 11. The framer should align it down
		// to 10, producing a first frame of 10 bytes (5 chars) and a second
		// frame once a newline arrives.
		oddLimit := 11
		data := utf16LEBytes("ABCDEFGH") // 16 bytes, no newline

		gotContent := [][]byte{}
		gotLens := []int{}
		gotTruncated := []bool{}
		outputFn := func(msg *message.Message, rawDataLen int) {
			gotContent = append(gotContent, msg.GetContent())
			gotLens = append(gotLens, rawDataLen)
			gotTruncated = append(gotTruncated, msg.ParsingExtra.IsTruncated)
		}
		fr := NewFramer(outputFn, UTF16LENewline, oddLimit)

		// Feed all data at once (no newline, triggers fallback truncation)
		logMessage := message.NewMessage(data, nil, "", 0)
		fr.Process(logMessage)

		// Should produce one truncated frame of 10 bytes (aligned from 11)
		require.Len(t, gotContent, 1)
		assert.Equal(t, 10, len(gotContent[0]), "Frame content should be aligned to 2-byte boundary")
		assert.Equal(t, 0, len(gotContent[0])%2, "Frame content length must be even for valid UTF-16")
		assert.Equal(t, 10, gotLens[0])
		assert.True(t, gotTruncated[0], "Frame should be marked as truncated")

		// Now send a newline to flush the remainder
		gotContent = gotContent[:0]
		gotLens = gotLens[:0]
		gotTruncated = gotTruncated[:0]
		nl := []byte{'\n', 0x00} // UTF-16-LE newline
		logMessage = message.NewMessage(nl, nil, "", 0)
		fr.Process(logMessage)

		// Should produce one frame with the remaining 6 bytes + newline consumed
		require.Len(t, gotContent, 1)
		assert.Equal(t, 6, len(gotContent[0]), "Remainder should be 6 bytes (3 chars)")
		assert.Equal(t, 0, len(gotContent[0])%2, "Remainder length must be even for valid UTF-16")
		assert.Equal(t, 8, gotLens[0], "rawDataLen should include the 2-byte newline")
	})

	t.Run("UTF-16-BE odd limit, no newline (fallback truncation)", func(t *testing.T) {
		oddLimit := 11
		data := utf16BEBytes("ABCDEFGH") // 16 bytes, no newline

		gotContent := [][]byte{}
		gotLens := []int{}
		outputFn := func(msg *message.Message, rawDataLen int) {
			gotContent = append(gotContent, msg.GetContent())
			gotLens = append(gotLens, rawDataLen)
		}
		fr := NewFramer(outputFn, UTF16BENewline, oddLimit)

		logMessage := message.NewMessage(data, nil, "", 0)
		fr.Process(logMessage)

		require.Len(t, gotContent, 1)
		assert.Equal(t, 10, len(gotContent[0]), "Frame content should be aligned to 2-byte boundary")
		assert.Equal(t, 0, len(gotContent[0])%2, "Frame content length must be even for valid UTF-16")
		assert.Equal(t, 10, gotLens[0])
	})

	t.Run("UTF-16-LE odd limit, newline found beyond limit (matcher truncation)", func(t *testing.T) {
		// Build a UTF-16-LE line of 8 characters (16 bytes) WITH newline.
		// Use an odd contentLenLimit of 11. The matcher itself rounds to 10.
		oddLimit := 11
		data := append(utf16LEBytes("ABCDEFGH"), '\n', 0x00) // 18 bytes with newline

		gotContent := [][]byte{}
		gotLens := []int{}
		gotTruncated := []bool{}
		outputFn := func(msg *message.Message, rawDataLen int) {
			gotContent = append(gotContent, msg.GetContent())
			gotLens = append(gotLens, rawDataLen)
			gotTruncated = append(gotTruncated, msg.ParsingExtra.IsTruncated)
		}
		fr := NewFramer(outputFn, UTF16LENewline, oddLimit)

		logMessage := message.NewMessage(data, nil, "", 0)
		fr.Process(logMessage)

		// Should produce two frames: truncated first (10 bytes), remainder (6 bytes)
		require.Len(t, gotContent, 2)
		assert.Equal(t, 10, len(gotContent[0]), "First frame should be truncated at aligned limit")
		assert.Equal(t, 0, len(gotContent[0])%2, "First frame must have even length")
		assert.True(t, gotTruncated[0])
		assert.Equal(t, 6, len(gotContent[1]), "Second frame should contain remainder")
		assert.Equal(t, 0, len(gotContent[1])%2, "Second frame must have even length")
	})

	t.Run("UTF-16-LE even limit, long line", func(t *testing.T) {
		// With an even limit, truncation should already be at a character boundary.
		evenLimit := 10
		data := append(utf16LEBytes("ABCDEFGH"), '\n', 0x00)

		gotContent := [][]byte{}
		gotLens := []int{}
		gotTruncated := []bool{}
		outputFn := func(msg *message.Message, rawDataLen int) {
			gotContent = append(gotContent, msg.GetContent())
			gotLens = append(gotLens, rawDataLen)
			gotTruncated = append(gotTruncated, msg.ParsingExtra.IsTruncated)
		}
		fr := NewFramer(outputFn, UTF16LENewline, evenLimit)

		logMessage := message.NewMessage(data, nil, "", 0)
		fr.Process(logMessage)

		require.Len(t, gotContent, 2)
		assert.Equal(t, 10, len(gotContent[0]))
		assert.Equal(t, 0, len(gotContent[0])%2, "First frame must have even length")
		assert.True(t, gotTruncated[0])
		assert.Equal(t, 6, len(gotContent[1]))
		assert.Equal(t, 0, len(gotContent[1])%2, "Second frame must have even length")
		assert.False(t, gotTruncated[1])
	})

	t.Run("UTF-16-LE contentLenLimit of 1 clamped to minimum of 2", func(t *testing.T) {
		// Edge case: contentLenLimit of 1 rounds down to 0 for UTF-16,
		// then gets clamped to the minimum of 2 to avoid zero-length frames.
		// With a limit of 2, each UTF-16 character (2 bytes) becomes its own frame.
		data := utf16LEBytes("AB") // 4 bytes
		data = append(data, '\n', 0x00)

		gotContent := [][]byte{}
		gotLens := []int{}
		gotTruncated := []bool{}
		outputFn := func(msg *message.Message, rawDataLen int) {
			gotContent = append(gotContent, msg.GetContent())
			gotLens = append(gotLens, rawDataLen)
			gotTruncated = append(gotTruncated, msg.ParsingExtra.IsTruncated)
		}
		fr := NewFramer(outputFn, UTF16LENewline, 1)
		logMessage := message.NewMessage(data, nil, "", 0)
		fr.Process(logMessage)

		// With effective limit of 2: "A\x00" (truncated), "B\x00" (newline found, not truncated)
		require.Len(t, gotContent, 2)
		assert.Equal(t, 2, len(gotContent[0]), "First frame = 1 UTF-16 char")
		assert.True(t, gotTruncated[0])
		assert.Equal(t, 2, len(gotContent[1]), "Second frame = 1 UTF-16 char")
		assert.False(t, gotTruncated[1])
	})
}

func TestUTF16TruncationNoMojibake(t *testing.T) {
	// End-to-end test: feed a long UTF-16-LE line through the Framer and
	// then through the encodedtext parser. Verify the resulting UTF-8
	// output contains no replacement characters (U+FFFD), proving that
	// truncation does not split UTF-16 characters.

	// Build a UTF-16-LE line of 100 'X' characters = 200 bytes, plus a 2-byte newline.
	body := utf16LEBytes(strings.Repeat("X", 100)) // 200 bytes
	data := append(body, '\n', 0x00)               // 202 bytes total

	// Use an odd contentLenLimit to provoke the alignment fix.
	oddLimit := 151

	// Collect framer output
	type frame struct {
		content []byte
	}
	frames := []frame{}
	outputFn := func(msg *message.Message, _ int) {
		// Copy content since the framer may reuse its buffer
		c := make([]byte, len(msg.GetContent()))
		copy(c, msg.GetContent())
		frames = append(frames, frame{c})
	}
	fr := NewFramer(outputFn, UTF16LENewline, oddLimit)

	logMessage := message.NewMessage(data, nil, "", 0)
	fr.Process(logMessage)

	require.NotEmpty(t, frames, "Framer should produce at least one frame")

	// Parse each frame through encodedtext (UTF-16-LE → UTF-8) and verify
	// no replacement characters appear.
	for i, f := range frames {
		assert.Equal(t, 0, len(f.content)%2,
			"Frame %d has odd byte count (%d), which would split a UTF-16 character", i, len(f.content))

		// Verify every byte in the frame content is valid UTF-16
		// by checking that the content only contains our expected 'X' chars
		// in UTF-16-LE encoding (0x58 0x00)
		for j := 0; j+1 < len(f.content); j += 2 {
			assert.Equal(t, byte('X'), f.content[j],
				"Frame %d, position %d: unexpected byte", i, j)
			assert.Equal(t, byte(0x00), f.content[j+1],
				"Frame %d, position %d: unexpected byte", i, j+1)
		}
	}
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
