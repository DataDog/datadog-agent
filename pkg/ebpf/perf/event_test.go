// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package perf

import (
	"sync/atomic"
	"testing"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
)

// TestAfterStopWaitsForHandler checks that AfterStop returns only after the
// handler has processed every record still buffered when the manager stopped,
// so that no handler call outlives the manager's Stop.
func TestAfterStopWaitsForHandler(t *testing.T) {
	const records = 5
	moduleName := names.NewModuleName("perf_test")

	for _, tc := range []struct {
		name  string
		setup func(e *EventHandler)
		send  func(e *EventHandler)
	}{
		{
			name: "perf buffer",
			setup: func(e *EventHandler) {
				e.perfChan = make(chan *perf.Record, records)
				e.readLoop = e.perfLoop
			},
			send: func(e *EventHandler) {
				e.perfChan <- &perf.Record{RawSample: []byte{1}}
			},
		},
		{
			name: "ring buffer",
			setup: func(e *EventHandler) {
				e.ringChan = make(chan *ringbuf.Record, records)
				e.readLoop = e.ringLoop
			},
			send: func(e *EventHandler) {
				e.ringChan <- &ringbuf.Record{RawSample: []byte{1}}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var handled atomic.Int32
			e := &EventHandler{
				handler: func([]byte) {
					time.Sleep(10 * time.Millisecond)
					handled.Add(1)
				},
			}
			tc.setup(e)
			require.NoError(t, e.PreStart(nil, moduleName))
			for range records {
				tc.send(e)
			}

			require.NoError(t, e.AfterStop(nil, moduleName, manager.CleanAll))
			assert.EqualValues(t, records, handled.Load())
		})
	}
}

// TestPreStartStartsOneReadLoop checks that a second PreStart, as on a repeated
// manager Start, doesn't start another read loop, which AfterStop wouldn't
// wait for: records are handled one at a time, by a single loop.
func TestPreStartStartsOneReadLoop(t *testing.T) {
	const records = 4
	moduleName := names.NewModuleName("perf_test")
	var active, maxActive, handled atomic.Int32
	e := &EventHandler{
		handler: func([]byte) {
			n := active.Add(1)
			for {
				m := maxActive.Load()
				if n <= m || maxActive.CompareAndSwap(m, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			handled.Add(1)
		},
		ringChan: make(chan *ringbuf.Record, records),
	}
	e.readLoop = e.ringLoop
	require.NoError(t, e.PreStart(nil, moduleName))
	require.NoError(t, e.PreStart(nil, moduleName))
	for range records {
		e.ringChan <- &ringbuf.Record{RawSample: []byte{1}}
	}

	require.NoError(t, e.AfterStop(nil, moduleName, manager.CleanAll))
	assert.EqualValues(t, records, handled.Load())
	assert.EqualValues(t, 1, maxActive.Load(), "records were handled concurrently by more than one read loop")
}

// TestAfterStopWithoutPreStart checks that AfterStop doesn't block when the
// read loop was never started, e.g. because the manager failed to start.
func TestAfterStopWithoutPreStart(t *testing.T) {
	e := &EventHandler{ringChan: make(chan *ringbuf.Record, 1)}
	done := make(chan error, 1)
	go func() {
		done <- e.AfterStop(nil, names.NewModuleName("perf_test"), manager.CleanAll)
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("AfterStop blocked without a started read loop")
	}
}
