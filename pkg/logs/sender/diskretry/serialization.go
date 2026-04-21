// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diskretry

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// Binary format per file:
//
//	[4 bytes] magic number
//	[4 bytes] version (uint32 LE)
//	[4 bytes] encoding string length (uint32 LE)
//	[N bytes] encoding string
//	[4 bytes] unencoded size (uint32 LE)
//	[4 bytes] encoded payload length (uint32 LE)
//	[N bytes] encoded payload (raw bytes)
//	[4 bytes] message count (uint32 LE)
//	[1 byte]  isMRF flag (optional; 0=false, 1=true; absent in older files)

const (
	fileMagic       = uint32(0x44524554) // "DRET" (Disk RETry)
	formatVersion   = uint32(1)
	headerSize      = 4 + 4                  // magic + version
	minFileSize     = headerSize + 4 + 4 + 4 // + encoding len + unencoded size + encoded len + count
	maxPayloadSize  = 100 * 1024 * 1024      // 100 MB sanity limit
	maxMessageCount = 100_000                // sanity limit for message metadata count
)

// SerializePayload serializes a message.Payload into the binary disk retry format.
func SerializePayload(payload *message.Payload) ([]byte, error) {
	encodingBytes := []byte(payload.Encoding)
	messageCount := uint32(len(payload.MessageMetas))

	// Calculate total size
	totalSize := headerSize +
		4 + len(encodingBytes) + // encoding string length + encoding
		4 + // unencoded size
		4 + len(payload.Encoded) + // encoded payload length + payload
		4 + // message count
		1 // isMRF flag

	buf := make([]byte, totalSize)
	offset := 0

	// Magic number
	binary.LittleEndian.PutUint32(buf[offset:], fileMagic)
	offset += 4

	// Version
	binary.LittleEndian.PutUint32(buf[offset:], formatVersion)
	offset += 4

	// Encoding string
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(encodingBytes)))
	offset += 4
	copy(buf[offset:], encodingBytes)
	offset += len(encodingBytes)

	// Unencoded size
	binary.LittleEndian.PutUint32(buf[offset:], uint32(payload.UnencodedSize))
	offset += 4

	// Encoded payload
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(payload.Encoded)))
	offset += 4
	copy(buf[offset:], payload.Encoded)
	offset += len(payload.Encoded)

	// Message count
	binary.LittleEndian.PutUint32(buf[offset:], messageCount)
	offset += 4

	// isMRF flag
	if payload.IsMRF() {
		buf[offset] = 1
	}

	return buf, nil
}

// diskRetrySource is a shared LogSource used for deserialized payloads.
// It provides the minimum non-nil structure needed so that deserialized payloads
// can flow through the auditor without nil-pointer panics. The auditor skips
// registry updates for payloads with an empty Origin.Identifier.
var diskRetrySource = sources.NewLogSource("disk_retry", &config.LogsConfig{})

// DeserializePayload deserializes bytes produced by SerializePayload back into
// a message.Payload. The returned payload has minimal MessageMetadata entries
// (with empty Origin identifiers) so the auditor safely skips them.
func DeserializePayload(data []byte) (*message.Payload, error) {
	if len(data) < minFileSize {
		return nil, fmt.Errorf("file too small: %d bytes", len(data))
	}

	offset := 0

	// Magic number
	magic := binary.LittleEndian.Uint32(data[offset:])
	if magic != fileMagic {
		return nil, fmt.Errorf("invalid magic number: 0x%08X", magic)
	}
	offset += 4

	// Version
	version := binary.LittleEndian.Uint32(data[offset:])
	if version != formatVersion {
		return nil, fmt.Errorf("unsupported format version: %d", version)
	}
	offset += 4

	// Encoding string
	if offset+4 > len(data) {
		return nil, errors.New("truncated file: missing encoding length")
	}
	encodingLen := binary.LittleEndian.Uint32(data[offset:])
	offset += 4
	if offset+int(encodingLen) > len(data) {
		return nil, errors.New("truncated file: encoding string")
	}
	encoding := string(data[offset : offset+int(encodingLen)])
	offset += int(encodingLen)

	// Unencoded size
	if offset+4 > len(data) {
		return nil, errors.New("truncated file: missing unencoded size")
	}
	unencodedSize := binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	// Encoded payload
	if offset+4 > len(data) {
		return nil, errors.New("truncated file: missing encoded length")
	}
	encodedLen := binary.LittleEndian.Uint32(data[offset:])
	offset += 4
	if encodedLen > maxPayloadSize {
		return nil, fmt.Errorf("encoded payload too large: %d bytes", encodedLen)
	}
	if offset+int(encodedLen) > len(data) {
		return nil, errors.New("truncated file: encoded payload")
	}
	encoded := make([]byte, encodedLen)
	copy(encoded, data[offset:offset+int(encodedLen)])
	offset += int(encodedLen)

	// Message count
	if offset+4 > len(data) {
		return nil, errors.New("truncated file: missing message count")
	}
	messageCount := binary.LittleEndian.Uint32(data[offset:])
	if messageCount > maxMessageCount {
		return nil, fmt.Errorf("message count too large: %d (max %d)", messageCount, maxMessageCount)
	}
	offset += 4

	// isMRF flag (optional — absent in older files, defaults to false)
	isMRF := false
	if offset < len(data) {
		isMRF = data[offset] != 0
	}

	// Build minimal MessageMetadata entries so payload.Count() returns the
	// correct value and the auditor doesn't panic on nil Origin pointers.
	// The auditor skips updates when Origin.Identifier is empty.
	metas := make([]*message.MessageMetadata, messageCount)
	for i := range metas {
		metas[i] = &message.MessageMetadata{
			Origin: message.NewOrigin(diskRetrySource),
			ParsingExtra: message.ParsingExtra{
				IsMRFAllow: isMRF,
			},
		}
	}

	return &message.Payload{
		MessageMetas:  metas,
		Encoded:       encoded,
		Encoding:      encoding,
		UnencodedSize: int(unencodedSize),
	}, nil
}
