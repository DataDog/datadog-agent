// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package guiimpl

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

var (
	loopbackV4 = net.ParseIP("127.0.0.1")
	loopbackV6 = net.ParseIP("::1")
)

// testProcessAgentSID is an arbitrary well-formed SID a mock process-agent hands back for any connection
// asked about by setupPeerIdentityResolutionForTest below; the real connection queried is always the test
// binary's own (both ends of the loopback connection run in the same process), but the test only needs to
// know that a real HTTP round trip through resolveConnectionOwnerSID resolved to a non-empty identity.
const testProcessAgentSID = "S-1-5-21-3623811015-3361044348-30300820-1013"

func init() {
	// On Windows, resolveConnectionOwnerSID requires a configured process-agent IPC client (see
	// peeridentity_windows.go); without this, Test_intentToken_peerIdentity's "same OS identity" subtest
	// would always resolve to an empty identity, since minting always goes through a real loopback connection.
	setupPeerIdentityResolutionForTest = func(t *testing.T) {
		resetPeerIdentityResolution(t)

		ipcMock := ipcmock.New(t)
		cfg := configmock.New(t)

		mux := http.NewServeMux()
		mux.HandleFunc("GET /connection/owner-sid", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte(testProcessAgentSID))
		})
		ts := ipcMock.NewMockServer(mux)
		pointConfigAtMockServer(t, cfg, ts)

		configurePeerIdentityResolution(ipcMock, cfg)
	}
}

func TestElevatedMintIdentity_Windows(t *testing.T) {
	assert.Equal(t, rootIdentity, mintTimeIdentity(rootIdentity))
}

// resetPeerIdentityResolution clears the package-level state configurePeerIdentityResolution writes, so
// one test's IPC client/config never leaks into another's. These tests never run in parallel with each
// other or with anything else that might call configurePeerIdentityResolution, since that state is
// unsynchronized by design (see peeridentity.go).
func resetPeerIdentityResolution(t *testing.T) {
	t.Cleanup(func() { configurePeerIdentityResolution(nil, nil) })
}

// pointConfigAtMockServer sets the keys resolveConnectionOwnerSID actually reads (cmd_host,
// process_config.cmd_port) on cfg from ts's real address. cfg and ipcMock's internal config are the same
// underlying instance (pkg/config/mock.New is a singleton within a test), so ipcMock.NewMockServer's
// cmd_host write already lands on cfg; the one gap this closes is the port key: NewMockServer sets plain
// cmd_port, but GetProcessAPIAddressPort reads process_config.cmd_port, so without this,
// resolveConnectionOwnerSID always dials the default port (6162) rather than the mock server's actual
// ephemeral one.
func pointConfigAtMockServer(t *testing.T, cfg model.BuildableConfig, ts *httptest.Server) {
	addr, err := url.Parse(ts.URL)
	require.NoError(t, err)
	host, portStr, err := net.SplitHostPort(addr.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	cfg.SetInTest("cmd_host", host)
	cfg.SetInTest("process_config.cmd_port", port)
}

func TestResolveConnectionOwnerSID_QueriesProcessAgent(t *testing.T) {
	resetPeerIdentityResolution(t)

	ipcMock := ipcmock.New(t)
	cfg := configmock.New(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /connection/owner-sid", func(w http.ResponseWriter, r *http.Request) {
		// The full connection 4-tuple must reach process-agent, since it (not the GUI) resolves and pins
		// the owning PID.
		q := r.URL.Query()
		assert.Equal(t, "4", q.Get("family"))
		assert.Equal(t, "127.0.0.1", q.Get("laddr"))
		assert.Equal(t, "54321", q.Get("lport"))
		assert.Equal(t, "127.0.0.1", q.Get("raddr"))
		assert.Equal(t, "5002", q.Get("rport"))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("S-1-5-21-3623811015-3361044348-30300820-1013\n"))
	})
	ts := ipcMock.NewMockServer(mux)
	pointConfigAtMockServer(t, cfg, ts)

	configurePeerIdentityResolution(ipcMock, cfg)

	id, err := resolveConnectionOwnerSID("4", loopbackV4, 54321, loopbackV4, 5002)
	require.NoError(t, err)
	// The handler's trailing newline (defensive on the caller's part; the real handler never adds one)
	// must be trimmed, not folded into the returned identity.
	assert.Equal(t, peerIdentity("S-1-5-21-3623811015-3361044348-30300820-1013"), id)
}

