// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"encoding/binary"
)

// Delimiter is responsible for adding delimiters to the frames being sent.
type Delimiter interface {
	delimit(content []byte) ([]byte, error)
}

// NewDelimiter returns a delimiter.
func NewDelimiter(useProto bool) Delimiter {
	if useProto {
		return lengthPrefixDelimiter{}
	}
	return lineBreakDelimiter{}
}

// lengthPrefixDelimiter is a delimiter that prepends the length of each message as an unsigned 32-bit integer, encoded in
// binary (big-endian).
//
// For example:
// BEFORE ENCODE (300 bytes)       AFTER ENCODE (302 bytes)
// +---------------+               +--------+---------------+
// | Raw Data      |-------------->| Length | Raw Data      |
// |  (300 bytes)  |               | 0xAC02 |  (300 bytes)  |
// +---------------+               +--------+---------------+
type lengthPrefixDelimiter struct{}

func (l lengthPrefixDelimiter) delimit(content []byte) ([]byte, error) {
	buf := make([]byte, 4+len(content))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(content)))
	copy(buf[4:], content)
	return buf, nil
}

// lineBreakDelimiter is a delimiter that appends a line break after each message.
//
// For example:
// BEFORE ENCODE (300 bytes)       AFTER ENCODE (301 bytes)
// +---------------+               +---------------+------------+
// | Raw Data      |-------------->| Raw Data      | Line Break |
// |  (300 bytes)  |               |  (300 bytes)  | 0x0A       |
// +---------------+               +---------------+------------+
type lineBreakDelimiter struct{}

func (l lineBreakDelimiter) delimit(content []byte) ([]byte, error) {
	return append(content, '\n'), nil
}
