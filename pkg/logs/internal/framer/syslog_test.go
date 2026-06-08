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
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// processSyslog feeds chunks through a SyslogFraming framer and collects non-empty output.
// Empty frames (from stray delimiters/unexpected bytes) are filtered, matching the
// behavior of forwardMessages which skips zero-length content.
func processSyslog(t *testing.T, limit int, chunks [][]byte) (contents []string, rawLens []int) {
	t.Helper()
	outputFn := func(msg *message.Message, rawDataLen int) {
		if len(msg.GetContent()) > 0 {
			contents = append(contents, string(msg.GetContent()))
			rawLens = append(rawLens, rawDataLen)
		}
	}
	fr := NewFramer(outputFn, SyslogFraming, limit)
	for _, c := range chunks {
		logMessage := message.NewMessage(c, nil, "", 0)
		fr.Process(logMessage)
	}
	fr.Flush()
	return
}

func TestSyslogNonTransparentFraming(t *testing.T) {
	// Simple LF-delimited syslog messages.
	msg1 := "<34>1 2024-01-01T00:00:00Z host app - - - hello"
	msg2 := "<34>1 2024-01-01T00:00:01Z host app - - - world"
	input := []byte(msg1 + "\n" + msg2 + "\n")
	wantContent := []string{msg1, msg2}
	wantLens := []int{len(msg1) + 1, len(msg2) + 1}

	t.Run("one chunk", func(t *testing.T) {
		got, lens := processSyslog(t, 4096, [][]byte{input})
		require.Equal(t, wantContent, got)
		require.Equal(t, wantLens, lens)
	})

	t.Run("one-byte chunks", func(t *testing.T) {
		chunks := make([][]byte, len(input))
		for i, b := range input {
			chunks[i] = []byte{b}
		}
		got, lens := processSyslog(t, 4096, chunks)
		require.Equal(t, wantContent, got)
		require.Equal(t, wantLens, lens)
	})
}

func TestSyslogNonTransparentNULDelimiter(t *testing.T) {
	// NUL-delimited frames (RFC 6587 §3.4.2 alternative trailer).
	msg1 := "<34>1 2024-01-01T00:00:00Z host app - - - hello"
	msg2 := "<34>1 2024-01-01T00:00:01Z host app - - - world"
	input := []byte(msg1 + "\x00" + msg2 + "\x00")

	got, lens := processSyslog(t, 4096, [][]byte{input})
	require.Equal(t, []string{msg1, msg2}, got)
	require.Equal(t, []int{len(msg1) + 1, len(msg2) + 1}, lens)
}

func TestSyslogNonTransparentCRLF(t *testing.T) {
	// CR+LF trailing should be trimmed from content.
	msg := "<34>1 2024-01-01T00:00:00Z host app - - - hello\r"
	input := []byte(msg + "\n")

	got, _ := processSyslog(t, 4096, [][]byte{input})
	require.Equal(t, []string{"<34>1 2024-01-01T00:00:00Z host app - - - hello"}, got)
}

func TestSyslogOctetCounting(t *testing.T) {
	// Two octet-counted messages back to back.
	msg1 := "<34>1 2024-01-01T00:00:00Z host app - - - hello"
	msg2 := "<34>1 2024-01-01T00:00:01Z host app - - - world"
	input := []byte(fmt.Sprintf("%d %s%d %s", len(msg1), msg1, len(msg2), msg2))

	wantContent := []string{msg1, msg2}
	header1Len := len(fmt.Sprintf("%d ", len(msg1)))
	header2Len := len(fmt.Sprintf("%d ", len(msg2)))
	wantLens := []int{header1Len + len(msg1), header2Len + len(msg2)}

	t.Run("one chunk", func(t *testing.T) {
		got, lens := processSyslog(t, 4096, [][]byte{input})
		require.Equal(t, wantContent, got)
		require.Equal(t, wantLens, lens)
	})

	t.Run("one-byte chunks", func(t *testing.T) {
		chunks := make([][]byte, len(input))
		for i, b := range input {
			chunks[i] = []byte{b}
		}
		got, lens := processSyslog(t, 4096, chunks)
		require.Equal(t, wantContent, got)
		require.Equal(t, wantLens, lens)
	})
}

func TestSyslogMixedFraming(t *testing.T) {
	// Mix of octet-counted and non-transparent frames in one stream.
	msg1 := "<34>1 host app - - - octet-counted"
	msg2 := "<34>1 host app - - - non-transparent"

	input := []byte(fmt.Sprintf("%d %s%s\n", len(msg1), msg1, msg2))

	headerLen := len(fmt.Sprintf("%d ", len(msg1)))
	got, lens := processSyslog(t, 4096, [][]byte{input})
	require.Equal(t, []string{msg1, msg2}, got)
	require.Equal(t, []int{headerLen + len(msg1), len(msg2) + 1}, lens)
}

func TestSyslogStrayDelimiters(t *testing.T) {
	// Stray newlines/NULs between frames should be consumed silently.
	msg := "<34>1 host app - - - message"
	input := []byte("\n\n\x00\r" + msg + "\n")

	got, _ := processSyslog(t, 4096, [][]byte{input})
	require.Equal(t, []string{msg}, got)
}

func TestSyslogOctetCountingPartialBuffer(t *testing.T) {
	// Octet-counted message split across two Process calls.
	msg := "<34>1 2024-01-01T00:00:00Z host app - - - hello world"
	full := []byte(fmt.Sprintf("%d %s", len(msg), msg))

	// Split in the middle of the message body.
	split := len(full) / 2
	chunk1 := full[:split]
	chunk2 := full[split:]

	got, lens := processSyslog(t, 4096, [][]byte{chunk1, chunk2})
	require.Equal(t, []string{msg}, got)
	headerLen := len(fmt.Sprintf("%d ", len(msg)))
	require.Equal(t, []int{headerLen + len(msg)}, lens)
}

