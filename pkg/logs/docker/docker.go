// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"bytes"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Length of the docker message header.
// See https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs:
// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
const messageHeaderLength = 8
const maxDockerBuffer = 16 * 1024

// ParseMessage extracts the date and the status from the raw docker message
// see https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs
// func ParseMessage(msg []byte) (string, string, []byte, error) {

// 	// The format of the message should be :
// 	// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
// 	// If we don't have at the very least 8 bytes we can consider this message can't be parsed.
// 	if len(msg) < messageHeaderLength {
// 		return "", "", nil, errors.New("Can't parse docker message: expected a 8 bytes header")
// 	}

// 	// First byte is 1 for stdout and 2 for stderr
// 	status := message.StatusInfo
// 	log.Errorf("msg[0] is %s or %c or %x or %U or %d or %v", msg[0], msg[0], msg[0], msg[0])
// 	log.Errorf("msg[1] is %s or %c or %x or %U or %d or %v", msg[1], msg[1], msg[1], msg[1])
// 	log.Errorf("msg[2] is %s or %c or %x or %U or %d or %v", msg[2], msg[2], msg[2], msg[2])
// 	log.Errorf("msg[3] is %s or %c or %x or %U or %d or %v", msg[3], msg[3], msg[3], msg[3])
// 	log.Errorf("msg[4] is %s or %c or %x or %U or %d or %v", msg[4], msg[4], msg[4], msg[4])
// 	log.Errorf("msg[5] is %s or %c or %x or %U or %d or %v", msg[5], msg[5], msg[5], msg[5])
// 	log.Errorf("msg[6] is %s or %c or %x or %U or %d or %v", msg[6], msg[6], msg[6], msg[6])
// 	log.Errorf("msg[7] is %s or %c or %x or %U or %d or %v", msg[7], msg[7], msg[7], msg[7])
// 	log.Errorf("msg[8] is %s or %c or %x or %U or %d or %v", msg[8], msg[8], msg[8], msg[8])

// 	if msg[0] == 2 {
// 		status = message.StatusError
// 	}

// 	// timestamp goes from byte 8 till first space
// 	to := bytes.Index(msg[messageHeaderLength:], []byte{' '})
// 	if to == -1 {
// 		return "", "", nil, errors.New("Can't parse docker message: expected a whitespace after header")
// 	}
// 	to += messageHeaderLength
// 	ts := string(msg[messageHeaderLength:to])

// 	log.Errorf("ts is %s", ts)
// 	log.Errorf("status is %s", status)
// 	//log.Errorf("msg[to+1:] is %s", msg[to+1:])
// 	log.Errorf("initial length of message is %d", len(msg))
// 	log.Errorf("new length of message is %d", len(msg[to+1:]))

// 	//if len(msg[to+1:]) < maxDockerBuffer {
// 	return ts, status, msg[to+1:], nil
// 	//}

// }

// ParseMessage extracts the date and the status from the raw docker message
//
// see https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs
func ParseMessage(msg []byte) (string, string, []byte, error) {

	// The format of the message should be :
	// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
	// If we don't have at the very least 8 bytes we can consider this message can't be parsed.
	if len(msg) < messageHeaderLength {
		return "", "", nil, errors.New("Can't parse docker message: expected a 8 bytes header")
	}

	preMessageLength := getPreMessageLength(msg)
	if len(msg[preMessageLength:]) > maxDockerBuffer {
		msg = removePartialHeaders(msg, preMessageLength)
	}

	// First byte is 1 for stdout and 2 for stderr
	status := message.StatusInfo
	if msg[0] == 2 {
		status = message.StatusError
	}

	log.Errorf("msg[0] is %s or %c or %x or %U", msg[0], msg[0], msg[0], msg[0])
	log.Errorf("msg[1] is %s or %c or %x or %U", msg[1], msg[1], msg[1], msg[1])
	log.Errorf("msg[2] is %s or %c or %x or %U", msg[2], msg[2], msg[2], msg[2])
	log.Errorf("msg[3] is %s or %c or %x or %U", msg[3], msg[3], msg[3], msg[3])
	log.Errorf("msg[4] is %s or %c or %x or %U", msg[4], msg[4], msg[4], msg[4])
	log.Errorf("msg[5] is %s or %c or %x or %U", msg[5], msg[5], msg[5], msg[5])
	log.Errorf("msg[6] is %s or %c or %x or %U", msg[6], msg[6], msg[6], msg[6])
	log.Errorf("msg[7] is %s or %c or %x or %U", msg[7], msg[7], msg[7], msg[7])
	log.Errorf("msg[8] is %s or %c or %x or %U", msg[8], msg[8], msg[8], msg[8])

	// timestamp goes from byte 8 till first space
	to := bytes.Index(msg[messageHeaderLength:], []byte{' '})
	if to == -1 {
		return "", "", nil, errors.New("Can't parse docker message: expected a whitespace after header")
	}
	to += messageHeaderLength
	ts := string(msg[messageHeaderLength:to])

	log.Errorf("msg[to+1:] is %s", msg[to+1:])
	log.Errorf("new length of message is %d", len(msg[to+1:]))

	return ts, status, msg[to+1:], nil

}

// get length of the 8 byte header, timestamp, and space
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

// function only used if message is longer than 16kb
func removePartialHeaders(msg []byte, size int) []byte {

	removed := msg[:size+maxDockerBuffer]
	msg = msg[size+maxDockerBuffer:]

	preMessageLength := getPreMessageLength(msg)

	M := min(len(msg), maxDockerBuffer+preMessageLength)

	for M > 0 {
		removed = append(removed, msg[preMessageLength:M]...)
		msg = msg[M:]
		preMessageLength = getPreMessageLength(msg)
		M = min(len(msg), maxDockerBuffer+preMessageLength)

	}
	log.Errorf("removed is %s", removed)
	return removed

}

// func RemoveAdditionalHeadersFromLongLogs(msg []byte) ([]byte, error) {
// 	if len(msg) > maxDockerBuffer {
// 		nextStartingPoint := bytes.Index(msg[maxDockerBuffer:], []byte(' '))
// 		if nextStartingPoint == -1 {
// 			return nil, errors.New("Can't parse docker message: expected a whitespace after header")
// 		}
// 		nextStartingPoint += 1
// 		if len(msg[maxDockerBuffer+nextStartingPoint:]) < maxDockerBuffer {
// 			return append(msg[:max], msg[nextStartingPoint:]...), nil
// 		}

// 		msg = []byte(string() + string())
// 		RemoveAdditionalHeadersFromLongLogs

// 	}
// }
