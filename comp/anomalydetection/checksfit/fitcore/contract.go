// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

package fitcore

import (
	"errors"
	"fmt"
)

// Transport versions and shared-layout identity exchanged during setup.
// They identify transport mechanics only; application identity and message
// types come from the ProtocolDescriptor.
const (
	setupVersion     = 1
	layoutVersion    = 3
	headerSize       = 64 // immutable metadata region at the start of the mapping
	recordHeaderSize = 8  // payload length and type in front of every record
)

var (
	layoutID    = array8("MQUEUE03")
	headerMagic = array8("MCHKSHM3")
)

// array8 converts an exact eight-byte string constant to a fixed-size
// identifier, since Go does not allow direct string-to-array conversion.
func array8(s string) [8]byte {
	var out [8]byte
	copy(out[:], s)
	return out
}

// ProtocolDescriptor describes the application protocol: identity, version,
// and the record types a peer supports. The registry lives in local code, so
// incompatible registry or codec changes require a protocol-version bump.
type ProtocolDescriptor struct {
	ID           [8]byte
	Version      uint32
	MessageTypes []uint32
}

func (d ProtocolDescriptor) validate() error {
	if len(d.MessageTypes) == 0 {
		return errors.New("message type IDs must be nonzero and nonempty")
	}
	for index, id := range d.MessageTypes {
		if id == 0 {
			return errors.New("message type IDs must be nonzero and nonempty")
		}
		for _, other := range d.MessageTypes[:index] {
			if other == id {
				return errors.New("duplicate message type ID")
			}
		}
	}
	return nil
}

func (d ProtocolDescriptor) supports(kind uint32) bool {
	for _, id := range d.MessageTypes {
		if id == kind {
			return true
		}
	}
	return false
}

// contractBytes builds the 29-byte compatibility tuple:
// setup_version, protocol_version, layout_version (big-endian u32 each),
// role, protocol ID, and layout ID.
func contractBytes(role byte, protocol ProtocolDescriptor) []byte {
	bytes := make([]byte, 0, 29)
	bytes = appendUint32BE(bytes, setupVersion)
	bytes = appendUint32BE(bytes, protocol.Version)
	bytes = appendUint32BE(bytes, layoutVersion)
	bytes = append(bytes, role)
	bytes = append(bytes, protocol.ID[:]...)
	return append(bytes, layoutID[:]...)
}

func checkContract(body []byte, role byte, protocol ProtocolDescriptor) error {
	expected := contractBytes(role, protocol)
	if len(body) != len(expected) {
		return fmt.Errorf("contract length: expected %d, received %d", len(expected), len(body))
	}
	for index, want := range expected {
		if body[index] != want {
			return fmt.Errorf("contract mismatch: expected %s, received %s", hexBytes(expected), hexBytes(body))
		}
	}
	return nil
}

func appendUint32BE(dst []byte, value uint32) []byte {
	return append(dst, byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
}

func hexBytes(bytes []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(bytes)*2)
	for index, b := range bytes {
		out[index*2] = digits[b>>4]
		out[index*2+1] = digits[b&0xf]
	}
	return string(out)
}
