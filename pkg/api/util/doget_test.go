// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func makeTestServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)
	return server
}

func TestDoGet(t *testing.T) {
	t.Run("simple request", func(t *testing.T) {
		handler := func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte("test"))
		}
		server := makeTestServer(t, http.HandlerFunc(handler))
		data, err := DoGet(server.Client(), server.URL, CloseConnection)
		require.NoError(t, err)
		require.Equal(t, "test", string(data))
	})

	t.Run("error response", func(t *testing.T) {
		handler := func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}
		server := makeTestServer(t, http.HandlerFunc(handler))
		_, err := DoGet(server.Client(), server.URL, CloseConnection)
		require.Error(t, err)
	})

	t.Run("url error", func(t *testing.T) {
		_, err := DoGet(http.DefaultClient, " http://localhost", CloseConnection)
		require.Error(t, err)
	})

	t.Run("request error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
		server.Close()

		_, err := DoGet(server.Client(), server.URL, CloseConnection)
		require.Error(t, err)
	})

	t.Run("check auth token", func(t *testing.T) {
		token = "mytoken"
		handler := func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}
		server := makeTestServer(t, http.HandlerFunc(handler))

		data, err := DoGet(server.Client(), server.URL, CloseConnection)
		require.NoError(t, err)
		require.Empty(t, data)
	})
}
