// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ringbuffer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
)

func TestDispatcherQueueSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      config.Config
		numCPU   int
		expected int
	}{
		{
			name:     "zero size falls back to default",
			cfg:      config.Config{EventStreamDispatcherQueueSize: 0},
			numCPU:   8,
			expected: defaultDispatcherQueueSize,
		},
		{
			name: "per-core multiplies by cpu count",
			cfg: config.Config{
				EventStreamDispatcherQueueSize:        100,
				EventStreamDispatcherQueueSizePerCore: true,
			},
			numCPU:   4,
			expected: 400,
		},
		{
			name: "min floor applied after per-core",
			cfg: config.Config{
				EventStreamDispatcherQueueSize:        100,
				EventStreamDispatcherQueueSizePerCore: true,
				EventStreamDispatcherQueueSizeMin:     1000,
			},
			numCPU:   2,
			expected: 1000,
		},
		{
			name: "min ignored when already larger",
			cfg: config.Config{
				EventStreamDispatcherQueueSize:        100,
				EventStreamDispatcherQueueSizePerCore: true,
				EventStreamDispatcherQueueSizeMin:     50,
			},
			numCPU:   4,
			expected: 400,
		},
		{
			name: "invalid cpu count treated as 1",
			cfg: config.Config{
				EventStreamDispatcherQueueSize:        100,
				EventStreamDispatcherQueueSizePerCore: true,
			},
			numCPU:   0,
			expected: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, dispatcherQueueSizeWithCPU(&tt.cfg, tt.numCPU))
		})
	}
}

func TestHandleEventInlineWhenQueueDisabled(t *testing.T) {
	var got []byte
	rb := New(context.Background(), func(_ int, data []byte) {
		got = append([]byte(nil), data...)
	}, nil)

	rec := &ringbuf.Record{RawSample: []byte{1, 2, 3}}
	rb.handleEvent(rec, nil, nil)

	require.Equal(t, []byte{1, 2, 3}, got)
	require.Zero(t, rb.enqueued.Load())
}

func TestHandleEventQueuesWhenEnabled(t *testing.T) {
	done := make(chan []byte, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	rb := New(ctx, func(_ int, data []byte) {
		done <- append([]byte(nil), data...)
	}, nil)
	rb.queue = make(chan *ringbuf.Record, 4)

	var wg sync.WaitGroup
	wg.Add(1)
	go rb.dispatch(&wg)

	rb.handleEvent(&ringbuf.Record{RawSample: []byte{9, 8, 7}}, nil, nil)

	select {
	case got := <-done:
		require.Equal(t, []byte{9, 8, 7}, got)
	case <-time.After(2 * time.Second):
		t.Fatal("dispatcher did not process the queued event")
	}

	require.Equal(t, uint64(1), rb.enqueued.Load())
	require.Equal(t, uint64(1), rb.processed.Load())
	require.Zero(t, rb.queueBytes.Load())

	cancel()
	wg.Wait()
}

func TestQueueTracksBytesAndPeak(t *testing.T) {
	rb := New(context.Background(), func(int, []byte) {}, nil)
	rb.queue = make(chan *ringbuf.Record, 8)

	rb.handleEvent(&ringbuf.Record{RawSample: make([]byte, 1024)}, nil, nil)
	rb.handleEvent(&ringbuf.Record{RawSample: make([]byte, 2048)}, nil, nil)

	require.Equal(t, int64(3072), rb.queueBytes.Load())
	require.Equal(t, int64(2), rb.occupancy.Load())
	require.Equal(t, int64(2), rb.peak.Load())
	require.Equal(t, uint64(2), rb.enqueued.Load())
}

func TestDispatcherRecoversFromHandlerPanic(t *testing.T) {
	done := make(chan []byte, 1)
	var n int
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	rb := New(ctx, func(_ int, data []byte) {
		n++
		if n == 1 {
			panic("boom")
		}
		done <- append([]byte(nil), data...)
	}, nil)
	rb.queue = make(chan *ringbuf.Record, 4)

	var wg sync.WaitGroup
	wg.Add(1)
	go rb.dispatch(&wg)

	rb.handleEvent(&ringbuf.Record{RawSample: []byte{1}}, nil, nil)
	rb.handleEvent(&ringbuf.Record{RawSample: []byte{2}}, nil, nil)

	select {
	case got := <-done:
		require.Equal(t, []byte{2}, got)
	case <-time.After(2 * time.Second):
		t.Fatal("dispatcher did not recover from handler panic")
	}

	require.Equal(t, uint64(2), rb.processed.Load())
	require.Zero(t, rb.occupancy.Load())

	cancel()
	wg.Wait()
}