func TestSyslogOctetCountingPartialHeader(t *testing.T) {
	// Octet-count header split across chunks (e.g., "4" then "9 <34>...").
	msg := "<34>1 2024-01-01T00:00:00Z host app - - - hello"
	full := []byte(fmt.Sprintf("%d %s", len(msg), msg))

	// Split within the length digits.
	chunk1 := full[:1] // just "4"
	chunk2 := full[1:] // "9 <34>..."

	got, _ := processSyslog(t, 4096, [][]byte{chunk1, chunk2})
	require.Equal(t, []string{msg}, got)
}

func TestSyslogContentLenLimitOctetCounted(t *testing.T) {
	// Octet-counted message exceeding content limit is split into
	// bounded continuation frames with zero body data loss. The
	// MSG-LEN SP header is transport framing and is stripped from
	// emitted content.
	limit := 20
	msg := "<34>1 " + strings.Repeat("x", 30) // 36 bytes total
	header := fmt.Sprintf("%d ", len(msg))
	full := []byte(header + msg)

	got, rawLens := processSyslog(t, limit, [][]byte{full})
	require.True(t, len(got) > 1, "oversized frame should be split into multiple frames")

	// Verify zero body data loss: concatenating all emitted frames
	// reproduces the message body (header is stripped as framing).
	combined := strings.Join(got, "")
	assert.Equal(t, msg, combined)

	// First frame should be exactly limit bytes of body content.
	assert.Len(t, got[0], limit)
	headerLen := len(header)
	assert.Equal(t, headerLen+limit, rawLens[0])
}

func TestSyslogContentLenLimitNonTransparent(t *testing.T) {
	// Non-transparent message exceeding content limit is split into
	// bounded continuation frames with zero data loss.
	limit := 20
	msg := "<34>1 " + strings.Repeat("x", 30) // 36 bytes
	input := []byte(msg + "\n")

	got, rawLens := processSyslog(t, limit, [][]byte{input})
	require.True(t, len(got) > 1, "oversized frame should be split into multiple frames")

	// First frame is raw bytes from the start of the buffer.
	assert.Len(t, got[0], limit)
	assert.Equal(t, limit, rawLens[0])

	// Verify zero data loss: concatenated output reproduces the full message.
	combined := strings.Join(got, "")
	assert.Equal(t, msg, combined)
}

func TestSyslogOversizedMalformedSplit(t *testing.T) {
	// Malformed content exceeding contentLenLimit is split rather than truncated.
	// Use a limit large enough to hold the valid syslog message in one frame.
	limit := 10
	junk := strings.Repeat("Z", 25)
	validMsg := "<34>1 msg"
	input := []byte(junk + validMsg + "\n")

	tailerInfo := status.NewInfoRegistry()
	var contents []string
	var truncated []bool
	outputFn := func(msg *message.Message, _ int) {
		if len(msg.GetContent()) > 0 {
			contents = append(contents, string(msg.GetContent()))
			truncated = append(truncated, msg.ParsingExtra.IsTruncated)
		}
	}
	fr := NewSyslogFramer(outputFn, limit, tailerInfo)
	fr.Process(message.NewMessage(input, nil, "", 0))

	require.True(t, len(contents) >= 3, "expected at least 3 frames: split malformed + valid syslog, got %d: %v", len(contents), contents)

	// The last frame is the valid syslog message (fits within limit).
	assert.Equal(t, validMsg, contents[len(contents)-1])

	// Verify zero data loss for the malformed portion.
	malformedParts := contents[:len(contents)-1]
	malformedCombined := strings.Join(malformedParts, "")
	assert.Equal(t, junk, malformedCombined)

	// First chunk should be flagged as truncated.
	assert.True(t, truncated[0], "first chunk of oversized malformed frame should be truncated")

	rendered := tailerInfo.Rendered()
	oversized := rendered["Syslog Oversized Frames"]
	require.NotEmpty(t, oversized)
}

// TestSyslogNeverEmitsEmptyFrames asserts the framer skips bytes that do not
// form a frame instead of emitting zero-length content. An empty frame would be
// wrapped by the syslog parser into a structured message that the stream tailer
// forwards as a blank log (HasContent is true for any structured message), so
// the framer must never emit one. Unlike processSyslog, collectAll records
// EVERY emitted frame, including empties, so the assertion actually fails if the
// framer regresses to emitting empty frames.
func TestSyslogNeverEmitsEmptyFrames(t *testing.T) {
	collectAll := func(limit int, chunks [][]byte) (all []string) {
		outputFn := func(msg *message.Message, _ int) {
			all = append(all, string(msg.GetContent()))
		}
		fr := NewFramer(outputFn, SyslogFraming, limit)
		for _, c := range chunks {
			fr.Process(message.NewMessage(c, nil, "", 0))
		}
		fr.Flush()
		return
	}

	assertNoEmpty := func(t *testing.T, frames []string) {
		t.Helper()
		for i, f := range frames {
			require.NotEmpty(t, f, "framer emitted an empty frame at index %d: %v", i, frames)
		}
	}

	t.Run("consecutive non-transparent delimiters", func(t *testing.T) {
		frames := collectAll(4096, [][]byte{[]byte("<13>a\n\n<13>b\n")})
		assertNoEmpty(t, frames)
		assert.Equal(t, []string{"<13>a", "<13>b"}, frames)
	})

	t.Run("leading delimiter", func(t *testing.T) {
		frames := collectAll(4096, [][]byte{[]byte("\n<13>a\n")})
		assertNoEmpty(t, frames)
		assert.Equal(t, []string{"<13>a"}, frames)
	})

	t.Run("oversized non-transparent body that is an exact multiple of the limit", func(t *testing.T) {
		// Body before the delimiter is 8 bytes = 2*limit, delivered without a
		// trailing delimiter so a chunk boundary lands exactly on the body end.
		// The delimiter then arrives alone, which previously produced an empty
		// continuation frame.
		limit := 4
		body := "<1>ABCDE" // 8 bytes
		frames := collectAll(limit, [][]byte{[]byte(body), []byte("\n")})
		assertNoEmpty(t, frames)
		assert.Equal(t, body, strings.Join(frames, ""), "split chunks must reconstruct the body with no data loss")
	})
}

