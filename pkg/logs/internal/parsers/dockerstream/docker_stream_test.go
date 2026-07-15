// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dockerstream

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

var dockerHeader = string([]byte{1, 0, 0, 0, 0, 0, 0, 0}) + "2018-06-14T18:27:03.246999277Z"
var container1Parser = New("container_1")

func TestGetDockerSeverity(t *testing.T) {
	assert.Equal(t, message.StatusInfo, getDockerSeverity([]byte{1}))
	assert.Equal(t, message.StatusError, getDockerSeverity([]byte{2}))
	assert.Equal(t, "", getDockerSeverity([]byte{3}))
}

func TestDockerStandaloneParserShouldSucceedWithValidInput(t *testing.T) {
	validMessage := dockerHeader + " " + "anything"
	parser := New("container_1")
	logMessage := message.NewMessage([]byte(validMessage), nil, "", 0)
	msg, err := parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte("anything"), msg.GetContent())
}

func TestDockerStandaloneParserShouldHandleEmptyMessage(t *testing.T) {
	logMessage := message.NewMessage([]byte(dockerHeader), nil, "", 0)
	msg, err := container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.GetContent()))
}

func TestDockerStandaloneParserShouldHandleNewlineOnlyMessage(t *testing.T) {
	emptyContent := [3]string{"\\n", "\\r", "\\r\\n"}

	for _, em := range emptyContent {
		logMessage := message.NewMessage([]byte("2018-06-14T18:27:03.246999277Z "+em), nil, "", 0)
		msg, err := container1Parser.Parse(logMessage)
		assert.Nil(t, err)
		assert.Equal(t, 0, len(msg.GetContent()))
	}
}

func TestDockerStandaloneParserShouldHandleTtyMessage(t *testing.T) {
	logMessage := message.NewMessage([]byte("2018-06-14T18:27:03.246999277Z foo"), nil, "", 0)
	msg, err := container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte("foo"), msg.GetContent())
}

func TestDockerStandaloneParserShouldHandleEmptyTtyMessage(t *testing.T) {
	logMessage := message.NewMessage([]byte("2018-06-14T18:27:03.246999277Z"), nil, "", 0)
	msg, err := container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.GetContent()))
	logMessage.SetContent([]byte("2018-06-14T18:27:03.246999277Z "))
	msg, err = container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(msg.GetContent()))
}

func TestDockerStandaloneParserShouldFailWithInvalidInput(t *testing.T) {
	var msg []byte
	var err error

	// missing dockerHeader separator
	msg = []byte{}
	msg = append(msg, []byte{1, 0, 0, 0, 0}...)
	logMessage := message.NewMessage(msg, nil, "", 0)
	_, err = container1Parser.Parse(logMessage)
	assert.Equal(t, errors.New("cannot parse docker message for container container_1: expected a 8 bytes header"), err)

}

func TestDockerStandaloneParserHandlesMalformedLargeMessage(t *testing.T) {
	// Test case discovered by fuzzing - malformed large message that causes
	// removePartialDockerMetadata to return content shorter than header length

	// Create a message that will trigger removePartialDockerMetadata but result
	// in content that's too short. This happens when the message claims to be
	// large but doesn't have the expected partial headers structure
	header := []byte{1, 0, 0, 0, 0, 0, 0x40, 0x83} // Size = 16515

	// Create content that's larger than dockerBufferSize to trigger partial removal
	// but doesn't have valid partial headers
	content := make([]byte, dockerBufferSize+100)
	for i := range content {
		if i < len(header) {
			content[i] = header[i]
		} else {
			content[i] = byte('A' + (i % 26))
		}
	}

	// Add some bytes that will confuse the metadata parser
	content[dockerHeaderLength] = 0 // This will make getDockerMetadataLength return 0

	logMessage := message.NewMessage(content, nil, "", 0)

	// Should return error instead of panicking
	_, err := container1Parser.Parse(logMessage)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "message too short after processing")
}

func TestDockerStandaloneParserShouldRemovePartialHeaders(t *testing.T) {
	var msgToClean []byte
	var expectedMsg []byte

	// 16kb log
	msgToClean = []byte(buildPartialMessage('a', dockerBufferSize) + dockerHeader)
	expectedMsg = []byte(buildMessage('a', dockerBufferSize))
	logMessage := message.NewMessage(msgToClean, nil, "", 0)
	msg, err := container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, expectedMsg, msg.GetContent())
	assert.Equal(t, dockerBufferSize, len(msg.GetContent()))

	// over 16kb
	logMessage.SetContent([]byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50)))
	expectedMsg = []byte(buildMessage('a', dockerBufferSize) + buildMessage('b', 50))
	msg, err = container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, expectedMsg, msg.GetContent())
	assert.Equal(t, dockerBufferSize+50, len(msg.GetContent()))

	// three times over 16kb
	logMessage.SetContent([]byte(buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('a', dockerBufferSize) + buildPartialMessage('b', 50)))
	expectedMsg = []byte(buildMessage('a', 3*dockerBufferSize) + buildMessage('b', 50))
	msg, err = container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, "2018-06-14T18:27:03.246999277Z", msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, expectedMsg, msg.GetContent())
	assert.Equal(t, 3*dockerBufferSize+50, len(msg.GetContent()))
}

func buildPartialMessage(r rune, count int) string {
	return dockerHeader + " " + strings.Repeat(string(r), count)
}

func buildMessage(r rune, count int) string {
	return strings.Repeat(string(r), count)
}

// ttyTimestamp is a sample RFC3339Nano timestamp used in TTY-mode tests.
var ttyTimestamp = "2018-06-14T18:27:03.246999277Z"

