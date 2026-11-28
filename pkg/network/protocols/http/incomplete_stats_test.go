// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux && bpf

package http

import (
	"cmp"
	"slices"
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

	t.Run("one connection takes at most three quarters of the room", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		for i := 0; i < 3*maxEntries; i++ {
			buffer.Add(newRequest(60000, now+int64(i)))
		}
		assert.Equal(t, 6, buffered(buffer))

		// Other connections still fit.
		buffer.Add(newRequest(60001, now))
		buffer.Add(newRequest(60002, now))
		assert.Equal(t, 2*maxEntries, buffered(buffer))
		assert.Equal(t, int64(3*maxEntries-6), buffer.telemetry.dropped.Get())
	})

	t.Run("cap counts requests and responses across connections", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		for port := uint16(60000); port < 60000+maxEntries; port++ {
			buffer.Add(newRequest(port, now))
			buffer.Add(newResponse(port, now+1))
		}
		buffer.Add(newRequest(60010, now))

		assert.Equal(t, 2*maxEntries, buffered(buffer))
		assert.Len(t, buffer.data, maxEntries)
		assert.Equal(t, int64(1), buffer.telemetry.dropped.Get())
	})

	t.Run("a connection that loses a half is not paired at the next flush", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		// The first request's response does not fit; the next response must not be joined with it.
		buffer.Add(newRequest(60000, now))
		for i := 0; i < 5; i++ {
			buffer.Add(newRequest(60001, now+int64(i)))
		}
		buffer.Add(newRequest(60002, now))
		buffer.Add(newResponse(60002, now+1))
		buffer.Add(newResponse(60000, now+1))
		buffer.Add(newResponse(60000, now+11))
		assert.Equal(t, int64(2), buffer.telemetry.dropped.Get())

		joined := buffer.Flush()
		require.Len(t, joined, 1)
		assert.Equal(t, uint16(60002), joined[0].ConnTuple().SrcPort)
		assert.NotContains(t, buffer.data, types.ConnectionKey{SrcPort: 60000})
		assert.Contains(t, buffer.data, types.ConnectionKey{SrcPort: 60001})
	})

	t.Run("pairs read before a dropped half are still joined", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		buffer.Add(newRequest(60000, now))
		buffer.Add(newResponse(60000, now+1))
		buffer.Add(newRequest(60000, now+10))
		for i := 0; i < 5; i++ {
			buffer.Add(newRequest(60001, now+int64(i)))
		}
		// Dropped: the response to the second request, then a later request on the same connection.
		buffer.Add(newResponse(60000, now+11))
		buffer.Add(newRequest(60000, now+20))

		joined := buffer.Flush()
		require.Len(t, joined, 1)
		assert.Equal(t, uint64(now), joined[0].RequestStarted())
		assert.Equal(t, uint64(now+1), joined[0].ResponseLastSeen())
		assert.NotContains(t, buffer.data, types.ConnectionKey{SrcPort: 60000})
	})

	t.Run("a request read late does not let later halves pair", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		// Pipelined: the second request and the first response are read before the first request, which does not fit.
		buffer.Add(newRequest(60000, now+2))
		buffer.Add(newResponse(60000, now+5))
		for i := 0; i < 6; i++ {
			buffer.Add(newRequest(uint16(60001+i), now))
		}
		buffer.Add(newRequest(60000, now+1))
		assert.Equal(t, int64(1), buffer.telemetry.dropped.Get())

		for _, tx := range buffer.Flush() {
			assert.NotEqual(t, uint16(60000), tx.ConnTuple().SrcPort, "the first response joined the second request")
		}
		assert.NotContains(t, buffer.data, types.ConnectionKey{SrcPort: 60000})
	})

	t.Run("a flush keeps the newest requests up to the setting", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		buffer.Add(newRequest(60000, now))
		buffer.Add(newRequest(60001, now+1))
		buffer.Add(newRequest(60000, now+2))
		buffer.Add(newRequest(60001, now+3))
		buffer.Add(newRequest(60002, now+4))
		buffer.Add(newRequest(60003, now+5))
		buffer.Add(newRequest(60004, now+6))
		buffer.Add(newRequest(60005, now+7))
		require.Empty(t, buffer.Flush())
		// The connections holding the oldest requests are dropped whole.
		assert.Equal(t, maxEntries, buffered(buffer))
		assert.Equal(t, int64(4), buffer.telemetry.dropped.Get())

		for port := uint16(60000); port < 60006; port++ {
			buffer.Add(newResponse(port, now+10+int64(port-60000)))
		}
		assert.Equal(t, int64(6), buffer.telemetry.dropped.Get())
		joined := buffer.Flush()
		require.Len(t, joined, maxEntries)
		slices.SortFunc(joined, func(a, b Transaction) int { return cmp.Compare(a.RequestStarted(), b.RequestStarted()) })
		for i, tx := range joined {
			assert.Equal(t, uint64(now+4+int64(i)), tx.RequestStarted())
			assert.Equal(t, uint64(now+12+int64(i)), tx.ResponseLastSeen())
		}
		assert.Equal(t, 0, buffered(buffer))
	})

	t.Run("responses to kept requests are buffered when the buffer is full", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		for port := uint16(60000); port < 60000+maxEntries; port++ {
			buffer.Add(newRequest(port, now))
		}
		require.Empty(t, buffer.Flush())
		for port := uint16(60010); port < 60010+maxEntries+1; port++ {
			buffer.Add(newRequest(port, now+1))
		}
		require.Equal(t, 2*maxEntries, buffered(buffer))
		require.Equal(t, int64(1), buffer.telemetry.dropped.Get())

		for port := uint16(60000); port < 60000+maxEntries; port++ {
			buffer.Add(newResponse(port, now+2))
		}
		assert.Equal(t, int64(1), buffer.telemetry.dropped.Get())
		joined := buffer.Flush()
		assert.Len(t, joined, maxEntries)
	})

	t.Run("requests started at the same time are kept up to the setting", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		for port := uint16(60000); port < 60000+2*maxEntries; port++ {
			buffer.Add(newRequest(port, now))
		}
		require.Empty(t, buffer.Flush())
		assert.Equal(t, maxEntries, buffered(buffer))
	})

	t.Run("a flush keeps every young request up to the setting", func(t *testing.T) {
		buffer := newBuffer()
		now, err := ebpf.NowNanoseconds()
		require.NoError(t, err)

		for i := 0; i < maxEntries; i++ {
			buffer.Add(newRequest(uint16(60000+i%2), now+int64(i)))
		}
		require.Empty(t, buffer.Flush())
		assert.Equal(t, maxEntries, buffered(buffer))
		assert.Equal(t, int64(0), buffer.telemetry.dropped.Get())
	})

	t.Run("a cap of zero or less buffers nothing", func(t *testing.T) {
		for _, max := range []int{0, -1} {
			libtelemetry.Clear()
			cfg := config.New()
			cfg.MaxHTTPStatsBuffered = max
			buffer := NewIncompleteBuffer(cfg, NewTelemetry("http")).(*incompleteBuffer)
			now, err := ebpf.NowNanoseconds()
			require.NoError(t, err)

			buffer.Add(newRequest(60000, now))
			buffer.Add(newResponse(60000, now+1))
			assert.Empty(t, buffer.Flush())
			assert.Empty(t, buffer.data)
			assert.Equal(t, int64(2), buffer.telemetry.dropped.Get())
		}
	})
}