// TestSyslogOctetCountedStraddlesLimit covers a sub-limit octet-counted frame
// whose "MSG-LEN SP" header pushes the full frame (header + body) past
// contentLenLimit, delivered fragmented across the limit boundary. The matcher
// must strip the header and never leak it into content or corrupt the frame
// boundary by falling through to the Framer's blind guard. Because the buffer
// fills to the limit before the full body arrives, the frame is split into
// bounded chunks and flagged truncated — framing-level truncation signals a
// severe upstream error, since real syslog lines are far below the limit. The
// split chunks must still reconstruct the body with no data loss.
func TestSyslogOctetCountedStraddlesLimit(t *testing.T) {
	limit := 20
	body := strings.Repeat("x", 20) // msgLen == limit; header pushes total to 23

	// "20 " + first 17 body bytes = 20 bytes (== limit) with the body still
	// incomplete; the remaining 3 body bytes then arrive in a second chunk so a
	// chunk boundary lands inside the declared body, above the limit.
	chunk1 := []byte("20 " + body[:17])
	chunk2 := []byte(body[17:])

	var contents []string
	var truncated []bool
	outputFn := func(msg *message.Message, _ int) {
		contents = append(contents, string(msg.GetContent()))
		truncated = append(truncated, msg.ParsingExtra.IsTruncated)
	}
	tailerInfo := status.NewInfoRegistry()
	fr := NewSyslogFramer(outputFn, limit, tailerInfo)
	fr.Process(message.NewMessage(chunk1, nil, "", 0))
	fr.Process(message.NewMessage(chunk2, nil, "", 0))
	fr.Flush()

	require.NotEmpty(t, contents, "expected at least one frame")
	assert.Equal(t, body, strings.Join(contents, ""), "header must be stripped and the split chunks must reconstruct the body with no data loss")
	for i, f := range contents {
		require.NotEmpty(t, f, "framer emitted an empty frame at index %d", i)
	}
	// "never the last": every split chunk except the final one is flagged
	// truncated. The final chunk completes the declared body, so it carries no
	// trailing truncation marker.
	lastIdx := len(contents) - 1
	for i := 0; i < lastIdx; i++ {
		assert.True(t, truncated[i], "non-final chunk %d (%q) must be flagged truncated", i, contents[i])
	}
	assert.False(t, truncated[lastIdx], "final chunk (%q) must NOT be flagged truncated", contents[lastIdx])
}

func TestSyslogOversizedFlushFrame(t *testing.T) {
	t.Run("matcher dumps the whole remainder in one shot", func(t *testing.T) {
		// FlushFrame does not chunk: the Framer never leaves a remainder larger
		// than contentLenLimit buffered, so flush is a straight dump of the
		// trailing bytes. Calling it directly emits the entire buffer at once.
		limit := 10
		matcher := &syslogFrameMatcher{contentLenLimit: limit}
		buf := []byte("<134>" + strings.Repeat("A", 25)) // 30 bytes

		content, rawDataLen := matcher.FlushFrame(buf)
		require.NotNil(t, content)
		assert.Len(t, content, len(buf), "the whole remainder is emitted as a single frame")
		assert.Equal(t, len(buf), rawDataLen)

		// Nothing remains to drain.
		content, rawDataLen = matcher.FlushFrame(buf[rawDataLen:])
		assert.Nil(t, content)
		assert.Equal(t, 0, rawDataLen)
	})

	t.Run("Flush loop emits all bytes at EOF", func(t *testing.T) {
		// Use a small enough message that Process() buffers it (under limit),
		// then verify Flush emits it.
		limit := 20
		msg := "<134>hello world" // 16 bytes, under limit

		var contents []string
		var truncated []bool
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
				truncated = append(truncated, msg.ParsingExtra.IsTruncated)
			}
		}
		tailerInfo := status.NewInfoRegistry()
		fr := NewSyslogFramer(outputFn, limit, tailerInfo)
		fr.Process(message.NewMessage([]byte(msg), nil, "", 0))
		require.Empty(t, contents, "no delimiter, nothing emitted yet")

		fr.Flush()
		require.Len(t, contents, 1)
		assert.Equal(t, msg, contents[0])
		assert.False(t, truncated[0], "single flush frame should not be truncated")
	})
}

