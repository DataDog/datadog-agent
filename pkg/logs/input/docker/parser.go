// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build docker

package docker

import (
	"bytes"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	logParser "github.com/DataDog/datadog-agent/pkg/logs/parser"
)

// Length of the docker message header.
// See https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs:
// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
const dockerHeaderLength = 8

// Docker splits logs that are larger than 16Kb
// https://github.com/moby/moby/blob/master/daemon/logger/copier.go#L19-L22
const dockerBufferSize = 16 * 1024

// Escaped CRLF, used for determine empty messages
var escapedCRLF = []byte{'\\', 'r', '\\', 'n'}

// dockerParser is the parser for stdout/stderr docker logs
var dockerParser *parser

type parser struct {
	logParser.Parser
}

// Parse extracts the date and the status from the raw docker message
// see https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs
func (p *parser) Parse(msg []byte) (*message.Message, error) {

	// The format of the message should be :
	// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
	// If we don't have at the very least 8 bytes we can consider this message can't be parsed.
	if len(msg) < dockerHeaderLength {
		return message.NewMessage(msg, nil, message.StatusInfo), errors.New("can't parse docker message: expected a 8 bytes header")
	}

	// Read the first byte to get the status
	status := getDockerSeverity(msg)
	if status == "" {

		// When tailing logs coming from a container running with a tty, docker
		// does not add the header. In that case, the message only contains
		// the timestamp followed by whatever comes from what is running in the
		// container (and maybe stdin). As a fallback, set the status to info.
		status = message.StatusInfo

	} else {

		// remove partial headers that are added by docker when the message gets too long
		if len(msg) > dockerBufferSize {
			msg = removePartialDockerMetadata(msg)
		}

		// remove the header as we don't need it anymore
		msg = msg[dockerHeaderLength:]

	}

	// timestamp goes till first space
	idx := bytes.Index(msg, []byte{' '})
	if idx == -1 || isEmptyMessage(msg[idx+1:]) {
		// Nothing after the timestamp: empty message
		return &message.Message{}, nil
	}

	parsedMsg := message.NewMessage(msg[idx+1:], nil, status)
	parsedMsg.Timestamp = string(msg[:idx])
	return parsedMsg, nil
}

// Unwrap removes the message header of docker logs
func (p *parser) Unwrap(line []byte) ([]byte, error) {
	headerLen := getDockerMetadataLength(line)
	return line[headerLen:], nil
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
//   H M1H M2H M3
// Output:
//   H M1M2M3
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

// min returns the minimum value between a and b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
