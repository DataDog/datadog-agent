// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testFlowKey() imdsFlowKey {
	return imdsFlowKey{
		srcIP:   [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 169, 254, 169, 254},
		dstIP:   [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 10, 0, 0, 1},
		srcPort: 80,
		dstPort: 54321,
		netns:   42,
	}
}

// buildIMDSResponse returns a complete Content-Length-framed HTTP response.
func buildIMDSResponse(body string) []byte {
	return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nServer: EC2ws\r\nContent-Length: %d\r\n\r\n%s", len(body), body))
}

func TestIMDSReassemblerSingleSegment(t *testing.T) {
	r, err := newIMDSReassembler()
	require.NoError(t, err)

	resp := buildIMDSResponse(`{"Type":"AWS-HMAC"}`)
	full, complete := r.process(testFlowKey(), 1000, resp)
	require.True(t, complete)
	assert.Equal(t, resp, full)
}

func TestIMDSReassemblerSplitInOrder(t *testing.T) {
	r, err := newIMDSReassembler()
	require.NoError(t, err)

	resp := buildIMDSResponse(`{"AccessKeyId":"ASIA","Type":"AWS-HMAC"}`)
	split := 30
	key := testFlowKey()

	full, complete := r.process(key, 1000, resp[:split])
	require.False(t, complete, "first segment should not complete the message")
	assert.Nil(t, full)

	full, complete = r.process(key, 1000+uint32(split), resp[split:])
	require.True(t, complete, "second segment should complete the message")
	assert.Equal(t, resp, full)
}

func TestIMDSReassemblerOutOfOrder(t *testing.T) {
	r, err := newIMDSReassembler()
	require.NoError(t, err)

	resp := buildIMDSResponse(`{"AccessKeyId":"ASIA","Token":"xxxxxxxxxx","Type":"AWS-HMAC"}`)
	split := 40
	key := testFlowKey()

	// deliver the tail first
	full, complete := r.process(key, 1000+uint32(split), resp[split:])
	require.False(t, complete, "tail-first segment cannot complete without the head")
	assert.Nil(t, full)

	// then the head
	full, complete = r.process(key, 1000, resp[:split])
	require.True(t, complete)
	assert.Equal(t, resp, full)
}

func TestIMDSReassemblerDuplicateSegment(t *testing.T) {
	r, err := newIMDSReassembler()
	require.NoError(t, err)

	resp := buildIMDSResponse(`{"Type":"AWS-HMAC"}`)
	split := 25
	key := testFlowKey()

	_, complete := r.process(key, 1000, resp[:split])
	require.False(t, complete)
	// duplicate retransmit of the first segment must not corrupt the buffer
	_, complete = r.process(key, 1000, resp[:split])
	require.False(t, complete)

	full, complete := r.process(key, 1000+uint32(split), resp[split:])
	require.True(t, complete)
	assert.Equal(t, resp, full)
}

func TestIMDSReassemblerKeepAlive(t *testing.T) {
	r, err := newIMDSReassembler()
	require.NoError(t, err)

	key := testFlowKey()
	resp1 := buildIMDSResponse(`{"token":"v2"}`)
	resp2 := buildIMDSResponse(`{"Type":"AWS-HMAC"}`)

	full, complete := r.process(key, 5000, resp1)
	require.True(t, complete)
	assert.Equal(t, resp1, full)

	// a subsequent message on the same flow (new base seq) reassembles independently
	full, complete = r.process(key, 9000, resp2)
	require.True(t, complete)
	assert.Equal(t, resp2, full)
}

func TestIMDSReassemblerOversized(t *testing.T) {
	r, err := newIMDSReassembler()
	require.NoError(t, err)

	key := testFlowKey()
	// header without terminator so the message never completes; grow past the byte cap
	chunk := make([]byte, 4096)
	for i := range chunk {
		chunk[i] = 'A'
	}
	var seq uint32 = 1
	dropped := false
	for i := 0; i < 100; i++ {
		_, complete := r.process(key, seq, chunk)
		require.False(t, complete)
		seq += uint32(len(chunk))
		if _, ok := r.flows.Get(key); !ok {
			dropped = true
			break
		}
	}
	// the oversized flow must have been dropped to bound memory
	assert.True(t, dropped)
}

func TestIMDSReassemblerStaleReset(t *testing.T) {
	r, err := newIMDSReassembler()
	require.NoError(t, err)

	key := testFlowKey()
	resp := buildIMDSResponse(`{"Type":"AWS-HMAC"}`)
	split := 25

	_, complete := r.process(key, 1000, resp[:split])
	require.False(t, complete)

	// age the buffer past the TTL: the next segment should start a fresh message rather than
	// being appended to the stale one
	buf, ok := r.flows.Get(key)
	require.True(t, ok)
	buf.lastUpdate = time.Now().Add(-2 * r.ttl)

	// a fresh, complete message at a different seq reassembles on its own
	full, complete := r.process(key, 7000, resp)
	require.True(t, complete)
	assert.Equal(t, resp, full)
}
