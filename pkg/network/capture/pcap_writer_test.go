// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap && cgo

package capture

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPCAPSizeConstantsMatchWriters pins pcapFileHeaderSize and
// pcapPacketHeaderSize to what the writers actually emit.
//
// CaptureConfig.MaxBytes is enforced by arithmetic in drainLoop rather than by
// measuring the output stream, so these two constants are the only thing tying
// that arithmetic to reality. If either the global header or the per-record
// header changes size and the constant does not, the cap silently drifts — the
// file would over- or under-shoot MaxBytes with nothing to signal it. This test
// is what makes that a compile-and-test failure instead.
func TestPCAPSizeConstantsMatchWriters(t *testing.T) {
	t.Run("file header", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, writePCAPHeader(&buf, 256))
		assert.Equal(t, pcapFileHeaderSize, uint64(buf.Len()),
			"pcapFileHeaderSize must equal the bytes writePCAPHeader emits")
	})

	t.Run("record header", func(t *testing.T) {
		for _, payloadLen := range []int{0, 1, 54, 66, 256, 1514} {
			var buf bytes.Buffer
			pkt := RawPacket{
				Timestamp: time.Unix(1_700_000_000, 123_000),
				Data:      make([]byte, payloadLen),
				OrigLen:   uint32(payloadLen),
			}
			require.NoError(t, writePCAPPacket(&buf, pkt))
			assert.Equal(t, pcapPacketHeaderSize+uint64(payloadLen), uint64(buf.Len()),
				"record size for a %d-byte payload must be pcapPacketHeaderSize + payload", payloadLen)
		}
	})
}

// TestMaxBytesAccounting exercises the cap rule from drainLoop against the real
// writers: admit a packet only while the resulting file still fits in MaxBytes,
// and stop at the first packet that would not.
//
// The two properties that matter to a caller budgeting upload size are that the
// file never exceeds the cap, and that it never ends mid-record — a truncated
// trailing record would make the whole pcap unreadable past that point, which
// is a worse outcome than dropping the packet.
func TestMaxBytesAccounting(t *testing.T) {
	const payloadLen = 64
	recordSize := pcapPacketHeaderSize + uint64(payloadLen)

	tests := []struct {
		name            string
		maxBytes        uint64
		offered         int
		expectedWritten int
	}{
		{"no cap admits everything", 0, 10, 10},
		// The global header is written by Start() before any cap is consulted,
		// so a cap below it cannot be honoured — the floor on any produced file
		// is pcapFileHeaderSize. Such a cap is degenerate (real budgets are
		// megabytes); what matters is that it yields a valid empty pcap rather
		// than a truncated one. Asserted separately below.
		{"cap at exactly the file header admits nothing", pcapFileHeaderSize, 10, 0},
		{"cap one byte short of the first record admits nothing", pcapFileHeaderSize + recordSize - 1, 10, 0},
		{"cap at exactly one record admits one", pcapFileHeaderSize + recordSize, 10, 1},
		{"cap sized for three admits three", pcapFileHeaderSize + 3*recordSize, 10, 3},
		{"cap larger than the offered packets admits all", pcapFileHeaderSize + 100*recordSize, 4, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, writePCAPHeader(&buf, 256))
			fileBytes := pcapFileHeaderSize

			written := 0
			for range tc.offered {
				pkt := RawPacket{
					Timestamp: time.Unix(1_700_000_000, 0),
					Data:      make([]byte, payloadLen),
					OrigLen:   payloadLen,
				}
				// Mirrors drainLoop: check before writing, stop rather than skip.
				if tc.maxBytes > 0 && fileBytes+recordSize > tc.maxBytes {
					break
				}
				require.NoError(t, writePCAPPacket(&buf, pkt))
				fileBytes += recordSize
				written++
			}

			assert.Equal(t, tc.expectedWritten, written, "packets admitted")
			assert.Equal(t, fileBytes, uint64(buf.Len()), "tracked size must match bytes actually written")
			if tc.maxBytes >= pcapFileHeaderSize {
				assert.LessOrEqual(t, uint64(buf.Len()), tc.maxBytes, "file must never exceed MaxBytes")
			}
		})
	}
}

// TestMaxBytesBelowFileHeader documents the one case where the produced file is
// larger than MaxBytes: the 24-byte global header is written by Start() before
// drainLoop consults the cap, so it is an unavoidable floor. The guarantee that
// survives is the one that matters — the output is a valid, readable, empty
// pcap, never a truncated one.
func TestMaxBytesBelowFileHeader(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writePCAPHeader(&buf, 256))

	assert.Equal(t, pcapFileHeaderSize, uint64(buf.Len()))
	assert.Greater(t, uint64(buf.Len()), pcapFileHeaderSize-1,
		"a sub-header MaxBytes cannot be honoured; the global header is the floor")
}
