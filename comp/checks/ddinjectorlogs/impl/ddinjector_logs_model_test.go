// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ddinjectorlogsimpl

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// traceLoggingMetadata builds TraceLogging event metadata for name, followed by fields.
func traceLoggingMetadata(extension []byte, name string, fields []byte) []byte {
	metadata := make([]byte, 2, 2+len(extension)+len(name)+1+len(fields))
	metadata = append(metadata, extension...)
	metadata = append(metadata, name...)
	metadata = append(metadata, 0)
	metadata = append(metadata, fields...)
	binary.LittleEndian.PutUint16(metadata, uint16(len(metadata)))
	return metadata
}

func TestTraceLoggingEventName(t *testing.T) {
	fields := []byte("Message\x00\x02ProcessId\x00\x08")
	for _, test := range []struct {
		name      string
		extension []byte
	}{
		{name: "single extension byte", extension: []byte{0}},
		{name: "chained extension bytes", extension: []byte{0x81, 0x80, 0x01}},
	} {
		t.Run(test.name, func(t *testing.T) {
			name, err := traceLoggingEventName(traceLoggingMetadata(test.extension, "CrashAttribution_Event", fields))
			require.NoError(t, err)
			assert.Equal(t, "CrashAttribution_Event", name)
		})
	}
}

func TestTraceLoggingEventNameIgnoresTrailingData(t *testing.T) {
	metadata := append(traceLoggingMetadata([]byte{0}, "Ioctl_CreateClose_Request", nil), "garbage"...)
	name, err := traceLoggingEventName(metadata)
	require.NoError(t, err)
	assert.Equal(t, "Ioctl_CreateClose_Request", name)
}

func TestTraceLoggingEventNameRejectsMalformedMetadata(t *testing.T) {
	valid := traceLoggingMetadata([]byte{0}, "CrashAttribution_Event", nil)
	oversized := append([]byte(nil), valid...)
	binary.LittleEndian.PutUint16(oversized, uint16(len(valid)+1))
	unterminatedName := append([]byte(nil), valid[:len(valid)-1]...)
	binary.LittleEndian.PutUint16(unterminatedName, uint16(len(unterminatedName)))

	for _, test := range []struct {
		name     string
		metadata []byte
	}{
		{name: "empty", metadata: nil},
		{name: "size smaller than its own field", metadata: []byte{1, 0}},
		{name: "size exceeds data", metadata: oversized},
		{name: "unterminated extension", metadata: []byte{4, 0, 0x80, 0x80}},
		{name: "unterminated name", metadata: unterminatedName},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := traceLoggingEventName(test.metadata)
			assert.Error(t, err)
		})
	}
}
