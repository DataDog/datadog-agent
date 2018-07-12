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

// Message represents a docker message
type Message struct {
	Content   []byte
	Status    string
	Timestamp string
}

// Length of the docker message header.
// See https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs:
// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
const dockerHeaderLength = 8

// Docker splits logs that are larger than 16Kb
// https://github.com/moby/moby/blob/master/daemon/logger/copier.go#L19-L22
const dockerBufferSize = 16 * 1024

// ParseMessage extracts the date and the status from the raw docker message
// see https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs
func ParseMessage(msg []byte) (Message, error) {

	// The format of the message should be :
	// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
	// If we don't have at the very least 8 bytes we can consider this message can't be parsed.
	if len(msg) < dockerHeaderLength {
		return Message{}, errors.New("Can't parse docker message: expected a 8 bytes header")
	}

	// remove partial headers that are added by docker when the message gets too long.
	if len(msg) > dockerBufferSize {
		msg = removePartialDockerMetadata(msg)
	}

	// First byte is 1 for stdout and 2 for stderr
	status := message.StatusInfo
	if msg[0] == 2 {
		status = message.StatusError
	}

	// timestamp goes from byte 8 till first space
	metadataLen := GetDockerMetadataLength(msg)
	if metadataLen == 0 {
		return Message{}, errors.New("Can't parse docker message: expected a whitespace after header")
	}
	timestamp := string(msg[dockerHeaderLength : metadataLen-1])

	return Message{
		Content:   msg[metadataLen:],
		Status:    status,
		Timestamp: timestamp,
	}, nil
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
	metadataLen := GetDockerMetadataLength(msgToClean)
	start := 0
	end := min(len(msgToClean), dockerBufferSize+metadataLen)

	for end > 0 && metadataLen > 0 {
		msg = append(msg, msgToClean[start:end]...)
		msgToClean = msgToClean[end:]
		metadataLen = GetDockerMetadataLength(msgToClean)
		start = metadataLen
		end = min(len(msgToClean), dockerBufferSize+metadataLen)
	}

	return msg
}

// GetDockerMetadataLength returns the length of the 8 bytes header, timestamp, and space
// that is in front of each message.
func GetDockerMetadataLength(msg []byte) int {
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
