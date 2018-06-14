// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"bytes"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Length of the docker message header.
// See https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs:
// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
const messageHeaderLength = 8
const maxDockerBufferSize = 16 * 1024

// ParseMessage extracts the date and the status from the raw docker message
// see https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs
func ParseMessage(msg []byte) (string, string, []byte, error) {

	// The format of the message should be :
	// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
	// If we don't have at the very least 8 bytes we can consider this message can't be parsed.
	if len(msg) < messageHeaderLength {
		return "", "", nil, errors.New("Can't parse docker message: expected a 8 bytes header")
	}

	preMessageLength := getPreMessageLength(msg)
	if len(msg) < preMessageLength {
		return "", "", nil, errors.New("can't parse docker message: expected 8 byte header, timestamp, then a space before message")
	}
	// if len(msg[preMessageLength:]) > maxDockerBufferSize {
	// 	msg = removePartialHeaders(msg, preMessageLength)
	// }

	msg = removePartialHeaders(msg)

	// First byte is 1 for stdout and 2 for stderr
	status := message.StatusInfo
	if msg[0] == 2 {
		status = message.StatusError
	}

	// timestamp goes from byte 8 till first space
	to := bytes.Index(msg[messageHeaderLength:], []byte{' '})
	if to == -1 {
		return "", "", nil, errors.New("Can't parse docker message: expected a whitespace after header")
	}
	to += messageHeaderLength
	ts := string(msg[messageHeaderLength:to])

	return ts, status, msg[to+1:], nil

}

// getPreMessageLength finds length of the 8 byte header, timestamp, and space
// the is in front of each 16Kb chunk of message
func getPreMessageLength(msg []byte) int {
	idx := bytes.Index(msg, []byte{' '})
	if idx == -1 {
		return 0
	}
	return idx + 1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// removePartialHeaders removes the 8 byte header, timestamp, and space
// that occurs between each 16Kb section of a log greater than 16 Kb in length.
// If a docker log is greater than 16Kb, each 16Kb section "PartialMessage" will
// have a header, timestamp, and space in front of it.  For illustration,
// let's call this "HeadTs ".  For example, a message that is 35kb will be of the form:
// `HeadTs PartialMessageHeadTs PartialMessageHeadTs PartialMessage`
// This function removes the "HeadTs " between two PartialMessage sections while
// leaving the very first "HeadTs "
func removePartialHeaders(msg []byte) []byte {

	preMessageLength := getPreMessageLength(msg)
	if len(msg[preMessageLength:]) < maxDockerBufferSize {
		return msg[:preMessageLength+len(msg[preMessageLength:])]
	}
	removed := msg[:preMessageLength+maxDockerBufferSize]
	msg = msg[preMessageLength+maxDockerBufferSize:]

	preMessageLength = getPreMessageLength(msg)

	nextPartialMessageSize := min(len(msg), maxDockerBufferSize+preMessageLength)

	for nextPartialMessageSize > 0 {
		removed = append(removed, msg[preMessageLength:nextPartialMessageSize]...)
		msg = msg[nextPartialMessageSize:]
		preMessageLength = getPreMessageLength(msg)
		nextPartialMessageSize = min(len(msg), maxDockerBufferSize+preMessageLength)

	}
	return removed
}
