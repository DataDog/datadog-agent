// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

import (
	"bytes"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

// Length of the docker message header.
// See https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs:
// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
const messageHeaderLength = 8

// ParseMessage extracts the date and the severity from the raw docker message
// see https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs
func ParseMessage(msg []byte) (string, []byte, []byte, error) {

	// The format of the message should be :
	// [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}[]byte{OUTPUT}
	// If we don't have at the very least 8 bytes we can consider this message can't be parsed.
	if len(msg) < messageHeaderLength {
		return "", nil, nil, errors.New("Can't parse docker message: expected a 8 bytes header")
	}

	// First byte is 1 for stdout and 2 for stderr
	sev := config.SevInfo
	if msg[0] == 2 {
		sev = config.SevError
	}

	// timestamp goes from byte 8 till first space
	to := bytes.Index(msg[messageHeaderLength:], []byte{' '})
	if to == -1 {
		return "", nil, nil, errors.New("Can't parse docker message: expected a whitespace after header")
	}
	to += messageHeaderLength
	ts := string(msg[messageHeaderLength:to])
	return ts, sev, msg[to+1:], nil
}
