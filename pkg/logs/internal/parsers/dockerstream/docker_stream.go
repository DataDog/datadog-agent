// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dockerstream parses the log format output by Docker when streaming
// via its API.
//
// See https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs
package dockerstream

import (
	"bytes"
	"fmt"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Length of the docker message header.
// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
const dockerHeaderLength = 8

// Docker splits logs that are larger than 16Kb
// https://github.com/moby/moby/blob/master/daemon/logger/copier.go#L19-L22
const dockerBufferSize = 16 * 1024

// Escaped CRLF, used for determine empty messages
var escapedCRLF = []byte{'\\', 'r', '\\', 'n'}

type dockerStreamFormat struct {
	containerID string
}

// New creates a new instance of docker parser for a specific container.  The given
// container ID is only used to generate error messages for invalid log data.
//
// The returned parser handles log messages as provided by the log-streaming
// API.  The format is documented at
// https://pkg.go.dev/github.com/moby/moby/client?utm_source=godoc#Client.ContainerLogs
func New(containerID string) parsers.Parser {
	return &dockerStreamFormat{
		containerID: containerID,
	}
}

// Parse implements Parser#Parse
func (p *dockerStreamFormat) Parse(msg *message.Message) (*message.Message, error) {
	return parseDockerStream(msg, p.containerID)
}

// SupportsPartialLine implements Parser#SupportsPartialLine
func (p *dockerStreamFormat) SupportsPartialLine() bool {
	return false
}

func parseDockerStream(msg *message.Message, containerID string) (*message.Message, error) {
	content := msg.GetContent()
	stream := "" // stdout or stderr
	// The format of the message should be :
	// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
	// If we don't have at the very least 8 bytes we can consider this message can't be parsed.
	if len(content) < dockerHeaderLength {
		msg.Status = message.StatusInfo
		return msg, fmt.Errorf("cannot parse docker message for container %v: expected a 8 bytes header", containerID)
	}

	// Read the first byte to get the status. Non-TTY containers prefix every
	// chunk with an 8-byte header where byte 0 is 1 (stdout) or 2 (stderr);
	// TTY containers omit this header entirely (the daemon emits raw
	// "RFC3339Nano SPACE content" instead). getDockerSeverity returns "" for
	// the TTY case, which is the gate for everything below.
	status := getDockerSeverity(content)
	if status == "" {
		// TTY path — no 8-byte header. The body looks like:
		//   RFC3339Nano SPACE content...
		// and for log lines spanning more than one 16KB Docker buffer:
		//   TS1 SPACE content1(16KB) TS2 SPACE content2(16KB) ... TSn SPACE contentn
		// (see https://github.com/moby/moby/issues/19696). When this happens
		// we need to strip the intermediate TSi SPACE markers so the caller
		// only sees the concatenated raw content. Spaces inside the user's
		// log content are preserved — the stripping operates on fixed 16KB
		// chunk boundaries, not on space lookups inside content.
		status = message.StatusInfo
		if len(content) > dockerBufferSize {
			content = removeTTYPartialTimestamps(content)
		}

	} else {
		// Non-TTY path — 8-byte stream header present. Each 16KB partial chunk
		// includes its own header + RFC3339Nano timestamp + space, handled by
		// removePartialDockerMetadata. The new TTY helper above is never
		// reached on this branch.
		if len(content) > dockerBufferSize {
			content = removePartialDockerMetadata(content)
		}

		// Check that we still have enough bytes after removePartialDockerMetadata
		if len(content) < dockerHeaderLength {
			msg.Status = status
			return msg, fmt.Errorf("cannot parse docker message for container %v: message too short after processing", containerID)
		}
		// Before removing the header, capture the stream from the first byte:
		// 1 -> stdout, 2 -> stderr
		switch content[0] {
		case 1:
			stream = "stdout"
		case 2:
			stream = "stderr"
		}
		// remove the header as we don't need it anymore
		content = content[dockerHeaderLength:]

	}

	// timestamp goes till first space
	idx := bytes.Index(content, []byte{' '})
	if idx == -1 || isEmptyMessage(content[idx+1:]) {
		// Nothing after the timestamp: empty message
		return &message.Message{}, nil
	}

	msg.ParsingExtra.Timestamp = string(content[:idx])
	msg.SetContent(content[idx+1:])
	msg.Status = status
	msg.ParsingExtra.IsPartial = false
	// Add a tag for the stream when deducible from the header byte
	if pkgconfigsetup.Datadog().GetBool("logs_config.add_logsource_tag") {
		if stream != "" {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.LogSourceTag(stream))
		}
	}
	return msg, nil
}

