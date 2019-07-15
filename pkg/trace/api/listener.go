// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package api

import (
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// rateLimitedListener wraps a regular TCPListener with rate limiting.
type rateLimitedListener struct {
	connLease int32 // How many connections are available for this listener before rate-limiting kicks in
	*net.TCPListener

	exit   chan struct{}
	closed uint32
}

// newRateLimitedListener returns a new wrapped listener, which is non-initialized
func newRateLimitedListener(l net.Listener, conns int) (*rateLimitedListener, error) {
	tcpL, ok := l.(*net.TCPListener)

	if !ok {
		return nil, errors.New("cannot wrap listener")
	}

	return &rateLimitedListener{
		connLease:   int32(conns),
		TCPListener: tcpL,
		exit:        make(chan struct{}),
	}, nil
}

// Refresh periodically refreshes the connection lease, and thus cancels any rate limits in place
func (sl *rateLimitedListener) Refresh(conns int) {
	defer close(sl.exit)

	t := time.NewTicker(30 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-sl.exit:
			return
		case <-t.C:
			atomic.StoreInt32(&sl.connLease, int32(conns))
			log.Debugf("Refreshed the connection lease: %d conns available", conns)
		}
	}
}

// rateLimitedError  indicates a user request being blocked by our rate limit
// It satisfies the net.Error interface
type rateLimitedError struct{}

// Error returns an error string
func (e *rateLimitedError) Error() string { return "request has been rate-limited" }

// Temporary tells the HTTP server loop that this error is temporary and recoverable
func (e *rateLimitedError) Temporary() bool { return true }

// Timeout tells the HTTP server loop that this error is not a timeout
func (e *rateLimitedError) Timeout() bool { return false }

// Accept reimplements the regular Accept but adds rate limiting.
func (sl *rateLimitedListener) Accept() (net.Conn, error) {
	if atomic.LoadInt32(&sl.connLease) <= 0 {
		// we've reached our cap for this lease period, reject the request
		return nil, &rateLimitedError{}
	}

	for {
		//Wait up to 1 second for Reads and Writes to the new connection
		sl.SetDeadline(time.Now().Add(time.Second))

		newConn, err := sl.TCPListener.Accept()

		if err != nil {
			netErr, ok := err.(net.Error)

			//If this is a timeout, then continue to wait for
			//new connections
			if ok && netErr.Timeout() && netErr.Temporary() {
				continue
			}
		}

		// decrement available conns
		atomic.AddInt32(&sl.connLease, -1)

		return newConn, err
	}
}

// Close wraps the Close method of the underlying tcp listener
func (sl *rateLimitedListener) Close() error {
	if !atomic.CompareAndSwapUint32(&sl.closed, 0, 1) {
		// already closed; avoid multiple calls if we're on go1.10
		// https://golang.org/issue/24803
		return nil
	}
	sl.exit <- struct{}{}
	<-sl.exit
	return sl.TCPListener.Close()
}
