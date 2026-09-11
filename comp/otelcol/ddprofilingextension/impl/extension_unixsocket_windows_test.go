// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package ddprofilingextension defines the OpenTelemetry Extension implementation.
package ddprofilingextensionimpl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"

	log "github.com/DataDog/datadog-agent/comp/core/log/impl"
)

// TestUnixSocketUnsupported covers the Windows half of the platform split. A
// socket reaches this process the same way it reaches a Linux one -- a shared
// collector config, or an environment set across a mixed fleet -- so what
// matters is that it is discarded rather than acted on.
//
// Discarding it is also what keeps the rest of the extension on its normal
// path: both start functions branch on unixSocket() != "", so an empty result
// is a complete proof that agent mode still builds its local forwarding server
// and standalone mode still honours agent_addr.
func TestUnixSocketUnsupported(t *testing.T) {
	require.False(t, hasUnixSocketSupport(), "no Windows Agent serves an APM unix socket")

	t.Run("config key is ignored", func(t *testing.T) {
		e := &ddExtension{cfg: &Config{UnixSocket: `C:\ProgramData\Datadog\apm.socket`}, log: log.NewTemporaryLoggerWithoutInit()}
		assert.Empty(t, e.unixSocket())
	})

	t.Run("environment is ignored", func(t *testing.T) {
		t.Setenv(ddUnixSocketEnvVar, "/var/run/datadog/apm.socket")
		e := &ddExtension{cfg: &Config{}, log: log.NewTemporaryLoggerWithoutInit()}
		assert.Empty(t, e.unixSocket())
	})

	// Standalone mode is constructed without a logger, so the ignore path has to
	// survive a nil one. Reaching the assertion at all is the test.
	t.Run("nil logger does not panic", func(t *testing.T) {
		t.Setenv(ddUnixSocketEnvVar, "/var/run/datadog/apm.socket")
		e := &ddExtension{cfg: &Config{UnixSocket: "/var/run/datadog/apm.socket"}}
		assert.Empty(t, e.unixSocket())
	})
}

// TestStandaloneExtensionUnixSocketFallsBackToHTTP is the end-to-end statement
// of the same thing: configuring a socket on Windows must degrade to the
// address-based transport, not disable profiling.
func TestStandaloneExtensionUnixSocketFallsBackToHTTP(t *testing.T) {
	got := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/profiling/v1/input", req.URL.Path)
		select {
		case got <- struct{}{}:
		default:
		}
		rw.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	// A nil logger, as NewFactory would supply.
	ext, err := NewComponent(&Config{
		UnixSocket: `C:\ProgramData\Datadog\apm.socket`,
		AgentAddr:  server.Listener.Addr().String(),
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
		t.Fatal("Timed out waiting for a profile over HTTP")
	}
}
