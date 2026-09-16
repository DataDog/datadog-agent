// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package guiimpl

import (
	"encoding/binary"
	"math/bits"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/cpu"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

var (
	loopbackV4 = net.ParseIP("127.0.0.1")
	loopbackV6 = net.ParseIP("::1")
)

// testProcessAgentSID is an arbitrary well-formed SID a mock process-agent hands back for any pid
// asked about by setupPeerIdentityResolutionForTest below; the real pid queried is always this test
// binary's own (both ends of the loopback connection run in the same process), but the test only needs
// to know that a real HTTP round trip through sidForPID resolved to a non-empty identity.
const testProcessAgentSID = "S-1-5-21-3623811015-3361044348-30300820-1013"

func init() {
	// On Windows, sidForPID requires a configured process-agent IPC client (see peeridentity_windows.go);
	// without this, Test_intentToken_peerIdentity's "same OS identity" subtest would always resolve to an
	// empty identity, since minting always goes through a real loopback connection.
	setupPeerIdentityResolutionForTest = func(t *testing.T) {
		resetPeerIdentityResolution(t)

		ipcMock := ipcmock.New(t)
		cfg := configmock.New(t)

		mux := http.NewServeMux()
		mux.HandleFunc("GET /pid/{pid}/sid", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte(testProcessAgentSID))
		})
		ts := ipcMock.NewMockServer(mux)
		pointConfigAtMockServer(t, cfg, ts)

		configurePeerIdentityResolution(ipcMock, cfg)
	}
}

// portToField is the inverse of portFromField, used to build fixtures.
func portToField(port uint16) uint32 {
	if cpu.IsBigEndian {
		return uint32(port) << 16
	}
	return bits.ReverseBytes32(uint32(port) << 16)
}

// addrToV4Field is the inverse of addrFromV4Field, used to build fixtures.
func addrToV4Field(ip net.IP) uint32 {
	v4 := ip.To4()
	if cpu.IsBigEndian {
		return binary.BigEndian.Uint32(v4)
	}
	return binary.LittleEndian.Uint32(v4)
}

func v4Row(localPort, remotePort uint16, localAddr, remoteAddr net.IP, pid uint32) mibTCPRowOwnerPID {
	return mibTCPRowOwnerPID{
		localAddr:  addrToV4Field(localAddr),
		localPort:  portToField(localPort),
		remoteAddr: addrToV4Field(remoteAddr),
		remotePort: portToField(remotePort),
		pid:        pid,
	}
}

func v6Row(localPort, remotePort uint16, localAddr, remoteAddr net.IP, pid uint32) mibTCP6RowOwnerPID {
	var row mibTCP6RowOwnerPID
	copy(row.localAddr[:], localAddr.To16())
	row.localPort = portToField(localPort)
	copy(row.remoteAddr[:], remoteAddr.To16())
	row.remotePort = portToField(remotePort)
	row.pid = pid
	return row
}

// buildV4Table lays out rows exactly as GetExtendedTcpTable(AF_INET) would: a 4-byte entry count followed by that many mibTCPRowOwnerPID records.
func buildV4Table(rows []mibTCPRowOwnerPID) []byte {
	size := int(unsafe.Sizeof(mibTCPTableOwnerPID{}))
	if extra := len(rows) - 1; extra > 0 {
		size += extra * int(unsafe.Sizeof(mibTCPRowOwnerPID{}))
	}
	buf := make([]byte, size)
	info := (*mibTCPTableOwnerPID)(unsafe.Pointer(&buf[0]))
	info.numEntries = uint32(len(rows))
	if len(rows) > 0 {
		copy(unsafe.Slice(&info.table[0], len(rows)), rows)
	}
	return buf
}

// buildV6Table is buildV4Table's IPv6 counterpart.
func buildV6Table(rows []mibTCP6RowOwnerPID) []byte {
	size := int(unsafe.Sizeof(mibTCP6TableOwnerPID{}))
	if extra := len(rows) - 1; extra > 0 {
		size += extra * int(unsafe.Sizeof(mibTCP6RowOwnerPID{}))
	}
	buf := make([]byte, size)
	info := (*mibTCP6TableOwnerPID)(unsafe.Pointer(&buf[0]))
	info.numEntries = uint32(len(rows))
	if len(rows) > 0 {
		copy(unsafe.Slice(&info.table[0], len(rows)), rows)
	}
	return buf
}

func TestPortFromField(t *testing.T) {
	for _, port := range []uint16{1, 80, 8080, 65535} {
		assert.Equal(t, port, portFromField(portToField(port)))
	}
}

func TestAddrFromV4Field(t *testing.T) {
	assert.True(t, loopbackV4.Equal(addrFromV4Field(addrToV4Field(loopbackV4))))
}