// buildTTYPartialMessage returns a TTY-mode chunk as Docker would send it:
// a bare RFC3339Nano timestamp followed by a space and count repetitions of r.
// There is no 8-byte stream header.
func buildTTYPartialMessage(r rune, count int) string {
	return ttyTimestamp + " " + strings.Repeat(string(r), count)
}

func TestDockerStandaloneParserShouldHandleLargeTtyMessage(t *testing.T) {
	// A single 16KB TTY log line: no intermediate timestamp injection expected.
	input := buildTTYPartialMessage('a', dockerBufferSize)
	logMessage := message.NewMessage([]byte(input), nil, "", 0)
	msg, err := container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, ttyTimestamp, msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte(buildMessage('a', dockerBufferSize)), msg.GetContent())
	assert.Equal(t, dockerBufferSize, len(msg.GetContent()))
}

func TestDockerStandaloneParserShouldRemoveTTYPartialTimestamps(t *testing.T) {
	// A TTY log line >16KB: Docker prepends a timestamp to each 16KB chunk.
	// The parser must strip the intermediate timestamps so users see clean content.

	// two chunks: 16KB + 50 bytes
	input := buildTTYPartialMessage('a', dockerBufferSize) + buildTTYPartialMessage('b', 50)
	expectedContent := buildMessage('a', dockerBufferSize) + buildMessage('b', 50)
	logMessage := message.NewMessage([]byte(input), nil, "", 0)
	msg, err := container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, ttyTimestamp, msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte(expectedContent), msg.GetContent())
	assert.Equal(t, dockerBufferSize+50, len(msg.GetContent()))

	// three full 16KB chunks + a 50-byte tail (mirrors the non-TTY test)
	input = buildTTYPartialMessage('a', dockerBufferSize) +
		buildTTYPartialMessage('a', dockerBufferSize) +
		buildTTYPartialMessage('a', dockerBufferSize) +
		buildTTYPartialMessage('b', 50)
	expectedContent = buildMessage('a', 3*dockerBufferSize) + buildMessage('b', 50)
	logMessage.SetContent([]byte(input))
	msg, err = container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, ttyTimestamp, msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte(expectedContent), msg.GetContent())
	assert.Equal(t, 3*dockerBufferSize+50, len(msg.GetContent()))
}

// TestDockerStandaloneParserTTYPreservesSpacesInContent guards against the
// theoretical risk that removeTTYPartialTimestamps could misinterpret a space
// inside the user's log content as a chunk-boundary timestamp marker. The
// stripping operates on fixed 16KB chunk windows, not on space lookups inside
// content, so embedded spaces must round-trip untouched.
func TestDockerStandaloneParserTTYPreservesSpacesInContent(t *testing.T) {
	// Build a 16KB chunk whose content contains a space NOT at the boundary.
	// Layout: 8000 'a' + 1 space + 8383 'b' = 16384 bytes (= dockerBufferSize).
	chunk1Content := strings.Repeat("a", 8000) + " " + strings.Repeat("b", 8383)
	assert.Equal(t, dockerBufferSize, len(chunk1Content))

	// Second chunk: shorter, with its own embedded space.
	chunk2Content := "hello world tail"

	input := ttyTimestamp + " " + chunk1Content + ttyTimestamp + " " + chunk2Content
	expectedContent := chunk1Content + chunk2Content

	logMessage := message.NewMessage([]byte(input), nil, "", 0)
	msg, err := container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, ttyTimestamp, msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte(expectedContent), msg.GetContent())
	// Confirm the embedded boundary survived — would be lost if the stripper
	// used space-search inside content instead of fixed 16KB windows.
	assert.Equal(t, dockerBufferSize+len(chunk2Content), len(msg.GetContent()))
	assert.Contains(t, string(msg.GetContent()), "aaaa b")
	assert.Contains(t, string(msg.GetContent()), "hello world tail")
}

// TestDockerStandaloneParserTTYPreservesUnmatchedTail guards against the
// codex review finding: when the tail after the first 16KB chunk does NOT
// begin with a valid RFC3339Nano timestamp + space (for example a single
// non-Docker frame slightly longer than 16KB, or a truncated frame), the
// stripper must not drop the tail or misinterpret content-internal spaces
// as a chunk boundary. The previous (loose) version returned 0 for tails
// without a space and silently truncated the message.
func TestDockerStandaloneParserTTYPreservesUnmatchedTail(t *testing.T) {
	// (a) Tail with no space at all: previous code dropped it entirely.
	chunk1 := strings.Repeat("a", dockerBufferSize)
	tail := "tail-without-space" // no space, no TS, must be preserved verbatim
	input := ttyTimestamp + " " + chunk1 + tail
	expectedContent := chunk1 + tail

	logMessage := message.NewMessage([]byte(input), nil, "", 0)
	msg, err := container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, ttyTimestamp, msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte(expectedContent), msg.GetContent())
	assert.Equal(t, dockerBufferSize+len(tail), len(msg.GetContent()))

	// (b) Tail with a space but no valid timestamp before it: previous code
	// would strip the prefix up to the space as if it were a TS marker.
	tail = "hello world, no timestamp prefix"
	input = ttyTimestamp + " " + chunk1 + tail
	expectedContent = chunk1 + tail

	logMessage = message.NewMessage([]byte(input), nil, "", 0)
	msg, err = container1Parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, ttyTimestamp, msg.ParsingExtra.Timestamp)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, []byte(expectedContent), msg.GetContent())
	assert.Equal(t, dockerBufferSize+len(tail), len(msg.GetContent()))
	// Verify the leading "hello" of the tail is NOT stripped (would happen
	// if the function treated the first space as a chunk-boundary marker).
	assert.Contains(t, string(msg.GetContent()), "hello world, no timestamp prefix")
}
