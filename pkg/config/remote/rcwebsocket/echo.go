// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rcwebsocket implements WebSocket connectivity to the RC backend.
package rcwebsocket

import (
	"bytes"
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gorilla/websocket"
)

// The server must transmit a PING or DATA frame at least once per
// messageTimeout interval or the test times out.
const messageTimeout = 5 * time.Minute

// RunEchoTest connects to the echo test endpoint ("/api/v0.2/echo-test") in the
// Remote Config backend, upgrades the HTTP request to a WebSocket connection,
// and exchanges a series of data frames to measure connectivity, delivery and
// latency metrics.
//
// The server is expected to "drive" the test by sending frames of varying
// configurations and waiting for the client to echo them back. The connection
// is closed by the server upon test completion.
//
// The test continues as long as the connection remains open and the server
// sends a frame at least once every 5 minutes, otherwise the test times out and
// the connection is (ungracefully) closed.
//
// Cancel ctx to abort the test - the function will return after the next
// message arrives (or times out).
func RunEchoTest(ctx context.Context, client *api.HTTPClient) {
	log.Debug("starting remote config websocket echo test")

	n, err := runEchoLoop(ctx, client)
	if err != nil {
		log.Debugf("failed to run websocket echo test: %s (%d data frames exchanged)", err, n)
	}

	log.Debugf("remote config websocket test complete (%d data frames exchanged)", n)
}

func runEchoLoop(ctx context.Context, client *api.HTTPClient) (uint, error) {
	conn, err := client.NewWebSocket(ctx, "/api/v0.2/echo-test")
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	// Set a sensible safety limit on the amount of data a connection will
	// consume to prevent unbounded memory consumption.
	conn.SetReadLimit(1024 * 1024 * 50) // 50 MiB

	// Decorate the default ping handler (which returns a PONG frame) with
	// keepalive management.
	cb := conn.PingHandler()
	conn.SetPingHandler(
		func(message string) error {
			bumpReadDeadline(conn)
			return cb(message)
		},
	)

	// Perform the frame echo test routine.
	n := uint(0)
	for {
		select {
		case <-ctx.Done():
			gracefulAbort(conn)
			return n, context.Cause(ctx)
		default:
		}

		bumpReadDeadline(conn)
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			return n, mapWsClose(err)
		}

		// Allow the server to set client-side configuration remotely by
		// watching for "magic" payloads.
		switch {
		case bytes.Equal(p, []byte("set_compress_on")):
			conn.EnableWriteCompression(true)
		case bytes.Equal(p, []byte("set_compress_off")):
			conn.EnableWriteCompression(false)
		}

		_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		if err := conn.WriteMessage(messageType, p); err != nil {
			return n, mapWsClose(err)
		}

		n++
	}
}

// mapWsClose returns nil for a graceful WebSocket close error, or err for any
// other error.
func mapWsClose(err error) error {
	// CloseNormalClosure: the connection was gracefully closed with a status
	// code & message.
	//
	// CloseNoStatusReceived: the connection was gracefully closed without a
	// status / message.
	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
		return nil
	}
	return err
}

// Set the deadline for the next message from the server to be NOW() +
// messageTimeout.
func bumpReadDeadline(conn *websocket.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(messageTimeout))
}

// Attempt to gracefully close the websocket connection due to an interrupted
// test.
func gracefulAbort(conn *websocket.Conn) {
	_ = conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseGoingAway, "test cancel"),
		time.Now().Add(time.Second),
	)
}
