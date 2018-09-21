// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package parser

import (
	"bytes"
	"errors"
)

// Length of the docker message header.
// See https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs:
// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
const dockerHeaderLength = 8

// Docker splits logs that are larger than 16Kb
// https://github.com/moby/moby/blob/master/daemon/logger/copier.go#L19-L22
const dockerBufferSize = 16 * 1024

const (
	// Containerd log partial flags
	logTagPartial = 'P'
	logTagFull    = 'F'
	// Containerd log stream type
	stdout = "stdout"
	stderr = "stderr"
)

// ParsedLine represents a containerd message
type ParsedLine struct {
	Content   []byte
	Severity  string
	Timestamp string
}

// Parser parse containerd messages
type Parser func([]byte) (ParsedLine, error)

// IdentityParser does nothing
func IdentityParser(msg []byte) (ParsedLine, error) {
	return ParsedLine{Content: msg, Severity: ""}, nil
}

// ContainerdParser parse log lines of containerd
// These line have the following format
// Timestamp ouputchannel partial_flag msg
// Example:
// 2018-09-20T11:54:11.753589172Z stdout F This is my message
func ContainerdParser(msg []byte) (ParsedLine, error) {
	// timestamp goes till first space
	endOfTimestampIdx := bytes.Index(msg, []byte{' '})
	if endOfTimestampIdx == -1 {
		// Nothing after the timestamp: ERROR
		return ParsedLine{}, errors.New("can't parse containerd message")
	}

	endOfLogStreamTypeIdx := bytes.Index(msg[endOfTimestampIdx+1:], []byte{' '})
	if endOfLogStreamTypeIdx == -1 {
		// Nothing after the output: ERROR
		return ParsedLine{}, errors.New("can't parse containerd message")
	}
	endOfLogStreamTypeIdx += endOfTimestampIdx + 1
	severity := getContainerdSeverity(msg[endOfTimestampIdx+1 : endOfLogStreamTypeIdx])

	endOfPartialFlagIdx := bytes.Index(msg[endOfLogStreamTypeIdx+1:], []byte{' '})
	if endOfPartialFlagIdx == -1 {
		// Nothing after the PartialFlag: empty message
		return ParsedLine{Severity: severity}, nil
	}
	endOfPartialFlagIdx += endOfLogStreamTypeIdx + 1
	if msg[endOfPartialFlagIdx-1] != byte(logTagFull) && msg[endOfPartialFlagIdx-1] != byte(logTagPartial) {
		return ParsedLine{Severity: severity}, errors.New("can't parse containerd message")
	}

	return ParsedLine{
		Content:  msg[endOfPartialFlagIdx+1:],
		Severity: severity,
	}, nil
}

// getContainerdSeverity returns the severity of the message based on the value of the
// STREAM_TYPE field in the header
func getContainerdSeverity(severity []byte) string {
	switch string(severity) {
	case stdout:
		return StatusInfo
	case stderr:
		return StatusError
	default:
		return ""
	}
}

// DockerStandaloneParser extracts the date and the status from the raw docker message
// see https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs
func DockerStandaloneParser(msg []byte) (ParsedLine, error) {

	// The format of the message should be :
	// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
	// If we don't have at the very least 8 bytes we can consider this message can't be parsed.
	if len(msg) < dockerHeaderLength {
		return ParsedLine{}, errors.New("can't parse docker message: expected a 8 bytes header")
	}

	// Read the first byte to get the status
	status := getDockerSeverity(msg)
	if status == "" {

		// When tailing logs coming from a container running with a tty, docker
		// does not add the header. In that case, the message only contains
		// the timestamp followed by whatever comes from what is running in the
		// container (and maybe stdin). As a fallback, set the status to info.
		status = StatusInfo

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
	if idx == -1 {
		// Nothing after the timestamp: empty message
		return ParsedLine{}, nil
	}

	return ParsedLine{
		Content:   msg[idx+1:],
		Severity:  status,
		Timestamp: string(msg[:idx]),
	}, nil
}

// getDockerSeverity returns the status of the message based on the value of the
// STREAM_TYPE byte in the header. STREAM_TYPE can be 1 for stdout and 2 for
// stderr. If it doesn't match either of these, return an empty string.
func getDockerSeverity(msg []byte) string {
	switch msg[0] {
	case 1:
		return StatusInfo
	case 2:
		return StatusError
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
// that is in front of each config.
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
