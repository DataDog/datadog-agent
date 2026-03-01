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
	// Octet-counted message exceeding content limit gets truncated.
	limit := 20
	msg := "<34>1 " + strings.Repeat("x", 30) // 36 bytes total
	full := []byte(fmt.Sprintf("%d %s", len(msg), msg))

	got, _ := processSyslog(t, limit, [][]byte{full})
	require.Len(t, got, 1)
	assert.Equal(t, msg[:limit], got[0])
}

func TestSyslogContentLenLimitNonTransparent(t *testing.T) {
	// Non-transparent message exceeding content limit gets truncated.
	limit := 20
	msg := "<34>1 " + strings.Repeat("x", 30) // 36 bytes
	input := []byte(msg + "\n")

	got, _ := processSyslog(t, limit, [][]byte{input})
	require.Len(t, got, 1)
	assert.Equal(t, msg[:limit], got[0])
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
	// An unexpected leading byte (not '<' or digit) should be consumed.
	input := []byte("X<34>1 host app - - - msg\n")

	got, _ := processSyslog(t, 4096, [][]byte{input})
	// The 'X' is consumed as a zero-length frame, then the real message is parsed.
	require.Equal(t, []string{"<34>1 host app - - - msg"}, got)
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
