// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func makeTestServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)
	return server
}

func TestDoGet(t *testing.T) {
	t.Run("simple request", func(t *testing.T) {
		handler := func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("test"))
		}
		server := makeTestServer(t, http.HandlerFunc(handler))
		data, err := DoGet(server.Client(), server.URL, CloseConnection)
		require.NoError(t, err)
		require.Equal(t, "test", string(data))
	})

	t.Run("error response", func(t *testing.T) {
		handler := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}
		server := makeTestServer(t, http.HandlerFunc(handler))
		_, err := DoGetWithOptions(server.Client(), server.URL, &ReqOptions{})
		require.Error(t, err)
	})

	t.Run("url error", func(t *testing.T) {
		_, err := DoGetWithOptions(http.DefaultClient, " http://localhost", &ReqOptions{})
		require.Error(t, err)
	})

	t.Run("request error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		server.Close()

		_, err := DoGetWithOptions(server.Client(), server.URL, &ReqOptions{})
		require.Error(t, err)
	})

	t.Run("check auth token", func(t *testing.T) {
		handler := func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer mytoken", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}
		server := makeTestServer(t, http.HandlerFunc(handler))

		options := &ReqOptions{Authtoken: "mytoken"}
		data, err := DoGetWithOptions(server.Client(), server.URL, options)
		require.NoError(t, err)
		require.Empty(t, data)
	})

	t.Run("check global auth token", func(t *testing.T) {
		handler := func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer 0123456789abcdef0123456789abcdef", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}
		server := makeTestServer(t, http.HandlerFunc(handler))

		cfg := model.NewConfig("datadog", "test", strings.NewReplacer("_", "."))
		dir := t.TempDir()
		authTokenPath := dir + "/auth_token"
		err := os.WriteFile(authTokenPath, []byte("0123456789abcdef0123456789abcdef"), 0644)
		require.NoError(t, err)
		cfg.SetWithoutSource("auth_token_file_path", authTokenPath)
		SetAuthToken(cfg)

		options := &ReqOptions{}
		data, err := DoGetWithOptions(server.Client(), server.URL, options)
		require.NoError(t, err)
		require.Empty(t, data)
	})

	t.Run("context cancel", func(t *testing.T) {
		handler := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}
		server := makeTestServer(t, http.HandlerFunc(handler))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		options := &ReqOptions{Ctx: ctx}
		_, err := DoGetWithOptions(server.Client(), server.URL, options)
		require.Error(t, err)
	})
}
