// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package api

import (
	"context"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConnContextConnectionTypeUnix(t *testing.T) {

	t.Run("uds connection", func(t *testing.T) {
		sockPath := "/tmp/test-transport.sock"
		os.Remove(sockPath)
		t.Cleanup(func() { os.Remove(sockPath) })
		ln, err := net.Listen("unix", sockPath)
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()

		done := make(chan net.Conn, 1)
		go func() {
			c, _ := ln.Accept()
			done <- c
		}()
		clientConn, err := net.Dial("unix", sockPath)
		if err != nil {
			t.Fatal(err)
		}
		defer clientConn.Close()
		serverConn := <-done
		defer serverConn.Close()

		ctx := connContext(context.Background(), serverConn)
		assert.Equal(t, ConnectionTypeUDS, GetConnectionType(ctx))
	})

	t.Run("unknown connection type defaults to ConnectionTypeUnknown", func(t *testing.T) {
		// A connection type that is neither *net.TCPConn nor *net.UnixConn
		// should fall through to the default case (ConnectionTypeUnknown on unix).
		ctx := connContext(context.Background(), &fakeConn{})
		assert.Equal(t, ConnectionTypeUnknown, GetConnectionType(ctx))
	})
}

// fakeConn implements net.Conn for testing the default/pipe branch.
type fakeConn struct{ net.Conn }