// Halves held in eBPF batches arrive after later halves of the same connection; while the buffer is full they
// must not be paired with the wrong request.
func TestStatKeeperDoesNotMisattributeResponsesWhenBufferIsFull(t *testing.T) {
	libtelemetry.Clear()
	cfg := config.New()
	cfg.MaxHTTPStatsBuffered = 4
	tel := NewTelemetry("http")
	sk := NewStatkeeper(cfg, tel, NewIncompleteBuffer(cfg, tel))

	now, err := ebpf.NowNanoseconds()
	require.NoError(t, err)
	request := func(port uint16, path string, started int64) *EbpfEvent {
		e := &EbpfEvent{Http: EbpfTx{
			Request_fragment: requestFragment([]byte("GET " + path)),
			Request_method:   uint8(MethodGet),
			Request_started:  uint64(started),
		}}
		e.Tuple.Sport = port
		return e
	}
	response := func(port uint16, status uint16, lastSeen int64) *EbpfEvent {
		e := &EbpfEvent{Http: EbpfTx{Response_status_code: status, Response_last_seen: uint64(lastSeen)}}
		e.Tuple.Sport = port
		return e
	}
	ms := int64(time.Millisecond)

	// A complete pair on another connection is still recorded.
	sk.Process(request(60010, "/d", now))
	sk.Process(response(60010, 200, now+ms))

	// Sequential requests /a, /b and /c on one connection; /a's response and /b's request are read last.
	sk.Process(request(60000, "/a", now))
	sk.Process(response(60000, 404, now+3*ms))
	sk.Process(request(60000, "/c", now+4*ms))
	for port := uint16(60001); port < 60007; port++ {
		sk.Process(request(port, "/other", now))
	}
	sk.Process(response(60000, 200, now+ms))
	sk.Process(request(60000, "/b", now+2*ms))

	recorded := false
	for key, stats := range sk.GetAndResetAllStats() {
		path := key.Path.Content.Get()
		if path == "/d" && stats.Data[200] != nil {
			recorded = true
		}
		if path == "/a" {
			assert.Nil(t, stats.Data[404], "/a joined with /b's response")
		}
		if path == "/b" {
			assert.Nil(t, stats.Data[200], "/b joined with /a's response")
		}
	}
	assert.True(t, recorded, "/d was not recorded")
}
