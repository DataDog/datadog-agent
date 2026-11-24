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
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/types"
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

func TestIncompleteBufferCapsBufferedTransactions(t *testing.T) {
	const maxEntries = 4

	newBuffer := func() *incompleteBuffer {
		libtelemetry.Clear()
		cfg := config.New()
		cfg.MaxHTTPStatsBuffered = maxEntries
		return NewIncompleteBuffer(cfg, NewTelemetry("http")).(*incompleteBuffer)
	}
	newRequest := func(port uint16, started int64) *EbpfEvent {
		e := &EbpfEvent{Http: EbpfTx{
			Request_fragment: requestFragment([]byte("GET /foo")),
			Request_started:  uint64(started),
		}}
		e.Tuple.Sport = port
		return e
	}
	newResponse := func(port uint16, lastSeen int64) *EbpfEvent {
		e := &EbpfEvent{Http: EbpfTx{
			Response_status_code: 200,
			Response_last_seen:   uint64(lastSeen),
		}}
		e.Tuple.Sport = port
		return e
	}
	buffered := func(b *incompleteBuffer) int {
		n := 0
		for _, parts := range b.data {
			n += len(parts.requests) + len(parts.responses)
		}
		return n
	}

	t.Run("transactions on a single connection are capped", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		for i := 0; i < 3*maxEntries; i++ {
			if i%2 == 0 {
				buffer.Add(newRequest(60000, now+int64(i)))
			} else {
				buffer.Add(newResponse(60000, now-int64(i)))
			}
		}

		assert.Len(t, buffer.data, 1)
		assert.Equal(t, maxEntries, buffered(buffer))
		assert.Equal(t, int64(2*maxEntries), buffer.telemetry.dropped.Get())
	})

	t.Run("cap counts transactions across connections", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		buffer.Add(newRequest(60000, now))
		buffer.Add(newRequest(60000, now+1))
		buffer.Add(newRequest(60001, now))
		buffer.Add(newRequest(60002, now))
		buffer.Add(newRequest(60003, now))

		assert.Equal(t, maxEntries, buffered(buffer))
		assert.Len(t, buffer.data, maxEntries-1)
		assert.Equal(t, int64(1), buffer.telemetry.dropped.Get())
	})

	t.Run("requests kept across a flush count toward the cap", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		for i := 0; i < maxEntries; i++ {
			buffer.Add(newRequest(60000, now+int64(i)))
		}
		require.Empty(t, buffer.Flush())
		require.Equal(t, maxEntries, buffered(buffer))

		buffer.Add(newRequest(60000, now+maxEntries))
		buffer.Add(newRequest(60001, now))
		assert.Equal(t, maxEntries, buffered(buffer))
		assert.Equal(t, int64(2), buffer.telemetry.dropped.Get())

		// Age out all but the newest kept request so the next flush keeps only a suffix.
		newest := uint64(now + maxEntries - 1)
		for _, parts := range buffer.data {
			for _, req := range parts.requests {
				if req.RequestStarted() < newest {
					req.(*EbpfEvent).Http.Request_started = uint64(now - buffer.minAgeNano)
				}
			}
		}
		require.Empty(t, buffer.Flush())
		require.Equal(t, 1, buffered(buffer))

		buffer.Add(newRequest(60001, now))
		buffer.Add(newRequest(60002, now))
		buffer.Add(newRequest(60003, now))
		buffer.Add(newRequest(60004, now))
		assert.Equal(t, maxEntries, buffered(buffer))
		assert.Equal(t, int64(3), buffer.telemetry.dropped.Get())
	})

	t.Run("paired transactions are joined when the buffer is full", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		assert.Empty(t, buffer.Add(newRequest(60000, now)))
		assert.Empty(t, buffer.Add(newResponse(60000, now+1)))
		assert.Empty(t, buffer.Add(newRequest(60000, now+2)))
		assert.Empty(t, buffer.Add(newResponse(60000, now+3)))

		// Only the first pair is followed by a later request; the second waits for Flush.
		joined := buffer.Add(newRequest(60001, now))
		require.Len(t, joined, 1)
		assert.Equal(t, uint64(now+1), joined[0].ResponseLastSeen())
		assert.Equal(t, int64(0), buffer.telemetry.dropped.Get())
		assert.Equal(t, 3, buffered(buffer))

		joined = buffer.Flush()
		require.Len(t, joined, 1)
		assert.Equal(t, uint64(now+3), joined[0].ResponseLastSeen())
		assert.Equal(t, 1, buffered(buffer))
	})

	t.Run("a response is not joined early without a later request", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		// The response to the first request has not been read yet; the buffered one belongs to a later request.
		buffer.Add(newRequest(60000, now))
		buffer.Add(newResponse(60000, now+30))
		buffer.Add(newRequest(60001, now))
		buffer.Add(newRequest(60002, now))

		assert.Empty(t, buffer.Add(newRequest(60003, now)))
		assert.Equal(t, int64(1), buffer.telemetry.dropped.Get())
		require.Len(t, buffer.data[types.ConnectionKey{SrcPort: 60000}].responses, 1)
	})

	t.Run("unpaired halves stay buffered when the buffer is full", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		// The early response matches no buffered request but is kept for a request read later.
		buffer.Add(newResponse(60000, now))
		buffer.Add(newRequest(60000, now+10))
		buffer.Add(newResponse(60000, now+11))
		buffer.Add(newRequest(60000, now+20))

		joined := buffer.Add(newRequest(60000, now-1))
		require.Len(t, joined, 1)
		assert.Equal(t, uint64(now+11), joined[0].ResponseLastSeen())
		assert.Equal(t, 3, buffered(buffer))

		joined = buffer.Flush()
		require.Len(t, joined, 1)
		assert.Equal(t, uint64(now), joined[0].ResponseLastSeen())
		assert.Equal(t, 1, buffered(buffer))
	})

	t.Run("joins resume after maxEntries/16 more adds", func(t *testing.T) {
		const maxEntries = 64
		libtelemetry.Clear()
		cfg := config.New()
		cfg.MaxHTTPStatsBuffered = maxEntries
		buffer := NewIncompleteBuffer(cfg, NewTelemetry("http")).(*incompleteBuffer)
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		for i := 0; i < maxEntries-3; i++ {
			buffer.Add(newRequest(uint16(60001+i), now))
		}
		buffer.Add(newRequest(60000, now))
		buffer.Add(newResponse(60000, now+5))
		buffer.Add(newRequest(60000, now+10))

		// The join frees one pair, fewer than maxEntries/16 halves.
		require.Len(t, buffer.Add(newRequest(60000, now+20)), 1)
		buffer.Add(newResponse(60000, now+15))
		require.Equal(t, maxEntries, buffered(buffer))

		assert.Empty(t, buffer.Add(newRequest(60100, now)))
		assert.Empty(t, buffer.Add(newRequest(60101, now)))
		assert.Equal(t, int64(2), buffer.telemetry.dropped.Get())

		joined := buffer.Add(newRequest(60102, now))
		require.Len(t, joined, 1)
		assert.Equal(t, uint64(now+15), joined[0].ResponseLastSeen())
		assert.Equal(t, int64(2), buffer.telemetry.dropped.Get())
	})
}