func TestAddrFromV4Field_LiteralOracle(t *testing.T) {
	// Unlike the subtest above, this literal is an independent oracle, not produced by addrToV4Field, so a symmetric bug shared by both can't hide a real decoding bug; 127.0.0.1 is 0x0100007F little-endian, matching both of Windows's supported architectures (amd64, arm64).
	if cpu.IsBigEndian {
		t.Skip("this literal is little-endian-specific; Windows has no supported big-endian architecture")
	}
	assert.True(t, loopbackV4.Equal(addrFromV4Field(0x0100007F)))
}

func TestFindPIDInV4Table(t *testing.T) {
	t.Run("matches the row with the right ports and addresses", func(t *testing.T) {
		buf := buildV4Table([]mibTCPRowOwnerPID{
			v4Row(1234, 443, loopbackV4, loopbackV4, 111),
			v4Row(8080, 80, loopbackV4, loopbackV4, 222),
		})

		pid, ok := findPIDInV4Table(buf, 8080, 80, loopbackV4, loopbackV4)
		require.True(t, ok)
		assert.Equal(t, uint32(222), pid)
	})

	t.Run("no matching connection", func(t *testing.T) {
		buf := buildV4Table([]mibTCPRowOwnerPID{v4Row(1234, 443, loopbackV4, loopbackV4, 111)})

		_, ok := findPIDInV4Table(buf, 8080, 80, loopbackV4, loopbackV4)
		assert.False(t, ok)
	})

	t.Run("an empty buffer does not match, and does not panic", func(t *testing.T) {
		_, ok := findPIDInV4Table(nil, 8080, 80, loopbackV4, loopbackV4)
		assert.False(t, ok)
	})

	t.Run("does not confuse connections that share a port pair across distinct server addresses", func(t *testing.T) {
		// Regression test: matching on ports and client address alone isn't enough; only the row actually made to remoteAddr (127.0.0.1 vs .2) must match.
		loopbackV4Alt := net.ParseIP("127.0.0.2")
		buf := buildV4Table([]mibTCPRowOwnerPID{
			v4Row(8080, 80, loopbackV4, loopbackV4, 4000),
			v4Row(8080, 80, loopbackV4, loopbackV4Alt, 5000),
		})

		pid, ok := findPIDInV4Table(buf, 8080, 80, loopbackV4, loopbackV4)
		require.True(t, ok)
		assert.Equal(t, uint32(4000), pid)

		pid, ok = findPIDInV4Table(buf, 8080, 80, loopbackV4, loopbackV4Alt)
		require.True(t, ok)
		assert.Equal(t, uint32(5000), pid)
	})
}

func TestFindPIDInV6Table(t *testing.T) {
	t.Run("matches the row with the right ports and addresses", func(t *testing.T) {
		buf := buildV6Table([]mibTCP6RowOwnerPID{
			v6Row(1234, 443, loopbackV6, loopbackV6, 111),
			v6Row(8080, 80, loopbackV6, loopbackV6, 222),
		})

		pid, ok := findPIDInV6Table(buf, 8080, 80, loopbackV6, loopbackV6)
		require.True(t, ok)
		assert.Equal(t, uint32(222), pid)
	})

	t.Run("no matching connection", func(t *testing.T) {
		buf := buildV6Table([]mibTCP6RowOwnerPID{v6Row(1234, 443, loopbackV6, loopbackV6, 111)})

		_, ok := findPIDInV6Table(buf, 8080, 80, loopbackV6, loopbackV6)
		assert.False(t, ok)
	})

	t.Run("an empty buffer does not match, and does not panic", func(t *testing.T) {
		_, ok := findPIDInV6Table(nil, 8080, 80, loopbackV6, loopbackV6)
		assert.False(t, ok)
	})
}

