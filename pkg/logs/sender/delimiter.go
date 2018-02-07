// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"bytes"
	"encoding/binary"
)

// Delimiter is responsible for adding delimiters to the frames being sent.
type Delimiter interface {
	delimit(content []byte) ([]byte, error)
}

// LengthPrefix is a delimiter that prepends the length of each message as an unsigned 32-bit integer, encoded in
// binary (big-endian).
//
// For example:
// BEFORE ENCODE (300 bytes)       AFTER ENCODE (302 bytes)
// +---------------+               +--------+---------------+
// | Raw Data      |-------------->| Length | Raw Data      |
// |  (300 bytes)  |               | 0xAC02 |  (300 bytes)  |
// +---------------+               +--------+---------------+
var LengthPrefix lengthPrefix

type lengthPrefix struct{}

func (l *lengthPrefix) delimit(content []byte) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 32))
	length := uint32(len(content))
	// Use big-endian to respect network byte order
	err := binary.Write(buf, binary.BigEndian, length)
	if err != nil {
		return nil, err
	}
	return append(buf.Bytes(), content...), nil

}

// LineBreak is a delimiter that appends a line break after each message.
//
// For example:
// BEFORE ENCODE (300 bytes)       AFTER ENCODE (301 bytes)
// +---------------+               +---------------+------------+
// | Raw Data      |-------------->| Raw Data      | Line Break |
// |  (300 bytes)  |               |  (300 bytes)  | 0x0A       |
// +---------------+               +---------------+------------+
var LineBreak lineBreak

type lineBreak struct{}

func (l *lineBreak) delimit(content []byte) ([]byte, error) {
	return append(content, '\n'), nil
}
