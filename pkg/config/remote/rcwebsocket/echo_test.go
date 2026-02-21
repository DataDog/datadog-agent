// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rcwebsocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

// Simulate a test run with a mixture of frame types sent by the server.
func TestWebSocketTest(t *testing.T) {
	upgrader := websocket.Upgrader{}
	auth := api.Auth{
		APIKey:    "bananas",
		PARJWT:    "platanos",
		AppKey:    "bananananana",
		UseAppKey: true,
	}

	type frame struct {
		messageType int
		data        []byte
	}

	testCases := []struct {
		desc string

		frames      []frame
		closeStatus []byte
	}{

		{
			desc:        "close with status",
			closeStatus: websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"),
		},
		{
			desc:        "close no status",
			closeStatus: []byte{},
		},
		{
			desc: "text frames",
			frames: []frame{
				{websocket.TextMessage, []byte("bananas")},
				{websocket.TextMessage, []byte("platanos")},
			},
		},
		{
			desc: "binary frames",
			frames: []frame{
				{websocket.BinaryMessage, []byte("bananas")},
				{websocket.BinaryMessage, []byte{0x00, 0x42, 0x00, 0x42}},
			},
		},
		{
			desc: "mixed frames",
			frames: []frame{
				{websocket.TextMessage, []byte("bananas")},
				{websocket.BinaryMessage, []byte{0x00, 0x42, 0x00, 0x42}},
				{websocket.TextMessage, []byte("platanos")},
			},
		},
		{
			desc: "binary frame - compression control",
			frames: []frame{
				{websocket.TextMessage, []byte("bananas")},
				{websocket.BinaryMessage, []byte("set_compress_on")}, // Binary magic frame
				{websocket.BinaryMessage, []byte{0x00, 0x42, 0x00, 0x42}},
				{websocket.TextMessage, []byte("platanos")},
				{websocket.BinaryMessage, []byte("set_compress_off")}, // Binary magic frame
				{websocket.BinaryMessage, []byte{0x00, 0x42, 0x00, 0x42}},
				{websocket.TextMessage, []byte("platanos")},
			},
		},
		{
			desc: "text frame - compression control",
			frames: []frame{
				{websocket.TextMessage, []byte("bananas")},
				{websocket.TextMessage, []byte("set_compress_on")}, // Text magic frame
				{websocket.BinaryMessage, []byte{0x00, 0x42, 0x00, 0x42}},
				{websocket.TextMessage, []byte("platanos")},
				{websocket.TextMessage, []byte("set_compress_off")}, // Text magic frame
				{websocket.BinaryMessage, []byte{0x00, 0x42, 0x00, 0x42}},
				{websocket.TextMessage, []byte("platanos")},
			},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.desc, func(t *testing.T) {
			assert := assert.New(t)
			agentConfig := mock.New(t)

			// TLS test uses bogus certs
			agentConfig.SetInTest("skip_ssl_validation", true)                    // Transport
			agentConfig.SetInTest("remote_configuration.no_tls_validation", true) // RC check

			ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Attempt to upgrade the HTTP connection into a WebSocket
				// connection.
				conn, err := upgrader.Upgrade(w, r, nil)
				assert.NoError(err)
				defer conn.Close()

				// Exchange the configured test frames.
				for _, frame := range tt.frames {
					err := conn.WriteMessage(frame.messageType, frame.data)
					assert.NoError(err)

					gotType, gotData, err := conn.ReadMessage()
					assert.NoError(err)

					// Echo messages must match exactly - control frames are not
					// returned by ReadMessage().
					assert.Equal(frame.messageType, gotType)
					assert.Equal(frame.data, gotData)
				}

				err = conn.WriteControl(websocket.CloseMessage, tt.closeStatus, time.Now().Add(time.Second))
				assert.NoError(err)
			}))
			defer ts.Close()

			// Configure and start the mock server.
			ts.StartTLS()

			url, err := url.Parse(ts.URL)
			assert.NoError(err)

			client, err := api.NewHTTPClient(auth, agentConfig, url)
			assert.NoError(err)

			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			// Drive the test and ensure the expected number of frames were
			// exchanged.
			n, err := runEchoLoop(ctx, client)
			assert.NoError(err)
			assert.Equal(uint(len(tt.frames)), n)
		})
	}
}

// Ensure the WebSocket PING handler dispatches a PONG frame in response.
func TestWebSocketTest_PING_PONG(t *testing.T) {
	upgrader := websocket.Upgrader{}
	assert := assert.New(t)
	agentConfig := mock.New(t)

	// TLS test uses bogus certs
	agentConfig.SetInTest("skip_ssl_validation", true)                    // Transport
	agentConfig.SetInTest("remote_configuration.no_tls_validation", true) // RC check

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Attempt to upgrade the HTTP connection into a WebSocket
		// connection.
		conn, err := upgrader.Upgrade(w, r, nil)
		assert.NoError(err)
		defer conn.Close()

		pongCh := make(chan struct{}, 1)
		conn.SetPongHandler(func(_data string) error {
			select {
			case pongCh <- struct{}{}:
			default:
			}
			return nil
		})

		// Read pump to drive internal delivery of PONGs to the PONG
		// handler.
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				conn.SetReadDeadline(time.Now().Add(time.Second))
				_, _, err := conn.ReadMessage()
				if err != nil {
					return
				}
			}
		}()

		// Send a "ping" control frame...
		err = conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second))
		assert.NoError(err)

		// ..and wait for the "pong".
		<-pongCh

		// Gracefully close the connection.
		err = conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(time.Second))
		assert.NoError(err)
	}))
	defer ts.Close()

	ts.StartTLS()
	url, err := url.Parse(ts.URL)
	assert.NoError(err)

	client, err := api.NewHTTPClient(api.Auth{}, agentConfig, url)
	assert.NoError(err)

	conn, err := client.NewWebSocket(ctx, "/bananas")
	assert.NoError(err)
	defer conn.Close()

	_, err = runEchoLoop(ctx, client)
	assert.NoError(err)
}