func TestSyslogFlushEmitsAllBytes(t *testing.T) {
	// Flush emits all remaining bytes at EOF, including partial
	// octet-counted frames and non-'<' prefixed content.
	limit := 4096

	t.Run("partial octet-counted frame is emitted at EOF", func(t *testing.T) {
		var contents []string
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
			}
		}
		tailerInfo := status.NewInfoRegistry()
		fr := NewSyslogFramer(outputFn, limit, tailerInfo)

		fr.Process(message.NewMessage([]byte("200 <134>partial"), nil, "", 0))
		require.Empty(t, contents)

		fr.Flush()
		require.Len(t, contents, 1, "partial octet-counted frame should now be emitted at EOF")
		assert.Equal(t, "200 <134>partial", contents[0])
	})

	t.Run("non-syslog content is emitted at EOF", func(t *testing.T) {
		var contents []string
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
			}
		}
		tailerInfo := status.NewInfoRegistry()
		fr := NewSyslogFramer(outputFn, limit, tailerInfo)

		fr.Process(message.NewMessage([]byte("just plain text"), nil, "", 0))
		require.Empty(t, contents)

		fr.Flush()
		require.Len(t, contents, 1)
		assert.Equal(t, "just plain text", contents[0])
	})
}

func TestSyslogOversizedZeroDataLoss(t *testing.T) {
	// End-to-end verification that every byte of an oversized syslog stream
	// appears in the output, regardless of framing method.
	limit := 15

	t.Run("octet-counted", func(t *testing.T) {
		body := "<34>1 " + strings.Repeat("B", 40)
		frame := fmt.Sprintf("%d %s", len(body), body)
		// Follow with a delimited message so the framer can sync.
		nextBody := "<34>1 next"
		nextMsg := nextBody + "\n"
		input := []byte(frame + nextMsg)

		got, _ := processSyslog(t, limit, [][]byte{input})
		require.True(t, len(got) >= 2, "expected split frames plus the next message")

		// Concatenating ALL output should reproduce the message bodies.
		// The octet-counting header (MSG-LEN SP) is transport framing
		// and is stripped from emitted content.
		combined := strings.Join(got, "")
		assert.Equal(t, body+nextBody, combined)
	})

	t.Run("non-transparent", func(t *testing.T) {
		body := "<34>1 " + strings.Repeat("C", 40) // 46 bytes
		input := []byte(body + "\n")

		got, _ := processSyslog(t, limit, [][]byte{input})
		combined := strings.Join(got, "")
		assert.Equal(t, body, combined)
	})
}

func TestSyslogFramingIntegrationWithFramer(t *testing.T) {
	// End-to-end test through the full Framer.Process path with various chunk sizes.
	msg1 := "<34>1 host app - - - msg1"
	msg2 := "<34>1 host app - - - msg2"
	msg3 := "<34>1 host app - - - msg3"

	// msg1: octet-counted, msg2: non-transparent LF, msg3: octet-counted
	input := []byte(fmt.Sprintf("%d %s%s\n%d %s", len(msg1), msg1, msg2, len(msg3), msg3))
	wantContent := []string{msg1, msg2, msg3}

	for _, chunkSize := range []int{1, 2, 5, 10, len(input)} {
		t.Run(fmt.Sprintf("chunk_%d", chunkSize), func(t *testing.T) {
			var chunks [][]byte
			for i := 0; i < len(input); i += chunkSize {
				end := i + chunkSize
				if end > len(input) {
					end = len(input)
				}
				chunks = append(chunks, input[i:end])
			}
			got, _ := processSyslog(t, 4096, chunks)
			require.Equal(t, wantContent, got)
		})
	}
}

func TestSyslogFrameMatcherUnexpectedByte(t *testing.T) {
	// Unexpected leading bytes before a valid frame are emitted as a single
	// malformed frame, then the real syslog message is parsed normally.
	input := []byte("X<34>1 host app - - - msg\n")

	got, _ := processSyslog(t, 4096, [][]byte{input})
	require.Equal(t, []string{"X", "<34>1 host app - - - msg"}, got)
}

func TestSyslogMalformedFrameEmission(t *testing.T) {
	t.Run("pure junk emitted as single malformed frame", func(t *testing.T) {
		tailerInfo := status.NewInfoRegistry()
		var contents []string
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
			}
		}
		fr := NewSyslogFramer(outputFn, 4096, tailerInfo)

		// "hello world" has no valid frame start and no delimiter, so the
		// matcher waits for more data. Flush emits nothing (no '<' prefix).
		// Adding a trailing newline lets the matcher emit it as one frame.
		fr.Process(message.NewMessage([]byte("hello world\n"), nil, "", 0))

		require.Equal(t, []string{"hello world"}, contents)

		rendered := tailerInfo.Rendered()
		malformed := rendered["Syslog Malformed Bytes"]
		require.NotEmpty(t, malformed)
		assert.Equal(t, "11", malformed[0])
	})

	t.Run("junk followed by valid frame resyncs at PRI", func(t *testing.T) {
		tailerInfo := status.NewInfoRegistry()
		var contents []string
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
			}
		}
		fr := NewSyslogFramer(outputFn, 4096, tailerInfo)

		validMsg := "<34>1 host app - - - real message"
		input := []byte("JUNK" + validMsg + "\n")
		fr.Process(message.NewMessage(input, nil, "", 0))

		require.Equal(t, []string{"JUNK", validMsg}, contents)

		rendered := tailerInfo.Rendered()
		malformed := rendered["Syslog Malformed Bytes"]
		require.NotEmpty(t, malformed)
		assert.Equal(t, "4", malformed[0])
	})

	t.Run("stray delimiters are not counted as malformed", func(t *testing.T) {
		tailerInfo := status.NewInfoRegistry()
		var contents []string
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
			}
		}
		fr := NewSyslogFramer(outputFn, 4096, tailerInfo)

		msg := "<34>1 host app - - - msg"
		input := []byte("\n\r\x00" + msg + "\n")
		fr.Process(message.NewMessage(input, nil, "", 0))

		require.Equal(t, []string{msg}, contents)

		rendered := tailerInfo.Rendered()
		malformed := rendered["Syslog Malformed Bytes"]
		require.NotEmpty(t, malformed)
		assert.Equal(t, "0", malformed[0])
	})

	t.Run("junk before octet-counted frame resyncs at digit+SP+PRI", func(t *testing.T) {
		tailerInfo := status.NewInfoRegistry()
		var contents []string
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
			}
		}
		fr := NewSyslogFramer(outputFn, 4096, tailerInfo)

		syslogMsg := "<34>1 host app - - - octet msg"
		octetFrame := fmt.Sprintf("%d %s", len(syslogMsg), syslogMsg)
		input := []byte("XX" + octetFrame)
		fr.Process(message.NewMessage(input, nil, "", 0))

		require.Equal(t, []string{"XX", syslogMsg}, contents)

		rendered := tailerInfo.Rendered()
		malformed := rendered["Syslog Malformed Bytes"]
		require.NotEmpty(t, malformed)
		assert.Equal(t, "2", malformed[0])
	})
}

