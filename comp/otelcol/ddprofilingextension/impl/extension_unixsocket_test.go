// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !windows

// Package ddprofilingextension defines the OpenTelemetry Extension implementation.
package ddprofilingextensionimpl

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"

	log "github.com/DataDog/datadog-agent/comp/core/log/impl"
)

// newUnixSocketServer serves HTTP on a unix socket and returns its path.
//
// The socket is placed under /tmp rather than t.TempDir(): sockaddr_un.sun_path
// is capped at 104 bytes on darwin and 108 on linux, and TMPDIR on darwin is a
// long /var/folders/... path that spends most of that budget before a filename
// is appended. /tmp is safe to assume here only because this file is excluded
// from Windows by the build tag above.
func newUnixSocketServer(t *testing.T, handler http.Handler) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "ddprof")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	socket := filepath.Join(dir, "apm.socket")
	ln, err := net.Listen("unix", socket)
	require.NoError(t, err)

	server := &httptest.Server{Listener: ln, Config: &http.Server{Handler: handler}}
	server.Start()
	t.Cleanup(server.Close)

	return socket
}

func TestUnixSocket(t *testing.T) {
	require.True(t, hasUnixSocketSupport(), "every platform this file builds for can dial a unix socket")

	t.Run("config wins over environment", func(t *testing.T) {
		t.Setenv(ddUnixSocketEnvVar, "/from/env.socket")
		e := &ddExtension{cfg: &Config{UnixSocket: "/from/config.socket"}}
		assert.Equal(t, "/from/config.socket", e.unixSocket())
	})

	t.Run("falls back to the environment", func(t *testing.T) {
		t.Setenv(ddUnixSocketEnvVar, "/from/env.socket")
		e := &ddExtension{cfg: &Config{}}
		assert.Equal(t, "/from/env.socket", e.unixSocket())
	})

	// A blank env var is how a case config disables the socket without removing
	// the key, so it must not be mistaken for a socket path.
	t.Run("blank environment is unset", func(t *testing.T) {
		t.Setenv(ddUnixSocketEnvVar, "   ")
		e := &ddExtension{cfg: &Config{}}
		assert.Empty(t, e.unixSocket())
	})

	t.Run("unset everywhere", func(t *testing.T) {
		e := &ddExtension{cfg: &Config{}}
		assert.Empty(t, e.unixSocket())
	})
}

func TestAgentExtensionUnixSocket(t *testing.T) {
	got := make(chan string, 1)
	socket := newUnixSocketServer(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/profiling/v1/input", req.URL.Path)
		select {
		case got <- req.Header.Get("User-Agent"):
		default:
		}
		rw.WriteHeader(http.StatusAccepted)
	}))

	// testComponent{} carries a nil *pkgagent.Agent, so GetHTTPHandler would
	// panic if it were called. Passing it here is the assertion that the socket
	// path never builds the local forwarding server.
	ext, err := NewComponent(&Config{
		UnixSocket: socket,
		ProfilerOptions: ProfilerOptions{
			Period: 1,
		},
	}, component.BuildInfo{}, testComponent{}, log.NewTemporaryLoggerWithoutInit())
	require.NoError(t, err)

	e, ok := ext.(*ddExtension)
	require.True(t, ok)
	require.True(t, e.agentMode, "a non-nil trace agent must still select agent mode")

	require.NoError(t, ext.Start(context.Background(), componenttest.NewNopHost()))
	t.Cleanup(func() { assert.NoError(t, ext.Shutdown(context.Background())) })

	assert.Nil(t, e.server, "the local forwarding server must not be started when a socket is configured")

	select {
	case out := <-got:
		assert.Equal(t, "Go-http-client/1.1", out)
	case <-time.After(15 * time.Second):
		t.Fatal("Timed out waiting for a profile on the unix socket")
	}
}

func TestStandaloneExtensionUnixSocket(t *testing.T) {
	got := make(chan struct{}, 1)
	socket := newUnixSocketServer(t, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/profiling/v1/input", req.URL.Path)
		select {
		case got <- struct{}{}:
		default:
		}
		rw.WriteHeader(http.StatusAccepted)
	}))

	// AgentAddr is deliberately set to an address nothing is listening on: if the
	// socket did not take precedence, the profile upload would fail instead of
	// arriving below.
	ext, err := NewComponent(&Config{
		UnixSocket: socket,
		AgentAddr:  "127.0.0.1:1",
		ProfilerOptions: ProfilerOptions{
			Period: 1,
		},
	}, component.BuildInfo{}, nil, nil)
	require.NoError(t, err)

	require.NoError(t, ext.Start(context.Background(), componenttest.NewNopHost()))
	t.Cleanup(func() { assert.NoError(t, ext.Shutdown(context.Background())) })

	select {
	case <-got:
	case <-time.After(15 * time.Second):
		t.Fatal("Timed out waiting for a profile on the unix socket")
	}
}