func TestResolveConnectionOwnerSID_IPv6(t *testing.T) {
	resetPeerIdentityResolution(t)

	ipcMock := ipcmock.New(t)
	cfg := configmock.New(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /connection/owner-sid", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assert.Equal(t, "6", q.Get("family"))
		assert.Equal(t, "::1", q.Get("laddr"))
		assert.Equal(t, "::1", q.Get("raddr"))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(testProcessAgentSID))
	})
	ts := ipcMock.NewMockServer(mux)
	pointConfigAtMockServer(t, cfg, ts)

	configurePeerIdentityResolution(ipcMock, cfg)

	id, err := resolveConnectionOwnerSID("6", loopbackV6, 54321, loopbackV6, 5002)
	require.NoError(t, err)
	assert.Equal(t, peerIdentity(testProcessAgentSID), id)
}

func TestResolveConnectionOwnerSID_NotConfigured(t *testing.T) {
	resetPeerIdentityResolution(t)
	// No usable IPC client/config: resolveConnectionOwnerSID must fail closed (empty identity, non-nil
	// error) rather than panic on the nil processAgentIPC/processAgentConfig.
	configurePeerIdentityResolution(nil, nil)

	id, err := resolveConnectionOwnerSID("4", loopbackV4, 54321, loopbackV4, 5002)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
	assert.Equal(t, peerIdentity(""), id)
}

func TestResolveConnectionOwnerSID_ErrorFromProcessAgent(t *testing.T) {
	resetPeerIdentityResolution(t)

	ipcMock := ipcmock.New(t)
	cfg := configmock.New(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /connection/owner-sid", func(w http.ResponseWriter, _ *http.Request) {
		// Simulate process-agent classifying the connection as ownerless (see cmd/process-agent/api/connection_windows.go).
		w.WriteHeader(http.StatusNotFound)
	})
	ts := ipcMock.NewMockServer(mux)
	pointConfigAtMockServer(t, cfg, ts)

	configurePeerIdentityResolution(ipcMock, cfg)

	id, err := resolveConnectionOwnerSID("4", loopbackV4, 54321, loopbackV4, 5002)
	require.Error(t, err)
	// A non-2xx response must surface as a Go error, not panic, and must not leave id holding a partial
	// or garbage value.
	assert.Contains(t, err.Error(), "status code: 404")
	assert.Equal(t, peerIdentity(""), id)
}

func TestResolveConnectionOwnerSID_ProcessAgentUnreachable(t *testing.T) {
	resetPeerIdentityResolution(t)

	ipcMock := ipcmock.New(t)
	cfg := configmock.New(t)

	// Start a mock server (so we get a real, briefly-bound loopback address), then close it immediately:
	// this reliably reproduces "process-agent isn't running" (connection refused) against the exact port
	// resolveConnectionOwnerSID will dial, without racing another process for that port for longer than necessary.
	ts := ipcMock.NewMockServer(http.NewServeMux())
	pointConfigAtMockServer(t, cfg, ts)
	ts.Close()

	configurePeerIdentityResolution(ipcMock, cfg)

	id, err := resolveConnectionOwnerSID("4", loopbackV4, 54321, loopbackV4, 5002)
	require.Error(t, err)
	// Caller (resolvePeerIdentity/lookupLoopbackPeerIdentity) must see a plain error and an empty
	// identity here too, so it fails closed exactly as it would for any other resolution failure.
	assert.Equal(t, peerIdentity(""), id)
}

func TestResolveConnectionOwnerSID_TimesOut(t *testing.T) {
	resetPeerIdentityResolution(t)

	originalTimeout := resolveOwnerSIDTimeout
	resolveOwnerSIDTimeout = 50 * time.Millisecond
	t.Cleanup(func() { resolveOwnerSIDTimeout = originalTimeout })

	ipcMock := ipcmock.New(t)
	cfg := configmock.New(t)

	blockUntilTestEnds := make(chan struct{})
	t.Cleanup(func() { close(blockUntilTestEnds) })
	mux := http.NewServeMux()
	mux.HandleFunc("GET /connection/owner-sid", func(_ http.ResponseWriter, r *http.Request) {
		// A handler that never responds must not be able to block resolveConnectionOwnerSID beyond resolveOwnerSIDTimeout.
		select {
		case <-blockUntilTestEnds:
		case <-r.Context().Done():
		}
	})
	ts := ipcMock.NewMockServer(mux)
	pointConfigAtMockServer(t, cfg, ts)

	configurePeerIdentityResolution(ipcMock, cfg)

	id, err := resolveConnectionOwnerSID("4", loopbackV4, 54321, loopbackV4, 5002)
	require.Error(t, err)
	assert.Equal(t, peerIdentity(""), id)
}