// getDockerSeverity returns the status of the message based on the value of the
// STREAM_TYPE byte in the header. STREAM_TYPE can be 1 for stdout and 2 for
// stderr. If it doesn't match either of these, return an empty string.
func getDockerSeverity(msg []byte) string {
	switch msg[0] {
	case 1:
		return message.StatusInfo
	case 2:
		return message.StatusError
	default:
		return ""
	}
}

// removePartialDockerMetadata removes the 8 byte header, timestamp, and space that occurs between 16Kb section of a log.
// If a docker log is greater than 16Kb, each 16Kb partial section will have a header, timestamp, and space in front of it.
// For example, a message that is 35kb will be of the form: `H M1H M2H M3` where "H" is what pre-pends each 16 Kb section.
// This function removes the "H " between two partial messages sections while leaving the very first "H ".
// Input:
//
//	H M1H M2H M3
//
// Output:
//
//	H M1M2M3
func removePartialDockerMetadata(msgToClean []byte) []byte {
	msg := []byte{}
	metadataLen := getDockerMetadataLength(msgToClean)
	start := 0
	end := min(len(msgToClean), dockerBufferSize+metadataLen)

	for end > 0 && metadataLen > 0 {
		msg = append(msg, msgToClean[start:end]...)
		msgToClean = msgToClean[end:]
		metadataLen = getDockerMetadataLength(msgToClean)
		start = metadataLen
		end = min(len(msgToClean), dockerBufferSize+metadataLen)
	}

	return msg
}

// getDockerMetadataLength returns the length of the 8 bytes header, timestamp, and space
// that is in front of each message.
func getDockerMetadataLength(msg []byte) int {
	if len(msg) < dockerHeaderLength {
		return 0
	}
	idx := bytes.Index(msg[dockerHeaderLength:], []byte{' '})
	if idx == -1 {
		return 0
	}
	return dockerHeaderLength + idx + 1
}

// removeTTYPartialTimestamps removes the timestamp and space that Docker injects
// before each 16KB chunk of content when a container runs in TTY mode.
//
// In TTY mode the 8-byte stream header is absent, so the wire format for a
// message that spans multiple 16KB buffers is:
//
//	TS1 SPACE CONTENT1(16KB) TS2 SPACE CONTENT2(16KB) ... TSn SPACE CONTENTn
//
// This function keeps the very first "TS1 SPACE" and concatenates each
// subsequent content chunk without its leading timestamp prefix:
//
//	Input:  TS1 SPACE CONTENT1 TS2 SPACE CONTENT2 TS3 SPACE CONTENT3
//	Output: TS1 SPACE CONTENT1 CONTENT2 CONTENT3
//
// If the tail after a chunk does not begin with a valid RFC3339Nano timestamp
// followed by a space (for example a single non-Docker frame longer than 16KB,
// or a truncated frame), the remainder is appended verbatim instead of being
// reinterpreted as another chunk boundary.
func removeTTYPartialTimestamps(msgToClean []byte) []byte {
	metadataLen, ok := getTTYMetadataLength(msgToClean)
	if !ok {
		return msgToClean
	}

	msg := []byte{}
	start := 0
	end := min(len(msgToClean), dockerBufferSize+metadataLen)

	for end > 0 {
		msg = append(msg, msgToClean[start:end]...)
		msgToClean = msgToClean[end:]
		if len(msgToClean) == 0 {
			break
		}
		metadataLen, ok = getTTYMetadataLength(msgToClean)
		if !ok {
			// Remainder doesn't look like another Docker chunk boundary.
			// Append the unmatched tail unchanged rather than stripping
			// bytes that might be part of the user's log content.
			msg = append(msg, msgToClean...)
			break
		}
		start = metadataLen
		end = min(len(msgToClean), dockerBufferSize+metadataLen)
	}

	return msg
}

// getTTYMetadataLength returns the length of the timestamp and trailing space
// that Docker prepends to each TTY-mode buffer chunk. In TTY mode there is no
// 8-byte stream header, so the metadata is just the RFC3339Nano timestamp
// followed by a single space character.
//
// Returns (metadataLen, true) when the message begins with a valid
// RFC3339Nano timestamp + space, and (0, false) otherwise. The second
// return guards against treating ordinary user content (which may contain
// spaces but does not start with a parseable timestamp) as a chunk
// boundary.
func getTTYMetadataLength(msg []byte) (int, bool) {
	idx := bytes.Index(msg, []byte{' '})
	if idx == -1 {
		return 0, false
	}
	if _, err := time.Parse(time.RFC3339Nano, string(msg[:idx])); err != nil {
		return 0, false
	}
	return idx + 1, true
}

// isEmptyMessage tests if the entire message is in the form of escaped new line
// i.e. \\n  or \\r or \\r\\n
func isEmptyMessage(content []byte) bool {
	if len(content) == 2 && content[0] == '\\' {
		switch content[1] {
		case 'n', 'r':
			return true
		}
	}
	return bytes.Equal(content, escapedCRLF)
}
