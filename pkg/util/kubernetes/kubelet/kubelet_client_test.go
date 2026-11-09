// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

// pipeListener serves HTTP over in-memory connections. Real sockets would
// keep a synctest bubble's fake clock from advancing.
type pipeListener struct {
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn), closed: make(chan struct{})}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

func (l *pipeListener) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case l.conns <- server:
		return client, nil
	case <-l.closed:
		return nil, net.ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

// newPipeKubeletClient returns a kubelet client whose requests are served by
// handler over in-memory connections, and a function that shuts it down.
func newPipeKubeletClient(t *testing.T, handler http.Handler, timeout time.Duration) (*kubeletClient, func()) {
	t.Helper()
	kc, err := newForConfig(&kubeletClientConfig{scheme: "http", baseURL: "kubelet"}, timeout)
	require.NoError(t, err)

	listener := newPipeListener()
	server := &http.Server{Handler: handler}
	go server.Serve(listener)

	// Both clients share this transport.
	transport := kc.client.Transport.(*http.Transport)
	transport.DialContext = listener.dial
	transport.Proxy = nil

	return kc, func() {
		server.Close()
		transport.CloseIdleConnections()
	}
}

// TestQueryWithRespStreamOutlivesTimeout checks that the client timeout cuts
// regular requests but not log streams.
func TestQueryWithRespStreamOutlivesTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const timeout = 30 * time.Second
		kc, shutdown := newPipeKubeletClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("first\n"))
			w.(http.Flusher).Flush()
			select {
			case <-time.After(3 * timeout):
				w.Write([]byte("second\n"))
			case <-r.Context().Done():
			}
		}), timeout)
		defer shutdown()

		body, err := kc.queryWithResp(context.Background(), "/containerLogs/ns/pod/container?follow=true")
		require.NoError(t, err)
		data, err := io.ReadAll(body)
		body.Close()
		require.NoError(t, err)
		require.Equal(t, "first\nsecond\n", string(data), "the stream client must not time out")

		_, resp, err := kc.rawQuery(context.Background(), kc.kubeletURL, "/pods")
		require.NoError(t, err)
		_, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		require.Error(t, err, "the regular client must time out")
	})
}

func TestQueryWithRespTimesOutWaitingForHeaders(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const timeout = 30 * time.Second
		kc, shutdown := newPipeKubeletClient(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}), timeout)
		defer shutdown()

		// Avoids waiting forever if the header timeout is missing.
		ctx, cancel := context.WithTimeout(context.Background(), 2*timeout)
		defer cancel()

		start := time.Now()
		_, err := kc.queryWithResp(ctx, "/containerLogs/ns/pod/container?follow=true")
		require.Error(t, err)
		require.Equal(t, timeout, time.Since(start), "the header timeout must fire")
	})
}

func TestRawQuery(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		body    string
		ctx     context.Context
		wantErr bool
	}{
		{
			name: "success",
			path: "/api/v1/path",
			ctx:  context.Background(),
			body: "success",
		},
		{
			name:    "failure_nil_context",
			path:    "/api/v1/path",
			ctx:     nil,
			body:    "fail",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Errorf("Expected path: %s, got: %s", tt.path, r.URL.Path)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.body))
			}))

			defer server.Close()

			kc := &kubeletClient{
				client: http.Client{},
			}

			_, resp, err := kc.rawQuery(tt.ctx, server.URL, tt.path)

			if tt.wantErr {
				// Check that we have en error when we want one
				require.NotNil(t, err)
			} else {
				// Validate that the response matches
				defer resp.Body.Close()
				bytes, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Errorf("cannot read response bytes: %v", err)
					return
				}

				require.Equal(t, tt.body, string(bytes))

			}

		})
	}
}

func TestQuery(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		desiredPath  string
		desiredQuery string
		useAPIServer bool
	}{
		{
			name:         "do_not_use_apiserver",
			useAPIServer: false,
			desiredPath:  kubeletPodPath,
			status:       http.StatusOK,
		},
		{
			name:         "use_apiserver",
			useAPIServer: true,
			desiredPath:  "/api/v1/pods",                    // intended to match apiServerQuery
			desiredQuery: "fieldSelector=spec.nodeName=abc", // nodeName is hardcoded to abc below
			status:       http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubelet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.useAPIServer {
					t.Error("expected to hit API server but hit the kubelet")
				}
				if r.URL.Path != tt.desiredPath {
					t.Errorf("expected path: %s, got: %s", tt.desiredPath, r.URL.Path)
				}
				if r.URL.RawQuery != tt.desiredQuery {
					t.Errorf("expected query: %s, got: %s", tt.desiredQuery, r.URL.RawQuery)
				}
				w.WriteHeader(tt.status)
			}))
			defer kubelet.Close()

			APIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !tt.useAPIServer {
					t.Error("expected to hit kubelet but hit the API server")
				}
				if r.URL.Path != tt.desiredPath {
					t.Errorf("expected path: %s, got: %s", tt.desiredPath, r.URL.Path)
				}
				if r.URL.RawQuery != tt.desiredQuery {
					t.Errorf("expected query: %s, got: %s", tt.desiredQuery, r.URL.RawQuery)
				}
				w.WriteHeader(tt.status)
			}))
			defer APIServer.Close()

			kc := &kubeletClient{
				client: http.Client{},
				config: &kubeletClientConfig{
					useAPIServer:  tt.useAPIServer,
					apiServerHost: APIServer.URL,
					nodeName:      "abc",
				},
				kubeletURL: kubelet.URL,
			}

			_, status, err := kc.query(context.Background(), kubeletPodPath)

			// We never expect an error given the current test cases
			if err != nil {
				t.Errorf("did not expect an error but got: %v", err)
			}

			// Check that the status is returned properly
			require.Equal(t, tt.status, status)

		})
	}
}
