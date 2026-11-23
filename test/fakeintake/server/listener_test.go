// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package server

import (
	"net"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerLifecycle(t *testing.T) {
	t.Run("WithListener is ignored on a running server", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			listener := newMemListener()
			ready := make(chan bool, 1)
			fi := NewServer(WithListener(listener, "http://fakeintake.mem"), WithReadyChannel(ready))
			fi.Start()
			require.True(t, <-ready)
			defer fi.Stop() //nolint:errcheck

			WithListener(newMemListener(), "http://other.mem")(fi)
			assert.Same(t, listener, fi.listener)
			assert.Equal(t, "http://fakeintake.mem", fi.URL())
		})
	})

	t.Run("Start on a running server notifies its readiness again", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ready := make(chan bool, 1)
			fi := NewServer(WithListener(newMemListener(), "http://fakeintake.mem"), WithReadyChannel(ready))
			fi.Start()
			require.True(t, <-ready)
			defer fi.Stop() //nolint:errcheck

			fi.Start()
			assert.True(t, <-ready)
		})
	})

	t.Run("a Start after a failed listen tries again", func(t *testing.T) {
		ready := make(chan bool, 1)
		fi := NewServer(WithAddress("127.0.0.1:-1"), WithReadyChannel(ready))
		fi.Start()
		require.False(t, <-ready, "listening on an invalid port must fail")
		assert.False(t, fi.IsRunning())

		WithAddress("127.0.0.1:0")(fi)
		fi.Start()
		require.True(t, <-ready, "the second Start must listen")
		assert.True(t, fi.IsRunning())
		assert.NoError(t, fi.Stop())
	})

	t.Run("Stop before the readiness is read closes the listener", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			listener := newMemListener()
			fi := NewServer(WithListener(listener, "http://fakeintake.mem"), WithReadyChannel(make(chan bool)))
			fi.Start()
			// the listen routine published the URL and is blocked notifying a readiness nobody reads
			synctest.Wait()
			require.True(t, fi.IsRunning())

			require.NoError(t, fi.Stop())
			select {
			case <-listener.closed:
			default:
				assert.Fail(t, "the listener must be closed once Stop returns")
			}
		})
	})
}

// memListener is an in-memory net.Listener, whose Accept blocks on bubbled channels in a testing/synctest bubble
type memListener struct {
	conns     chan net.Conn
	closed    chan struct{}
	closeOnce sync.Once
}

func newMemListener() *memListener {
	return &memListener{conns: make(chan net.Conn), closed: make(chan struct{})}
}

// Accept implements net.Listener
func (l *memListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

// Close implements net.Listener
func (l *memListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

// Addr implements net.Listener
func (l *memListener) Addr() net.Addr {
	return memAddr{}
}

// memAddr is the address of a memListener
type memAddr struct{}

// Network implements net.Addr
func (memAddr) Network() string { return "mem" }

// String implements net.Addr
func (memAddr) String() string { return "mem" }
