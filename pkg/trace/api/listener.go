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

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// rateLimitedListener wraps a regular TCPListener with rate limiting.
type rateLimitedListener struct {
	*net.TCPListener

	lease  int32         // connections allowed until refresh
	exit   chan struct{} // exit notification channel
	closed uint32        // closed will be non-zero if the listener was closed

	// stats
	accepted uint32
	rejected uint32
	timedout uint32
	errored  uint32
}

// newRateLimitedListener returns a new wrapped listener, which is non-initialized
func newRateLimitedListener(l net.Listener, conns int) (*rateLimitedListener, error) {
	tcpL, ok := l.(*net.TCPListener)

	if !ok {
		return nil, errors.New("cannot wrap listener")
	}

	return &rateLimitedListener{
		lease:       int32(conns),
		TCPListener: tcpL,
		exit:        make(chan struct{}),
	}, nil
}

// Refresh periodically refreshes the connection lease, and thus cancels any rate limits in place
func (sl *rateLimitedListener) Refresh(conns int) {
	defer close(sl.exit)

	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	tickStats := time.NewTicker(10 * time.Second)
	defer tickStats.Stop()

	for {
		select {
		case <-sl.exit:
			return
		case <-tickStats.C:
			for tag, stat := range map[string]*uint32{
				"status:accepted": &sl.accepted,
				"status:rejected": &sl.rejected,
				"status:timedout": &sl.timedout,
				"status:errored":  &sl.errored,
			} {
				v := int64(atomic.SwapUint32(stat, 0))
				metrics.Count("datadog.trace_agent.receiver.tcp_connections", v, []string{tag}, 1)
			}
		case <-t.C:
			atomic.StoreInt32(&sl.lease, int32(conns))
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
	if atomic.LoadInt32(&sl.lease) <= 0 {
		// we've reached our cap for this lease period; reject the request
		atomic.AddUint32(&sl.rejected, 1)
		return nil, &rateLimitedError{}
	}
	for {
		// ensure potential TCP handshake timeouts don't stall us forever
		sl.SetDeadline(time.Now().Add(time.Second))
		conn, err := sl.TCPListener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if ne.Temporary() {
					// deadline expired; continue
					continue
				} else {
					// don't count temporary errors; they usually signify expired deadlines
					// see (golang/go/src/internal/poll/fd.go).TimeoutError
					atomic.AddUint32(&sl.timedout, 1)
				}
			} else {
				atomic.AddUint32(&sl.errored, 1)
			}
			return conn, err
		}

		atomic.AddInt32(&sl.lease, -1)
		atomic.AddUint32(&sl.accepted, 1)

		return conn, nil
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