func TestSyslogSplitTruncationFlags(t *testing.T) {
	// assertNeverTheLast checks the "never the last" contract for a stream of
	// the form [split-frame chunks..., trailing independent message]. Every
	// chunk of the split frame except its final one must be flagged truncated;
	// the split frame's final chunk and the trailing independent message must
	// not be. frameChunks is the number of chunks the oversized frame produced.
	assertNeverTheLast := func(t *testing.T, contents []string, truncated []bool, frameChunks int, trailing string) {
		t.Helper()
		require.True(t, len(contents) >= frameChunks+1,
			"expected %d split chunks plus the trailing message, got %d: %v", frameChunks, len(contents), contents)

		frameLast := frameChunks - 1
		for i := 0; i < frameLast; i++ {
			assert.True(t, truncated[i], "non-final split chunk %d (%q) should be truncated", i, contents[i])
		}
		assert.False(t, truncated[frameLast], "final split chunk (%q) should NOT be truncated (never the last)", contents[frameLast])
		assert.False(t, truncated[frameChunks], "trailing independent message (%q) should NOT be truncated", contents[frameChunks])
		assert.Equal(t, trailing, contents[frameChunks])
	}

	t.Run("octet-counted split flags every chunk except the last", func(t *testing.T) {
		limit := 15
		body := "<34>1 " + strings.Repeat("B", 40) // 46 bytes
		frame := fmt.Sprintf("%d %s", len(body), body)
		nextMsg := "<34>1 next"
		input := []byte(frame + nextMsg + "\n")

		tailerInfo := status.NewInfoRegistry()
		var contents []string
		var truncated []bool
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
				truncated = append(truncated, msg.ParsingExtra.IsTruncated)
			}
		}
		fr := NewSyslogFramer(outputFn, limit, tailerInfo)
		fr.Process(message.NewMessage(input, nil, "", 0))

		// 46-byte body at limit 15 → chunks of 15, 15, 15, 1.
		assertNeverTheLast(t, contents, truncated, 4, nextMsg)
		assert.Equal(t, body, strings.Join(contents[:4], ""), "split chunks must reconstruct the body")
	})

	t.Run("non-transparent split flags every chunk except the last", func(t *testing.T) {
		limit := 15
		body := "<34>1 " + strings.Repeat("C", 40) // 46 bytes
		nextMsg := "<34>1 next"
		input := []byte(body + "\n" + nextMsg + "\n")

		tailerInfo := status.NewInfoRegistry()
		var contents []string
		var truncated []bool
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
				truncated = append(truncated, msg.ParsingExtra.IsTruncated)
			}
		}
		fr := NewSyslogFramer(outputFn, limit, tailerInfo)
		fr.Process(message.NewMessage(input, nil, "", 0))

		// 46-byte body at limit 15 → chunks of 15, 15, 15, 1.
		assertNeverTheLast(t, contents, truncated, 4, nextMsg)
		assert.Equal(t, body, strings.Join(contents[:4], ""), "split chunks must reconstruct the body")
	})

	t.Run("malformed split flags every chunk except the last", func(t *testing.T) {
		limit := 10
		junk := strings.Repeat("Z", 25)
		validMsg := "<34>1 msg"
		input := []byte(junk + validMsg + "\n")

		tailerInfo := status.NewInfoRegistry()
		var contents []string
		var truncated []bool
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
				truncated = append(truncated, msg.ParsingExtra.IsTruncated)
			}
		}
		fr := NewSyslogFramer(outputFn, limit, tailerInfo)
		fr.Process(message.NewMessage(input, nil, "", 0))

		// 25 bytes of junk at limit 10 → chunks of 10, 10, 5.
		assertNeverTheLast(t, contents, truncated, 3, validMsg)
		assert.Equal(t, junk, strings.Join(contents[:3], ""), "split chunks must reconstruct the junk run")
	})

	t.Run("octet continuation emitted promptly", func(t *testing.T) {
		// Octet-counted frame whose declared body exceeds the limit. Because
		// MSG-LEN is the authoritative boundary, the continuation bytes are
		// emitted as raw chunks as soon as they are available — they are never
		// re-run through frame detection or deferred to Flush.
		limit := 15
		body := "<34>1 " + strings.Repeat("x", 14) // 20 bytes
		frame := fmt.Sprintf("%d %s", len(body), body)
		input := []byte(frame)

		tailerInfo := status.NewInfoRegistry()
		var contents []string
		var truncated []bool
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
				truncated = append(truncated, msg.ParsingExtra.IsTruncated)
			}
		}
		fr := NewSyslogFramer(outputFn, limit, tailerInfo)
		fr.Process(message.NewMessage(input, nil, "", 0))
		require.Len(t, contents, 2, "oversized octet frame split into two chunks during Process")
		assert.True(t, truncated[0], "first split chunk should be truncated")
		assert.False(t, truncated[1], "final chunk completes the declared body, so it is not truncated (never the last)")

		fr.Flush()
		require.Len(t, contents, 2, "nothing left to flush")

		// The header (MSG-LEN SP) is stripped; concatenated output is the body.
		combined := strings.Join(contents, "")
		assert.Equal(t, body, combined)
	})

	t.Run("octet continuation deferred to flush when under-delivered", func(t *testing.T) {
		// Declared length exceeds the bytes actually delivered. After the
		// first split chunk, the remaining declared bytes have not all arrived
		// so the continuation cannot complete during Process; Flush drains
		// what was received, still flagged truncated.
		limit := 15
		declared := 25
		body := "<34>1 " + strings.Repeat("x", 12) // 18 bytes delivered, < declared
		frame := fmt.Sprintf("%d %s", declared, body)
		input := []byte(frame)

		tailerInfo := status.NewInfoRegistry()
		var contents []string
		var truncated []bool
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
				truncated = append(truncated, msg.ParsingExtra.IsTruncated)
			}
		}
		fr := NewSyslogFramer(outputFn, limit, tailerInfo)
		fr.Process(message.NewMessage(input, nil, "", 0))
		require.Len(t, contents, 1, "only the first bounded chunk fits during Process")
		assert.True(t, truncated[0], "split chunk should be truncated")

		fr.Flush()
		require.Len(t, contents, 2, "remaining delivered bytes flushed at EOF")
		assert.False(t, truncated[1], "flushed tail is the final segment, so it is not truncated (never the last)")

		// Header is stripped; only the bytes actually delivered are emitted.
		combined := strings.Join(contents, "")
		assert.Equal(t, body, combined)
	})

	t.Run("non-split frame is not truncated", func(t *testing.T) {
		msg := "<34>1 host app - - - hello"
		input := []byte(msg + "\n")

		tailerInfo := status.NewInfoRegistry()
		var truncated []bool
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				truncated = append(truncated, msg.ParsingExtra.IsTruncated)
			}
		}
		fr := NewSyslogFramer(outputFn, 4096, tailerInfo)
		fr.Process(message.NewMessage(input, nil, "", 0))

		require.Len(t, truncated, 1)
		assert.False(t, truncated[0])
	})
}

