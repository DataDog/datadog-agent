// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rcprotocoltestimpl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

// The server must transmit a PING or DATA frame at least once per
// messageTimeout interval or the test times out.
const messageTimeout = 5 * time.Minute

// ALPNMode specifies the ALPN protocol mode for WebSocket connections.
type ALPNMode int

const (
	// ALPNDefault uses no ALPN protocol negotiation.
	ALPNDefault ALPNMode = 0
	// ALPNDDRC uses the dd-rc-v1 ALPN protocol.
	ALPNDDRC ALPNMode = 1
)

// alpnProtocolDDRC is the ALPN protocol identifier for Datadog Remote Config
// WebSocket connections. This protocol enables optimized routing and handling
// of remote config traffic at the load balancer and backend level.
const alpnProtocolDDRC = "dd-rc-v1"

func runEchoLoop(ctx context.Context, client *api.HTTPClient, runCount uint64, alpnMode ALPNMode) (uint, error) {
	conn, err := newWebSocketClient(ctx, "/api/v0.2/echo-test", client, runCount, alpnMode)
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

// newWebSocketClient connects to the RC WebSocket backend and returns a new
// WebSocket connection or a connection / handshake error.
//
// The "endpointPath" specifies the resource path to connect to, which is
// appended to the client baseURL.
//
// The "alpnMode" specifies the ALPN protocol mode. Use ALPNDefault for no ALPN
// or ALPNDDRC for dd-rc-v1 ALPN protocol.
func newWebSocketClient(ctx context.Context, endpointPath string, httpClient *api.HTTPClient, runCount uint64, alpnMode ALPNMode) (*websocket.Conn, error) {
	// Extract the TLS & Proxy configuration from the HTTP client.
	transport, err := httpClient.Transport()
	if err != nil {
		return nil, err
	}

	tlsConfig := transport.TLSClientConfig

	// Parse the "base URL" the client uses to connect to RC.
	url, err := httpClient.BaseURL()
	if err != nil {
		return nil, err
	}

	// Handle ALPN if requested.
	if alpnMode == ALPNDDRC {
		// ALPN requires TLS, so this test cannot run with plain HTTP.
		if strings.ToLower(url.Scheme) == "http" {
			return nil, errors.New("ALPN websocket test requires TLS (remote_configuration.no_tls must be false)")
		}

		// Clone and configure TLS for ALPN.
		tlsConfig = tlsConfig.Clone()
		tlsConfig.NextProtos = []string{alpnProtocolDDRC}
	}

	dialer := &websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
		TLSClientConfig:  tlsConfig,
		Proxy:            transport.Proxy,
	}

	// The WebSocket request MUST include the same auth credentials as the plain
	// HTTP requests.
	headers := httpClient.Headers()
	// In addition to extra debug headers.
	headers.Set("X-Echo-Run-Count", strconv.FormatUint(runCount, 10))
	headers.Set("X-Agent-UUID", uuid.GetUUID())

	// Append the specific path to the WebSocket resource.
	url.Path = path.Join(url.Path, endpointPath)
	// Change the protocol to use websockets.
	switch strings.ToLower(url.Scheme) {
	case "http":
		url.Scheme = "ws"
	case "https":
		url.Scheme = "wss"
	}

	logMsg := "connecting to websocket endpoint " + url.String()
	if alpnMode == ALPNDDRC {
		logMsg += " with ALPN " + alpnProtocolDDRC
	}
	log.Debug(logMsg)

	// Send the HTTP request, wait for the upgrade response and then perform the
	// WebSocket handshake.
	conn, resp, err := dialer.DialContext(ctx, url.String(), headers)
	if err != nil {
		return nil, fmt.Errorf("failed to open websocket connection: %s", err)
	}
	_ = resp.Body.Close()

	logMsg = "websocket connected"
	if alpnMode == ALPNDDRC {
		logMsg += " with ALPN " + alpnProtocolDDRC
	}
	log.Debug(logMsg)

	return conn, nil
}