func TestStatKeeperCountsTransactionsJoinedWhenBufferIsFull(t *testing.T) {
	libtelemetry.Clear()
	cfg := config.New()
	cfg.MaxHTTPStatsBuffered = 4
	tel := NewTelemetry("http")
	sk := NewStatkeeper(cfg, tel, NewIncompleteBuffer(cfg, tel))

	now, err := ebpf.NowNanoseconds()
	require.NoError(t, err)
	const pairs = 10
	for i := 0; i < pairs; i++ {
		request := &EbpfEvent{Http: EbpfTx{
			Request_fragment: requestFragment([]byte("GET /foo")),
			Request_method:   uint8(MethodGet),
			Request_started:  uint64(now + int64(i)*int64(time.Millisecond)),
		}}
		request.Tuple.Sport = 60000
		response := &EbpfEvent{Http: EbpfTx{
			Response_status_code: 200,
			Response_last_seen:   uint64(now + int64(i)*int64(time.Millisecond) + 5000),
		}}
		response.Tuple.Sport = 60000
		sk.Process(request)
		sk.Process(response)
	}

	count := 0
	for _, stats := range sk.GetAndResetAllStats() {
		if s := stats.Data[200]; s != nil {
			count += s.Count
		}
	}
	assert.Equal(t, pairs, count)
	assert.Equal(t, int64(0), tel.dropped.Get())
}
