// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/server"
)

// TestInMemoryListener runs a fakeintake on an in-memory listener in a testing/synctest bubble, where network I/O
// would keep the bubble from becoming idle: payloads are ingested and queried, including by the control requests,
// over the listener, which is closed once the server stops.
func TestInMemoryListener(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		listener := newPipeListener()
		ready := make(chan bool, 1)
		fi := server.NewServer(server.WithListener(listener, "http://fakeintake.pipe"), server.WithReadyChannel(ready))
		fi.Start()
		require.True(t, <-ready, "fakeintake server did not become ready")
		assert.Equal(t, "http://fakeintake.pipe", fi.URL())

		transport := &http.Transport{DialContext: listener.dial}
		defer transport.CloseIdleConnections()
		agent := &http.Client{Transport: transport}
		resp, err := agent.Post(fi.URL()+"/api/v2/agenthealth", "application/json", bytes.NewReader([]byte(`{"issues":{}}`)))
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		client := NewClient(fi.URL(), WithHTTPTransport(transport))
		payloads, err := client.GetRawPayloads("/api/v2/agenthealth")
		require.NoError(t, err)
		require.Len(t, payloads, 1)
		assert.JSONEq(t, `{"issues":{}}`, string(payloads[0].Data))
		// the Private Action Runner control requests use the transport too
		dequeues, err := client.GetPARDequeueCount()
		require.NoError(t, err)
		assert.Zero(t, dequeues)

		require.NoError(t, fi.Stop())
		_, err = listener.dial(context.Background(), "tcp", "fakeintake.pipe:80")
		assert.ErrorIs(t, err, net.ErrClosed, "the listener must be closed once Stop returns")
	})
}

// TestInMemoryListenerURL checks the URL of a server given a listener: the given one without a trailing slash, or one
// built from the listener address
func TestInMemoryListenerURL(t *testing.T) {
	for _, tc := range []struct{ url, expected string }{
		{url: "http://fakeintake.pipe/", expected: "http://fakeintake.pipe"},
		{url: "", expected: "http://pipe"},
	} {
		synctest.Test(t, func(t *testing.T) {
			ready := make(chan bool, 1)
			fi := server.NewServer(server.WithListener(newPipeListener(), tc.url), server.WithReadyChannel(ready))
			fi.Start()
			require.True(t, <-ready, "fakeintake server did not become ready")
			assert.Equal(t, tc.expected, fi.URL())
			assert.True(t, fi.IsRunning())
			require.NoError(t, fi.Stop())
		})
	}
}

// pipeListener is an in-memory net.Listener made of net.Pipe connections, which are bubbled channels in a
// testing/synctest bubble
type pipeListener struct {
	conns     chan net.Conn
	closed    chan struct{}
	closeOnce sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn), closed: make(chan struct{})}
}

// Accept implements net.Listener
func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

// Close implements net.Listener
func (l *pipeListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

// Addr implements net.Listener
func (l *pipeListener) Addr() net.Addr {
	return pipeAddr{}
}

func (l *pipeListener) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	select {
	case <-l.closed:
		return nil, net.ErrClosed
	default:
	}
	serverConn, clientConn := net.Pipe()
	select {
	case l.conns <- serverConn:
		return clientConn, nil
	case <-l.closed:
		return nil, errors.Join(net.ErrClosed, serverConn.Close(), clientConn.Close())
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// pipeAddr is the address of a pipeListener: not a host:port, hence the explicit server URL
type pipeAddr struct{}

// Network implements net.Addr
func (pipeAddr) Network() string { return "pipe" }

// String implements net.Addr
func (pipeAddr) String() string { return "pipe" }
