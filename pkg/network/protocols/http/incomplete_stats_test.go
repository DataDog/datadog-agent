// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux && bpf

package http

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

func TestOrphanEntries(t *testing.T) {
	t.Run("orphan entries can be joined even after flushing", func(t *testing.T) {
		now := time.Now()
		tel := NewTelemetry("http")
		buffer := NewIncompleteBuffer(config.New(), tel)
		request := &EbpfEvent{
			Http: EbpfTx{
				Request_fragment: requestFragment([]byte("GET /foo/bar")),
				Request_started:  uint64(now.UnixNano()),
			},
		}
		request.Tuple.Sport = 60000

		buffer.Add(request)
		now = now.Add(5 * time.Second)
		complete := buffer.Flush()
		assert.Len(t, complete, 0)

		response := &EbpfEvent{
			Http: EbpfTx{
				Response_status_code: 200,
				Response_last_seen:   uint64(now.UnixNano()),
			},
		}
		response.Tuple.Sport = 60000
		buffer.Add(response)
		complete = buffer.Flush()
		require.Len(t, complete, 1)

		completeTX := complete[0]
		path, _ := completeTX.Path(make([]byte, 256))
		assert.Equal(t, "/foo/bar", string(path))
		assert.Equal(t, uint16(200), completeTX.StatusCode())
	})

	t.Run("orphan entries are not kept indefinitely", func(t *testing.T) {
		tel := NewTelemetry("http")
		// Temporary cast until we introduce a HTTP2 dedicated implementation for incompleteBuffer.
		buffer := NewIncompleteBuffer(config.New(), tel).(*incompleteBuffer)
		startTime, err := ebpf.NowNanoseconds()
		require.NoError(t, err)
		buffer.minAgeNano = (1 * time.Second).Nanoseconds()
		request := &EbpfEvent{
			Http: EbpfTx{
				Request_fragment: requestFragment([]byte("GET /foo/bar")),
				Request_started:  uint64(startTime),
			},
		}
		buffer.Add(request)
		_ = buffer.Flush()

		require.NotEmpty(t, buffer.data)
		for key := range buffer.data {
			buffer.data[key].requests[0].(*EbpfEvent).Http.Request_started = uint64(startTime - buffer.minAgeNano)
		}

		_ = buffer.Flush()
		require.Empty(t, buffer.data)
	})
}

func TestFlushMatchesOutOfOrderTransactionsByTimestamp(t *testing.T) {
	newRequest := func(path string, started time.Time) *EbpfEvent {
		e := &EbpfEvent{Http: EbpfTx{
			Request_fragment: requestFragment([]byte("GET " + path)),
			Request_started:  uint64(started.UnixNano()),
		}}
		e.Tuple.Sport = 60000
		return e
	}
	newResponse := func(status uint16, lastSeen time.Time) *EbpfEvent {
		e := &EbpfEvent{Http: EbpfTx{
			Response_status_code: status,
			Response_last_seen:   uint64(lastSeen.UnixNano()),
		}}
		e.Tuple.Sport = 60000
		return e
	}

	now := time.Now()
	at := func(seconds int) time.Time { return now.Add(time.Duration(seconds) * time.Second) }
	buffer := NewIncompleteBuffer(config.New(), NewTelemetry("http"))

	// A: request at t0/response at t1, B: t2/t3, C: t4/t5, added out-of-order.
	buffer.Add(newResponse(404, at(5))) // C's response
	buffer.Add(newRequest("/a", at(0)))
	buffer.Add(newResponse(500, at(1))) // A's response
	buffer.Add(newRequest("/c", at(4)))
	buffer.Add(newRequest("/b", at(2)))
	buffer.Add(newResponse(200, at(3))) // B's response

	flushed := buffer.Flush()
	require.Len(t, flushed, 3)

	pathA, _ := flushed[0].Path(make([]byte, 256))
	assert.Equal(t, "/a", string(pathA))
	assert.Equal(t, uint16(500), flushed[0].StatusCode())

	pathB, _ := flushed[1].Path(make([]byte, 256))
	assert.Equal(t, "/b", string(pathB))
	assert.Equal(t, uint16(200), flushed[1].StatusCode())

	pathC, _ := flushed[2].Path(make([]byte, 256))
	assert.Equal(t, "/c", string(pathC))
	assert.Equal(t, uint16(404), flushed[2].StatusCode())
}