func TestSyslogMalformedSyncHeuristic(t *testing.T) {
	t.Run("JSON on syslog port produces single frame not 13", func(t *testing.T) {
		// Previously, bare digits in JSON timestamps caused isSyslogFrameStart
		// to fire on every digit, fragmenting one JSON line into 13+ entries.
		input := []byte(`{"level":"warn","msg":"test","ts":"2026-04-20T12:00:00Z"}`)

		tailerInfo := status.NewInfoRegistry()
		var contents []string
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
			}
		}
		fr := NewSyslogFramer(outputFn, 4096, tailerInfo)
		fr.Process(message.NewMessage(input, nil, "", 0))
		fr.Flush()

		// Should produce at most 2 frames: the JSON body as malformed + possibly
		// a small tail. Previously this was 13+ fragments.
		require.LessOrEqual(t, len(contents), 2,
			"JSON line should not fragment into many entries, got %d: %v", len(contents), contents)

		combined := strings.Join(contents, "")
		assert.Equal(t, string(input), combined, "all bytes must be preserved")
	})

	t.Run("garbage before two octet-counted frames recovers both", func(t *testing.T) {
		msg1 := "<134>1 2026-04-30T12:00:00Z host app 1234 - - message one"
		msg2 := "<134>1 2026-04-30T12:00:00Z host app 1234 - - message two"
		oc1 := fmt.Sprintf("%d %s", len(msg1), msg1)
		oc2 := fmt.Sprintf("%d %s", len(msg2), msg2)
		input := []byte("GARBAGE" + oc1 + oc2)

		got, _ := processSyslog(t, 4096, [][]byte{input})
		require.Len(t, got, 3, "expected: malformed + 2 octet-counted frames")
		assert.Equal(t, "GARBAGE", got[0])
		assert.Equal(t, msg1, got[1])
		assert.Equal(t, msg2, got[2])
	})

	t.Run("garbage before non-transparent frames recovers at PRI", func(t *testing.T) {
		msg1 := "<134>1 host app - - - msg1"
		msg2 := "<134>1 host app - - - msg2"
		input := []byte("GARBAGE" + msg1 + "\n" + msg2 + "\n")

		got, _ := processSyslog(t, 4096, [][]byte{input})
		require.Len(t, got, 3)
		assert.Equal(t, "GARBAGE", got[0])
		assert.Equal(t, msg1, got[1])
		assert.Equal(t, msg2, got[2])
	})

	t.Run("digits in garbage do not trigger false sync", func(t *testing.T) {
		// "error code 42" contains digits but no "digit SP <digit" pattern.
		msg := "<34>1 host app - - - real"
		input := []byte("error code 42" + msg + "\n")

		got, _ := processSyslog(t, 4096, [][]byte{input})
		require.Len(t, got, 2)
		assert.Equal(t, "error code 42", got[0])
		assert.Equal(t, msg, got[1])
	})

	t.Run("ISO timestamp in garbage does not fragment", func(t *testing.T) {
		// The timestamp "2026-04-30T14:05:42Z" must not cause fragmentation.
		garbage := "log 2026-04-30T14:05:42Z some event"
		msg := "<34>1 host app - - - real"
		input := []byte(garbage + msg + "\n")

		got, _ := processSyslog(t, 4096, [][]byte{input})
		require.Len(t, got, 2)
		assert.Equal(t, garbage, got[0])
		assert.Equal(t, msg, got[1])
	})
}

