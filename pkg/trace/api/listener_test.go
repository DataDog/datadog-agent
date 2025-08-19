// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
)

type mockNetError struct{ timeout bool }

func (e mockNetError) Error() string   { return "mock-net-error" }
func (e mockNetError) Timeout() bool   { return e.timeout }
func (e mockNetError) Temporary() bool { return !e.timeout }

type mockListener struct {
	mu  sync.RWMutex // guards below fields
	err error        // returned error
}

var _ net.Listener = (*mockListener)(nil)

func (ml *mockListener) Accept() (net.Conn, error) {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	return nil, ml.err
}

func (ml *mockListener) SetSuccess() {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.err = nil
}

func (ml *mockListener) SetError() {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.err = mockNetError{timeout: false}
}

func (ml *mockListener) SetTimeoutError() {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.err = mockNetError{timeout: true}
}

func (ml *mockListener) SetArbitraryError(err error) {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.err = err
}

func (ml *mockListener) Close() error { return nil }

func (ml *mockListener) Addr() net.Addr { return nil }

func TestMockListener(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		var ln mockListener
		_, err := ln.Accept()
		assert.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		assert := assert.New(t)
		var ln mockListener
		ln.SetError()
		_, err := ln.Accept()
		nerr, ok := err.(net.Error)
		assert.True(ok)
		assert.True(nerr.Temporary())
		assert.False(nerr.Timeout())
	})

	t.Run("timeout", func(t *testing.T) {
		assert := assert.New(t)
		var ln mockListener
		ln.SetTimeoutError()
		_, err := ln.Accept()
		nerr, ok := err.(net.Error)
		assert.True(ok)
		assert.False(nerr.Temporary())
		assert.True(nerr.Timeout())
	})

	t.Run("toggle", func(t *testing.T) {
		assert := assert.New(t)
		var ln mockListener

		ln.SetTimeoutError()
		_, err := ln.Accept()
		nerr, ok := err.(net.Error)
		assert.True(ok)
		assert.False(nerr.Temporary())
		assert.True(nerr.Timeout())

		ln.SetSuccess()
		_, err = ln.Accept()
		assert.NoError(err)

		ln.SetError()
		_, err = ln.Accept()
		nerr, ok = err.(net.Error)
		assert.True(ok)
		assert.True(nerr.Temporary())
		assert.False(nerr.Timeout())

		ln.SetSuccess()
		_, err = ln.Accept()
		assert.NoError(err)
	})
}

func TestAcceptFailure(t *testing.T) {
	assert := assert.New(t)
	stats := &teststatsd.Client{}

	var mockln mockListener
	mockln.SetError()
	ln := NewMeasuredListener(&mockln, "test-metric", 5, stats).(*measuredListener)

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "OK")
		}),
	}

	expectErr := errors.New("unrecoverable error")
	go func() {
		time.Sleep(1 * time.Second)
		mockln.SetArbitraryError(expectErr)
	}()

	err := srv.Serve(ln)
	assert.Equal(expectErr, err)
}

func TestMeasuredListener(t *testing.T) {
	assert := assert.New(t)
	stats := &teststatsd.Client{}

	var mockln mockListener
	ln := NewMeasuredListener(&mockln, "test-metric", 1000, stats).(*measuredListener)
	mockln.SetSuccess()
	ln.Accept()
	ln.Accept()
	ln.Accept()
	ln.flushMetrics()
	call, ok := stats.GetCountSummaries()["datadog.trace_agent.receiver.test-metric"]
	assert.True(ok)
	assert.Len(call.Calls, 1)
	assert.EqualValues(call.Calls[0].Tags, []string{"status:accepted"})
	assert.EqualValues(call.Calls[0].Value, 3)

	stats.Reset()
	mockln.SetError()
	ln.Accept()
	ln.Accept()
	ln.flushMetrics()
	call, ok = stats.GetCountSummaries()["datadog.trace_agent.receiver.test-metric"]
	assert.True(ok)
	assert.Len(call.Calls, 1)
	assert.EqualValues(call.Calls[0].Tags, []string{"status:errored"})
	assert.EqualValues(call.Calls[0].Value, 2)

	stats.Reset()
	mockln.SetTimeoutError()
	ln.Accept()
	ln.flushMetrics()
	call, ok = stats.GetCountSummaries()["datadog.trace_agent.receiver.test-metric"]
	assert.True(ok)
	assert.Len(call.Calls, 1)
	assert.EqualValues(call.Calls[0].Tags, []string{"status:timedout"})
	assert.EqualValues(call.Calls[0].Value, 1)
}

func TestOnCloseConn(t *testing.T) {

	var closed int
	p, _ := net.Pipe()
	c := OnCloseConn(p, func() {
		closed++
	})

	c.Close()
	assert.Equal(t, 1, closed)
	// Make sure multiple close calls don't execute the
	// callback multiple times.
	c.Close()
	assert.Equal(t, 1, closed)
}
