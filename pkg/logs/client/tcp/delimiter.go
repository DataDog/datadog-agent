// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"bytes"
	"encoding/binary"
)

// Delimiter is responsible for adding delimiters to the frames being sent.
type Delimiter interface {
	delimit(content []byte) ([]byte, error)
}

// NewDelimiter returns a delimiter.
func NewDelimiter(useProto bool) Delimiter {
	if useProto {
		return &lengthPrefix
	}
	return &lineBreak
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
var lengthPrefix lengthPrefixDelimiter

type lengthPrefixDelimiter struct {
	Delimiter
}

func (l *lengthPrefixDelimiter) delimit(content []byte) ([]byte, error) {
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
var lineBreak lineBreakDelimiter

type lineBreakDelimiter struct {
	Delimiter
}

func (l *lineBreakDelimiter) delimit(content []byte) ([]byte, error) {
	return append(content, '\n'), nil
}