func TestSyslogMalformedOctetCount(t *testing.T) {
	t.Run("length exceeds actual data flushes on close", func(t *testing.T) {
		// Octet count says 999 bytes but the stream has far fewer.
		// findOctetCounted returns nil (waiting for more data).
		// On connection close, Flush emits whatever was buffered.
		input := []byte("999 <134>1 short msg")
		got, _ := processSyslog(t, 4096, [][]byte{input})
		require.Len(t, got, 1, "Flush should emit the buffered data on close")
		assert.Equal(t, "999 <134>1 short msg", got[0])
	})

	t.Run("zero length skipped then next frame parsed", func(t *testing.T) {
		// Leading '0' is not in '1'-'9', so it enters scanMalformed.
		// scanMalformed scans forward and resyncs at the '<' of <134>.
		// "0 " is emitted as malformed junk, then the real message parses.
		msg := "<134>1 2024-01-01T00:00:00Z h app - - - after zero"
		input := []byte("0 " + msg + "\n")
		got, _ := processSyslog(t, 4096, [][]byte{input})
		require.Len(t, got, 2)
		assert.Equal(t, "0 ", got[0], "malformed prefix emitted as junk")
		assert.Equal(t, msg, got[1], "valid frame parsed after resync")
	})

	t.Run("overstated length consumes into next message", func(t *testing.T) {
		// The declared octet count (30) exceeds the first message's actual
		// size (12 bytes). When the second message arrives and the total
		// buffer reaches 30+ bytes past the header, findOctetCounted slices
		// buf[headerLen : headerLen+30] — blindly consuming the tail of the
		// first message AND the head of the second.
		//
		// This is RFC-correct: octet counting is the authoritative boundary
		// (RFC 6587 §3.4.1). A lying sender corrupts both messages.
		msg1Body := "<134>1 short"                                               // 12 bytes
		msg2 := "<134>1 2024-01-01T00:00:00Z host app - - - second message here" // 62 bytes
		// "30 " header = 3 bytes; payload window = bytes [3:33].
		// The window covers msg1Body (12) + \n (1) + first 17 bytes of msg2.
		stream := "30 " + msg1Body + "\n" + msg2 + "\n"
		got, _ := processSyslog(t, 4096, [][]byte{[]byte(stream)})

		require.NotEmpty(t, got)
		frame0 := got[0]
		assert.Len(t, frame0, 30, "frame should be exactly the declared octet count")
		assert.Contains(t, frame0, msg1Body, "frame includes the actual first message")
		assert.Contains(t, frame0, "<134>1 2024", "frame bleeds into the second message")

		// Whatever remains after position 33 is processed as a new frame.
		// It will be a fragment of msg2, emitted via scanMalformed or Flush.
		if len(got) > 1 {
			remainder := strings.Join(got[1:], "")
			assert.Contains(t, remainder, "second message here",
				"the tail of the second message should still be emitted")
		}
	})

	t.Run("non-numeric after digit emits whole run as malformed and resyncs", func(t *testing.T) {
		// "3X " — findOctetCounted sees '3' then 'X' (not a digit or SP), which
		// proves these bytes are not an octet-counting header. The whole run is
		// handed to scanMalformed and emitted as one coherent malformed frame
		// ("3X "); the leading digit is NOT silently dropped. Then scanMalformed
		// resyncs at <134> and the real message parses.
		msg := "<134>1 2024-01-01T00:00:00Z host app - - - hello"
		input := []byte("3X " + msg + "\n")

		tailerInfo := status.NewInfoRegistry()
		var contents []string
		outputFn := func(msg *message.Message, _ int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
			}
		}
		fr := NewSyslogFramer(outputFn, 4096, tailerInfo)
		fr.Process(message.NewMessage(input, nil, "", 0))

		require.Equal(t, []string{"3X ", msg}, contents,
			"the digit-prefixed garbage must be emitted as a single malformed frame, not have its prefix dropped")

		rendered := tailerInfo.Rendered()
		malformed := rendered["Syslog Malformed Bytes"]
		require.NotEmpty(t, malformed)
		assert.Equal(t, "3", malformed[0], "all 3 garbage bytes counted once")
	})

	t.Run("over-long digit run is malformed, not octet length", func(t *testing.T) {
		// A prefix longer than 10 digits cannot be a plausible MSG-LEN. It is
		// treated as malformed rather than silently dropping the digit run.
		digits := "123456789012" // 12 digits, no SP
		msg := "<134>1 host app - - - hi"
		input := []byte(digits + " junk" + msg + "\n")
		got, _ := processSyslog(t, 4096, [][]byte{input})
		require.NotEmpty(t, got)
		assert.Equal(t, msg, got[len(got)-1], "valid frame recovered after the over-long digit run")
		assert.Contains(t, strings.Join(got[:len(got)-1], ""), digits,
			"the over-long digit run must be emitted as malformed, not dropped")
	})
}

