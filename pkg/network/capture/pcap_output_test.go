// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && pcap && cgo

package capture

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureInfo(capLen, origLen int) gopacket.CaptureInfo {
	return gopacket.CaptureInfo{
		Timestamp:     time.Unix(1_700_000_000, 123_000),
		CaptureLength: capLen,
		Length:        origLen,
	}
}

// TestPCAPSizeConstantsMatchWriter pins pcapFileHeaderSize and
// pcapPacketHeaderSize to what pcapgo.Writer actually emits.
//
// CaptureConfig.MaxBytes is enforced by arithmetic in drainLoop rather than by
// measuring the output stream, so these two constants are the only thing tying
// that arithmetic to reality. If the writer's output changes size and the
// constants do not, the cap silently drifts; this test makes that a failure.
func TestPCAPSizeConstantsMatchWriter(t *testing.T) {
	t.Run("file header", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, pcapgo.NewWriter(&buf).WriteFileHeader(maxSnapLen, layers.LinkTypeLinuxSLL))
		assert.Equal(t, pcapFileHeaderSize, uint64(buf.Len()),
			"pcapFileHeaderSize must equal the bytes WriteFileHeader emits")
	})

	t.Run("record header", func(t *testing.T) {
		for _, payloadLen := range []int{0, 1, 54, 66, 256, 1514} {
			var buf bytes.Buffer
			require.NoError(t, pcapgo.NewWriter(&buf).WritePacket(captureInfo(payloadLen, payloadLen), make([]byte, payloadLen)))
			assert.Equal(t, pcapPacketHeaderSize+uint64(payloadLen), uint64(buf.Len()),
				"record size for a %d-byte payload must be pcapPacketHeaderSize + payload", payloadLen)
		}
	})
}

// TestPCAPOutputReadsBack checks that a file written the way drainLoop writes
// it — headers trimmed, original length kept — is read back intact by a pcap
// reader, with the link type, timestamps and both lengths preserved.
func TestPCAPOutputReadsBack(t *testing.T) {
	var buf bytes.Buffer
	w := pcapgo.NewWriter(&buf)
	require.NoError(t, w.WriteFileHeader(maxSnapLen, layers.LinkTypeLinuxSLL))

	packets := []struct {
		data    []byte
		origLen int
	}{
		{bytes.Repeat([]byte{0xaa}, 54), 1514},
		{bytes.Repeat([]byte{0xbb}, 66), 66},
		{bytes.Repeat([]byte{0xcc}, 128), 9000},
	}
	for _, p := range packets {
		require.NoError(t, w.WritePacket(captureInfo(len(p.data), p.origLen), p.data))
	}

	r, err := pcapgo.NewReader(&buf)
	require.NoError(t, err)
	assert.Equal(t, layers.LinkTypeLinuxSLL, r.LinkType())
	assert.Equal(t, uint32(maxSnapLen), r.Snaplen())

	for i, p := range packets {
		data, ci, err := r.ReadPacketData()
		require.NoError(t, err, "packet %d", i)
		assert.Equal(t, p.data, data, "packet %d bytes", i)
		assert.Equal(t, len(p.data), ci.CaptureLength, "packet %d captured length", i)
		assert.Equal(t, p.origLen, ci.Length, "packet %d original length", i)
		assert.True(t, ci.Timestamp.Equal(time.Unix(1_700_000_000, 123_000)), "packet %d timestamp", i)
	}
	_, _, err = r.ReadPacketData()
	assert.ErrorIs(t, err, io.EOF, "no trailing bytes after the last record")
}

// TestMaxBytesAccounting exercises the cap rule from drainLoop against the real
// writer: admit a packet only while the resulting file still fits in MaxBytes,
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
		// The file header is written by Start() before any cap is consulted,
		// so a cap below it cannot be honoured — the floor on any produced file
		// is pcapFileHeaderSize. Such a cap is degenerate (real budgets are
		// megabytes); what matters is that it yields a valid empty pcap rather
		// than a truncated one.
		{"cap at exactly the file header admits nothing", pcapFileHeaderSize, 10, 0},
		{"cap one byte short of the first record admits nothing", pcapFileHeaderSize + recordSize - 1, 10, 0},
		{"cap at exactly one record admits one", pcapFileHeaderSize + recordSize, 10, 1},
		{"cap sized for three admits three", pcapFileHeaderSize + 3*recordSize, 10, 3},
		{"cap larger than the offered packets admits all", pcapFileHeaderSize + 100*recordSize, 4, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := pcapgo.NewWriter(&buf)
			require.NoError(t, w.WriteFileHeader(maxSnapLen, layers.LinkTypeLinuxSLL))
			fileBytes := pcapFileHeaderSize

			written := 0
			for range tc.offered {
				// Mirrors drainLoop: check before writing, stop rather than skip.
				if tc.maxBytes > 0 && fileBytes+recordSize > tc.maxBytes {
					break
				}
				require.NoError(t, w.WritePacket(captureInfo(payloadLen, payloadLen), make([]byte, payloadLen)))
				fileBytes += recordSize
				written++
			}

			assert.Equal(t, tc.expectedWritten, written, "packets admitted")
			assert.Equal(t, fileBytes, uint64(buf.Len()), "tracked size must match bytes actually written")
			if tc.maxBytes >= pcapFileHeaderSize {
				assert.LessOrEqual(t, uint64(buf.Len()), tc.maxBytes, "file must never exceed MaxBytes")
			}

			r, err := pcapgo.NewReader(&buf)
			require.NoError(t, err, "output must be a readable pcap")
			read := 0
			for {
				if _, _, err := r.ReadPacketData(); err != nil {
					require.ErrorIs(t, err, io.EOF, "output must not end mid-record")
					break
				}
				read++
			}
			assert.Equal(t, written, read, "every admitted packet reads back")
		})
	}
}
