// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextension defines the OpenTelemetry Extension implementation.
package ddprofilingextensionimpl

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/ptrace"

	log "github.com/DataDog/datadog-agent/comp/core/log/impl"
	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	otlpattributes "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	otlpsource "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
)

type testComponent struct {
	*pkgagent.Agent
}

func (c testComponent) SetOTelAttributeTranslator(attrstrans *otlpattributes.Translator) {
	c.Agent.OTLPReceiver.SetOTelAttributeTranslator(attrstrans)
}

func (c testComponent) ReceiveOTLPSpans(ctx context.Context, rspans ptrace.ResourceSpans, httpHeader http.Header, hostFromAttributesHandler otlpattributes.HostFromAttributesHandler) (otlpsource.Source, error) {
	return c.Agent.OTLPReceiver.ReceiveResourceSpans(ctx, rspans, httpHeader, hostFromAttributesHandler)
}

func (c testComponent) SendStatsPayload(p *pb.StatsPayload) {
	c.Agent.StatsWriter.SendPayload(p)
}

func (c testComponent) GetHTTPHandler(endpoint string) http.Handler {
	c.Agent.Receiver.BuildHandlers()
	if v, ok := c.Agent.Receiver.Handlers[endpoint]; ok {
		return v
	}
	return nil
}

type hostWithExtensions struct {
	component.Host
	exts map[component.ID]component.Component
}

func newHostWithExtensions(exts map[component.ID]component.Component) component.Host {
	return &hostWithExtensions{
		Host: componenttest.NewNopHost(),
		exts: exts,
	}
}

func (h *hostWithExtensions) GetExtensions() map[component.ID]component.Component {
	return h.exts
}

func TestNewComponent(t *testing.T) {
	ext, err := NewComponent(&Config{}, component.BuildInfo{}, testComponent{}, log.NewTemporaryLoggerWithoutInit())
	assert.NoError(t, err)

	_, ok := ext.(*ddExtension)
	require.True(t, ok)
}

func testServer(t *testing.T, got chan string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		assert.NoError(t, err)
		assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", req.Header.Get("DD-Api-Key"))
		got <- req.Header.Get("User-Agent")
		rw.WriteHeader(http.StatusAccepted)
	}))
}

func TestAgentExtension(t *testing.T) {
	// fake intake
	got := make(chan string, 1)
	server := testServer(t, got)
	defer server.Close()

	// create agent
	tcfg := config.New()
	tcfg.ReceiverEnabled = false
	tcfg.Endpoints[0].APIKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tcfg.DecoderTimeout = 10000
	tcfg.ProfilingProxy = config.ProfilingProxyConfig{DDURL: server.URL}
	ctx := context.Background()
	traceagent := pkgagent.NewAgent(ctx, tcfg, telemetry.NewNoopCollector(), &ddgostatsd.NoOpClient{}, gzip.NewComponent())

	// create extension
	ext, err := NewComponent(&Config{
		ProfilerOptions: ProfilerOptions{
			Period: 1,
		},
	}, component.BuildInfo{}, testComponent{traceagent}, log.NewTemporaryLoggerWithoutInit())
	assert.NoError(t, err)

	ext, ok := ext.(*ddExtension)
	require.True(t, ok)

	host := newHostWithExtensions(
		map[component.ID]component.Component{
			component.MustNewIDWithName("ddprofiling", "custom"): nil,
		},
	)

	err = ext.Start(context.Background(), host)
	assert.NoError(t, err)

	timeout := time.After(15 * time.Second)
	select {
	case out := <-got:
		assert.Equal(t, "Go-http-client/1.1", out)
	case <-timeout:
		t.Fatal("Timed out")
	}
	err = ext.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestStandaloneExtension(t *testing.T) {
	got := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/profiling/v1/input", req.URL.Path)
		got <- req.Header.Get("User-Agent")
		rw.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	ext, err := NewComponent(&Config{
		AgentAddr: server.Listener.Addr().String(),
		ProfilerOptions: ProfilerOptions{
			Period: 1,
		},
	}, component.BuildInfo{}, nil, nil)
	require.NoError(t, err)

	err = ext.Start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	timeout := time.After(15 * time.Second)
	select {
	case out := <-got:
		assert.Equal(t, "Go-http-client/1.1", out)
	case <-timeout:
		t.Fatal("Timed out")
	}

	err = ext.Shutdown(context.Background())
	require.NoError(t, err)
}

// newUnixSocketServer serves HTTP on a unix socket and returns its path.
//
// The socket is placed under /tmp rather than t.TempDir(): sockaddr_un.sun_path
// is capped at 104 bytes on darwin and 108 on linux, and t.TempDir() on darwin
// alone can spend most of that budget before the filename is appended.
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
