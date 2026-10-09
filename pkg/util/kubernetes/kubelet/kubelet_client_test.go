// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestKubeletClient(t *testing.T, serverURL string, timeout time.Duration) *kubeletClient {
	t.Helper()
	u, err := url.Parse(serverURL)
	require.NoError(t, err)
	kc, err := newForConfig(&kubeletClientConfig{scheme: "http", baseURL: u.Host}, timeout)
	require.NoError(t, err)
	return kc
}

// TestQueryWithRespStreamOutlivesTimeout checks that the client timeout cuts
// regular requests but not log streams.
func TestQueryWithRespStreamOutlivesTimeout(t *testing.T) {
	const timeout = 100 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("first\n"))
		w.(http.Flusher).Flush()
		time.Sleep(3 * timeout)
		w.Write([]byte("second\n"))
	}))
	defer server.Close()

	kc := newTestKubeletClient(t, server.URL, timeout)

	t.Run("stream client", func(t *testing.T) {
		body, err := kc.queryWithResp(context.Background(), "/containerLogs/ns/pod/container?follow=true")
		require.NoError(t, err)
		defer body.Close()

		data, err := io.ReadAll(body)
		require.NoError(t, err)
		require.Equal(t, "first\nsecond\n", string(data))
	})

	t.Run("regular client", func(t *testing.T) {
		_, resp, err := kc.rawQuery(context.Background(), kc.kubeletURL, "/pods")
		require.NoError(t, err)
		defer resp.Body.Close()

		_, err = io.ReadAll(resp.Body)
		require.Error(t, err)
	})
}

func TestQueryWithRespTimesOutWaitingForHeaders(t *testing.T) {
	const timeout = 100 * time.Millisecond
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	kc := newTestKubeletClient(t, server.URL, timeout)

	// Avoids hanging if the header timeout is missing.
	ctx, cancel := context.WithTimeout(context.Background(), 20*timeout)
	defer cancel()

	start := time.Now()
	_, err := kc.queryWithResp(ctx, "/containerLogs/ns/pod/container?follow=true")
	require.Error(t, err)
	require.Less(t, time.Since(start), 10*timeout, "the header timeout must fire before the request deadline")
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
