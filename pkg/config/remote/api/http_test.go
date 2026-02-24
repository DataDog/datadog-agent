// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

// Test WebSocket connectivity under varying transport configurations, and
// assert various transport-level properties such as credential transmission.
func TestNewWebSocket(t *testing.T) {
	upgrader := websocket.Upgrader{}
	auth := Auth{
		APIKey:    "bananas",
		PARJWT:    "platanos",
		AppKey:    "banana_key",
		UseAppKey: true,
	}

	testCases := []struct {
		desc string
		path string

		serverHTTP2 bool
		serverTLS   bool
	}{
		{
			desc:        "tls with http1",
			path:        "/",
			serverHTTP2: false,
			serverTLS:   true,
		},
		{
			desc:        "tls with http2",
			path:        "/",
			serverHTTP2: true,
			serverTLS:   true,
		},
		{
			desc:        "plain with http1",
			path:        "/",
			serverHTTP2: false,
			serverTLS:   false,
		},
		{
			desc:        "plain with http2",
			path:        "/",
			serverHTTP2: true,
			serverTLS:   false,
		},
		{
			desc:        "path",
			path:        "/api/v0.2/echo",
			serverHTTP2: false,
			serverTLS:   false,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.desc, func(t *testing.T) {
			assert := assert.New(t)
			agentConfig := mock.New(t)

			// TLS test uses bogus certs
			agentConfig.SetInTest("skip_ssl_validation", true)                    // Transport
			agentConfig.SetInTest("remote_configuration.no_tls_validation", true) // RC check

			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(tt.path, r.URL.Path)

				// Inspect the auth headers ensuring they were sent in the
				// request.
				assert.Equal(auth.PARJWT, r.Header.Get("DD-PAR-JWT"))
				assert.Equal(auth.APIKey, r.Header.Get("DD-Api-Key"))
				assert.Equal(auth.AppKey, r.Header.Get("DD-Application-Key"))

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
				select {
				case <-pongCh:
				case <-ctx.Done():
					t.Fatal("ping timeout")
					return
				}

				// Gracefully close the connection.
				err = conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(time.Second))
				assert.NoError(err)
			}))
			defer ts.Close()

			// Configure and start the mock server.
			ts.EnableHTTP2 = tt.serverHTTP2
			if tt.serverTLS {
				ts.StartTLS()
			} else {
				// TLS requires an explicit config opt-in.
				agentConfig.SetInTest("remote_configuration.no_tls", true)
				ts.Start()
			}

			url, err := url.Parse(ts.URL)
			assert.NoError(err)

			client, err := NewHTTPClient(auth, agentConfig, url)
			assert.NoError(err)

			conn, err := client.NewWebSocket(ctx, tt.path)
			assert.NoError(err)
			defer conn.Close()

			// Read the connection to drive the internal ping / pong handler.
			_, _, err = conn.ReadMessage()
			assert.True(websocket.IsCloseError(
				err,
				websocket.CloseNormalClosure,
				websocket.CloseNoStatusReceived,
			))
		})
	}
}

func TestUserAgent(t *testing.T) {
	assert := assert.New(t)
	agentConfig := mock.New(t)

	// TLS test uses bogus certs
	agentConfig.SetInTest("skip_ssl_validation", true)                    // Transport
	agentConfig.SetInTest("remote_configuration.no_tls_validation", true) // RC check
	agentConfig.SetInTest("remote_configuration.no_tls", true)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	userAgentCh := make(chan string, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		userAgentCh <- r.UserAgent()
	}))
	defer ts.Close()

	url, err := url.Parse(ts.URL)
	assert.NoError(err)

	client, err := NewHTTPClient(Auth{}, agentConfig, url)
	assert.NoError(err)

	_, _ = client.FetchOrgData(ctx)

	select {
	case ua := <-userAgentCh:
		// Regex explained:
		//   * ^datadog-agent\/ == must start with "datadog-agent/"
		//   * (unknown|\d+\.\d+\.\d+) == either "unknown" or a semver string
		//   * \(go\d+\.\d+\.\d+\)$ == ends in " (go1.2.3)" where 1.2.3 is a semver string
		uaRegex := regexp.MustCompile(`^datadog-agent\/(.+) \(go\d+\.\d+\.\d+\)$`)
		parts := uaRegex.FindStringSubmatch(ua)
		assert.Len(parts, 2) // Original string + the extracted group.

		// The extracted string must match either "unknown" or be valid semver.
		switch parts[1] {
		case "unknown":
		default:
			_, err = semver.NewVersion(parts[1])
			assert.NoError(err)
		}

	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for user agent string")
	}
}
