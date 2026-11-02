// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ddinjectorlogsimpl forwards selected DDInjector ETW events as Agent Telemetry logs.
package ddinjectorlogsimpl

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// traceLoggingEventName extracts the event name from TraceLogging event metadata, laid out as:
//
//	UINT16 Size;        // size of the whole metadata blob, including this field
//	UINT8  Extension[]; // one or more bytes, the last one has its high bit unset
//	char   Name[];      // nul-terminated UTF-8 event name
//	...                 // field metadata
func traceLoggingEventName(metadata []byte) (string, error) {
	if len(metadata) < 2 {
		return "", errors.New("TraceLogging metadata is missing its size")
	}
	size := int(binary.LittleEndian.Uint16(metadata))
	if size < 2 || size > len(metadata) {
		return "", errors.New("TraceLogging metadata size does not match the extended data")
	}
	metadata = metadata[2:size]

	extensionEnd := 0
	for ; extensionEnd < len(metadata); extensionEnd++ {
		if metadata[extensionEnd]&0x80 == 0 {
			break
		}
	}
	if extensionEnd >= len(metadata) {
		return "", errors.New("TraceLogging metadata extension is not terminated")
	}
	metadata = metadata[extensionEnd+1:]

	nameEnd := bytes.IndexByte(metadata, 0)
	if nameEnd < 0 {
		return "", errors.New("TraceLogging event name is not null terminated")
	}
	return string(metadata[:nameEnd]), nil
}
