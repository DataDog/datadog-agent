// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package healthplatform

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// pipeListenerAddress is the host:port the clients of a pipeListener dial: any other address is unreachable
const pipeListenerAddress = "fakeintake.pipe:80"

// pipeListener is an in-memory net.Listener for the tests running in a testing/synctest bubble. A goroutine blocked on
// network I/O isn't durably blocked, so it would keep the bubble clock from advancing, while net.Pipe connections are
// made of bubbled channels.
type pipeListener struct {
	conns     chan net.Conn
	closed    chan struct{}
	closeOnce sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{
		conns:  make(chan net.Conn),
		closed: make(chan struct{}),
	}
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

// transport returns an HTTP transport connecting to the listener. Requests to any other address fail as if it were
// unreachable, so a test still notices requests sent elsewhere.
func (l *pipeListener) transport() *http.Transport {
	return &http.Transport{DialContext: l.dial}
}

func (l *pipeListener) dial(ctx context.Context, network, address string) (net.Conn, error) {
	if address != pipeListenerAddress {
		return nil, &net.OpError{Op: "dial", Net: network, Err: fmt.Errorf("%s is unreachable: only %s is", address, pipeListenerAddress)}
	}
	serverConn, clientConn := net.Pipe()
	select {
	case l.conns <- serverConn:
		return clientConn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// pipeAddr is the address of a pipeListener
type pipeAddr struct{}

// Network implements net.Addr
func (pipeAddr) Network() string { return "pipe" }

// String implements net.Addr
func (pipeAddr) String() string { return pipeListenerAddress }