func TestSyslogOversizedSelfBounding(t *testing.T) {
	// collectSyslog runs a stream through a real Framer and returns every
	// emitted chunk plus its raw length and truncation flag.
	collectSyslog := func(t *testing.T, limit int, chunks [][]byte) (contents []string, rawLens []int, truncated []bool, info *status.InfoRegistry) {
		t.Helper()
		info = status.NewInfoRegistry()
		outputFn := func(msg *message.Message, rawDataLen int) {
			if len(msg.GetContent()) > 0 {
				contents = append(contents, string(msg.GetContent()))
				rawLens = append(rawLens, rawDataLen)
				truncated = append(truncated, msg.ParsingExtra.IsTruncated)
			}
		}
		fr := NewSyslogFramer(outputFn, limit, info)
		for _, c := range chunks {
			fr.Process(message.NewMessage(c, nil, "", 0))
		}
		fr.Flush()
		return
	}

	t.Run("emitted chunks never exceed the content limit", func(t *testing.T) {
		// A single huge frame must never produce content larger than the
		// limit, regardless of framing method — the buffer is self-bounded.
		limit := 16
		for _, tc := range []struct {
			name  string
			input []byte
		}{
			{"octet-counted", func() []byte {
				body := "<34>1 " + strings.Repeat("x", 500)
				return []byte(fmt.Sprintf("%d %s", len(body), body))
			}()},
			{"non-transparent", []byte("<34>1 " + strings.Repeat("y", 500) + "\n")},
			{"malformed", []byte(strings.Repeat("Z", 500) + "<34>1 ok\n")},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, _, _, _ := collectSyslog(t, limit, [][]byte{tc.input})
				for i, c := range got {
					assert.LessOrEqual(t, len(c), limit, "chunk %d (%q) exceeds the content limit", i, c)
				}
			})
		}
	})

	t.Run("octet continuation preserves embedded delimiter (no re-dispatch)", func(t *testing.T) {
		// MSG-LEN is authoritative: an embedded LF inside an oversized
		// octet-counted body is message data, not a frame boundary. The buggy
		// re-dispatch path would route the continuation through delimiter
		// detection and silently drop the LF.
		limit := 10
		body := "<34>1 ABCD\nEFGHIJKLMN" // 21 bytes, LF at index 10 (past chunk 1)
		input := []byte(fmt.Sprintf("%d %s", len(body), body))

		got, _, _, _ := collectSyslog(t, limit, [][]byte{input})

		require.Len(t, got, 3, "21-byte body at limit 10 splits into 3 size-driven chunks")
		combined := strings.Join(got, "")
		assert.Equal(t, body, combined, "embedded LF must be preserved, not consumed as a delimiter")
		assert.Contains(t, combined, "\n", "the embedded newline must survive")
	})

	t.Run("octet continuation is byte-fragmentation safe", func(t *testing.T) {
		// Same guarantee when bytes arrive one at a time.
		limit := 10
		body := "<34>1 ABCD\nEFGHIJKLMN"
		full := []byte(fmt.Sprintf("%d %s", len(body), body))
		chunks := make([][]byte, len(full))
		for i, b := range full {
			chunks[i] = []byte{b}
		}

		got, _, _, _ := collectSyslog(t, limit, chunks)
		assert.Equal(t, body, strings.Join(got, ""))
	})

	t.Run("non-transparent continuation is not re-detected as octet framing", func(t *testing.T) {
		// An oversized non-transparent frame whose continuation happens to
		// begin with a "digit SP <" sequence must not be reparsed as an
		// octet-counted frame (which would consume the wrong byte count).
		limit := 10
		body := "<34>1 AAAA5 <99>injected payload here" // no embedded delimiter
		input := []byte(body + "\n")

		got, _, _, _ := collectSyslog(t, limit, [][]byte{input})

		combined := strings.Join(got, "")
		assert.Equal(t, body, combined, "every byte of the body must be emitted verbatim")
		// Splitting is size-driven, not content-driven: all but the last
		// chunk are exactly `limit` bytes.
		for i := 0; i < len(got)-1; i++ {
			assert.Len(t, got[i], limit, "non-final chunk %d should be a full bounded chunk", i)
		}
	})

	t.Run("oversized frame is counted exactly once", func(t *testing.T) {
		limit := 10
		body := "<34>1 " + strings.Repeat("x", 40) // 46 bytes -> 5 chunks
		input := []byte(fmt.Sprintf("%d %s", len(body), body))

		got, _, _, info := collectSyslog(t, limit, [][]byte{input})
		require.Greater(t, len(got), 1, "frame should have been split")

		rendered := info.Rendered()
		oversized := rendered["Syslog Oversized Frames"]
		require.NotEmpty(t, oversized)
		assert.Equal(t, "1", oversized[0], "an oversized frame must be counted once, not once per chunk")
	})

	t.Run("malformed bytes counted exactly once", func(t *testing.T) {
		// A 25-byte malformed run split across multiple bounded chunks must
		// report exactly 25 malformed bytes. The previous re-dispatch path
		// re-counted the (shrinking) remainder on every chunk, over-counting.
		limit := 10
		junk := strings.Repeat("Z", 25)
		validMsg := "<34>1 msg"
		input := []byte(junk + validMsg + "\n")

		got, _, _, info := collectSyslog(t, limit, [][]byte{input})
		assert.Equal(t, validMsg, got[len(got)-1], "valid frame recovered after the malformed run")

		rendered := info.Rendered()
		malformed := rendered["Syslog Malformed Bytes"]
		require.NotEmpty(t, malformed)
		assert.Equal(t, "25", malformed[0], "malformed bytes must equal the malformed run length exactly")
	})
}

func TestSyslogTrimTrailer(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{"empty", []byte{}, []byte{}},
		{"no trailer", []byte("hello"), []byte("hello")},
		{"LF", []byte("hello\n"), []byte("hello")},
		{"NUL", []byte("hello\x00"), []byte("hello")},
		{"CRLF", []byte("hello\r\n"), []byte("hello")},
		{"CR only", []byte("hello\r"), []byte("hello")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := syslogTrimTrailer(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}
