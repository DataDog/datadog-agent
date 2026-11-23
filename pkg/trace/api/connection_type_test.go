// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package api

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetConnectionType(t *testing.T) {
	t.Run("empty context", func(t *testing.T) {
		ctx := t.Context()
		assert.Equal(t, ConnectionType(""), GetConnectionType(ctx))
	})

	t.Run("tcp", func(t *testing.T) {
		ctx := withConnectionType(t.Context(), ConnectionTypeTCP)
		assert.Equal(t, ConnectionTypeTCP, GetConnectionType(ctx))
	})

	t.Run("uds", func(t *testing.T) {
		ctx := withConnectionType(t.Context(), ConnectionTypeUDS)
		assert.Equal(t, ConnectionTypeUDS, GetConnectionType(ctx))
	})

	t.Run("pipe", func(t *testing.T) {
		ctx := withConnectionType(t.Context(), ConnectionTypePipe)
		assert.Equal(t, ConnectionTypePipe, GetConnectionType(ctx))
	})
}

func TestConnContextConnectionType(t *testing.T) {
	t.Run("tcp connection", func(t *testing.T) {
		// Create a real TCP listener/connection pair
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()

		done := make(chan net.Conn, 1)
		go func() {
			c, _ := ln.Accept()
			done <- c
		}()
		clientConn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer clientConn.Close()
		serverConn := <-done
		defer serverConn.Close()

		ctx := connContext(t.Context(), serverConn)
		assert.Equal(t, ConnectionTypeTCP, GetConnectionType(ctx))
	})
}