func TestFindPID_DoesNotConfuseAddressFamilies(t *testing.T) {
	// Regression test: each finder must reject rows whose address family doesn't match, even though lookupLoopbackPeerIdentity already routes to the table matching peerAddr's own family in practice.
	v4Buf := buildV4Table([]mibTCPRowOwnerPID{v4Row(8080, 80, loopbackV4, loopbackV4, 4000)})
	v6Buf := buildV6Table([]mibTCP6RowOwnerPID{v6Row(8080, 80, loopbackV6, loopbackV6, 6000)})

	_, ok := findPIDInV4Table(v4Buf, 8080, 80, loopbackV6, loopbackV4)
	assert.False(t, ok, "must not match a v4 row against a v6 local address")

	_, ok = findPIDInV6Table(v6Buf, 8080, 80, loopbackV4, loopbackV6)
	assert.False(t, ok, "must not match a v6 row against a v4 local address")
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

// pointConfigAtMockServer sets the keys sidForPID actually reads (cmd_host, process_config.cmd_port) on
// cfg from ts's real address. cfg and ipcMock's internal config are the same underlying instance
// (pkg/config/mock.New is a singleton within a test), so ipcMock.NewMockServer's cmd_host write already
// lands on cfg; the one gap this closes is the port key: NewMockServer sets plain cmd_port, but
// GetProcessAPIAddressPort reads process_config.cmd_port, so without this, sidForPID always dials the
// default port (6162) rather than the mock server's actual ephemeral one.
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

func TestSidForPID_QueriesProcessAgent(t *testing.T) {
	resetPeerIdentityResolution(t)

	ipcMock := ipcmock.New(t)
	cfg := configmock.New(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /pid/{pid}/sid", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "4242", r.PathValue("pid"))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("S-1-5-21-3623811015-3361044348-30300820-1013\n"))
	})
	ts := ipcMock.NewMockServer(mux)
	pointConfigAtMockServer(t, cfg, ts)

	configurePeerIdentityResolution(ipcMock, cfg)

	id, err := sidForPID(4242)
	require.NoError(t, err)
	// The handler's trailing newline (defensive on the caller's part; the real handler never adds one)
	// must be trimmed, not folded into the returned identity.
	assert.Equal(t, peerIdentity("S-1-5-21-3623811015-3361044348-30300820-1013"), id)
}

func TestSidForPID_NotConfigured(t *testing.T) {
	resetPeerIdentityResolution(t)
	// No configurePeerIdentityResolution call: sidForPID must fail closed (empty identity, non-nil error)
	// rather than panic on the nil processAgentIPC/processAgentConfig.
	configurePeerIdentityResolution(nil, nil)

	id, err := sidForPID(1234)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
	assert.Equal(t, peerIdentity(""), id)
}

func TestSidForPID_ErrorFromProcessAgent(t *testing.T) {
	resetPeerIdentityResolution(t)

	ipcMock := ipcmock.New(t)
	cfg := configmock.New(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /pid/{pid}/sid", func(w http.ResponseWriter, _ *http.Request) {
		// Simulate process-agent classifying the pid as not found (see cmd/process-agent/api/pid_windows.go).
		w.WriteHeader(http.StatusNotFound)
	})
	ts := ipcMock.NewMockServer(mux)
	pointConfigAtMockServer(t, cfg, ts)

	configurePeerIdentityResolution(ipcMock, cfg)

	id, err := sidForPID(4242)
	require.Error(t, err)
	// A non-2xx response must surface as a Go error, not panic, and must not leave id holding a partial
	// or garbage value.
	assert.Contains(t, err.Error(), "status code: 404")
	assert.Equal(t, peerIdentity(""), id)
}

func TestSidForPID_ProcessAgentUnreachable(t *testing.T) {
	resetPeerIdentityResolution(t)

	ipcMock := ipcmock.New(t)
	cfg := configmock.New(t)

	// Start a mock server (so we get a real, briefly-bound loopback address), then close it immediately:
	// this reliably reproduces "process-agent isn't running" (connection refused) against the exact port
	// sidForPID will dial, without racing another process for that port for longer than necessary.
	ts := ipcMock.NewMockServer(http.NewServeMux())
	pointConfigAtMockServer(t, cfg, ts)
	ts.Close()

	configurePeerIdentityResolution(ipcMock, cfg)

	id, err := sidForPID(4242)
	require.Error(t, err)
	// Caller (resolvePeerIdentity/lookupLoopbackPeerIdentity) must see a plain error and an empty
	// identity here too, so it fails closed exactly as it would for any other resolution failure.
	assert.Equal(t, peerIdentity(""), id)
}

func TestSidForPID_TimesOut(t *testing.T) {
	resetPeerIdentityResolution(t)

	originalTimeout := sidForPIDTimeout
	sidForPIDTimeout = 50 * time.Millisecond
	t.Cleanup(func() { sidForPIDTimeout = originalTimeout })

	ipcMock := ipcmock.New(t)
	cfg := configmock.New(t)

	blockUntilTestEnds := make(chan struct{})
	t.Cleanup(func() { close(blockUntilTestEnds) })
	mux := http.NewServeMux()
	mux.HandleFunc("GET /pid/{pid}/sid", func(_ http.ResponseWriter, r *http.Request) {
		// A handler that never responds must not be able to block sidForPID beyond sidForPIDTimeout.
		select {
		case <-blockUntilTestEnds:
		case <-r.Context().Done():
		}
	})
	ts := ipcMock.NewMockServer(mux)
	pointConfigAtMockServer(t, cfg, ts)

	configurePeerIdentityResolution(ipcMock, cfg)

	id, err := sidForPID(4242)
	require.Error(t, err)
	assert.Equal(t, peerIdentity(""), id)
}
