// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package ebpfless

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// ============================================================================
// Synthetic buffer helpers
// ============================================================================

// buildXinpgen builds a minimal xinpgen header/trailer record (24 bytes).
func buildXinpgen(count uint32) []byte {
	// xinpgen: xig_len(4) + xig_count(4) + xig_gen(8) + xig_sogen(8) = 24
	buf := make([]byte, 24)
	binary.LittleEndian.PutUint32(buf[0:4], 24) // xig_len
	binary.LittleEndian.PutUint32(buf[4:8], count)
	return buf
}

// buildXsoSocketRecord builds a fake XSO_SOCKET record (marks start of a socket group).
func buildXsoSocketRecord() []byte {
	const recLen = uint32(64)
	buf := make([]byte, recLen)
	binary.LittleEndian.PutUint32(buf[0:4], recLen)
	binary.LittleEndian.PutUint32(buf[4:8], xsoSocket)
	return buf
}

// buildInpcbRecord builds a fake XSO_INPCB record with the given local port
// and a zero foreign port (unconnected/listening socket).
func buildInpcbRecord(lport uint16) []byte {
	const recLen = uint32(64)
	buf := make([]byte, recLen)
	binary.LittleEndian.PutUint32(buf[0:4], recLen)
	binary.LittleEndian.PutUint32(buf[4:8], xsoInpcb)
	binary.BigEndian.PutUint16(buf[inpcbLportOffset:], lport)
	// inp_fport (inpcbFportOffset) is left as zero (no foreign connection)
	return buf
}

// buildConnectedInpcbRecord builds a fake XSO_INPCB record with both local
// and foreign ports set, simulating a connected TCP socket.
func buildConnectedInpcbRecord(lport, fport uint16) []byte {
	buf := buildInpcbRecord(lport)
	binary.BigEndian.PutUint16(buf[inpcbFportOffset:], fport)
	return buf
}

// buildXsoTCBRecord builds a fake XSO_TCB record with the given TCP state.
// The record is sized to include t_state at tcbStateOffset.
func buildXsoTCBRecord(tcpState int32) []byte {
	recLen := uint32(tcbStateOffset + 4 + 32) // extra headroom
	buf := make([]byte, recLen)
	binary.LittleEndian.PutUint32(buf[0:4], recLen)
	binary.LittleEndian.PutUint32(buf[4:8], xsoTCB)
	binary.LittleEndian.PutUint32(buf[tcbStateOffset:tcbStateOffset+4], uint32(tcpState))
	return buf
}

// buildListenGroup builds a full XSO_SOCKET+XSO_INPCB+XSO_TCB group for a
// port that is in TCPS_LISTEN state.
func buildListenGroup(lport uint16) []byte {
	var buf []byte
	buf = append(buf, buildXsoSocketRecord()...)
	buf = append(buf, buildInpcbRecord(lport)...)
	buf = append(buf, buildXsoTCBRecord(tcpsListen)...)
	return buf
}

// buildEstablishedGroup builds a full socket group for a connected socket,
// simulating an accepted connection (local port == server port, fport == remote port).
func buildEstablishedGroup(lport, fport uint16) []byte {
	var buf []byte
	buf = append(buf, buildXsoSocketRecord()...)
	buf = append(buf, buildConnectedInpcbRecord(lport, fport)...)
	buf = append(buf, buildXsoTCBRecord(tcpsEstablished)...)
	return buf
}

// buildStateGroup builds a full socket group for an arbitrary TCP state.
func buildStateGroup(lport uint16, tcpState int32) []byte {
	var buf []byte
	buf = append(buf, buildXsoSocketRecord()...)
	buf = append(buf, buildInpcbRecord(lport)...)
	buf = append(buf, buildXsoTCBRecord(tcpState)...)
	return buf
}

// ============================================================================
// Synthetic unit tests (no sysctl, no live sockets)
// ============================================================================

// TestParseTCPPcblistN_ListenVsEstablished is the core correctness test.
// It verifies that only LISTEN state sockets are returned.
func TestParseTCPPcblistN_ListenVsEstablished(t *testing.T) {
	var buf []byte
	buf = append(buf, buildXinpgen(3)...)
	// Port 80 in LISTEN state — should be included
	buf = append(buf, buildListenGroup(80)...)
	// Port 443 with ESTABLISHED state — should NOT be included even though
	// it's the server port of an accepted connection
	buf = append(buf, buildEstablishedGroup(443, 54321)...)
	// Port 8080 in LISTEN state — should be included
	buf = append(buf, buildListenGroup(8080)...)
	buf = append(buf, buildXinpgen(3)...)

	ports, err := parseTCPPcblistN(buf)
	require.NoError(t, err)

	assert.Contains(t, ports, uint16(80), "LISTEN port 80 should be included")
	assert.NotContains(t, ports, uint16(443), "ESTABLISHED port 443 should not be included")
	assert.Contains(t, ports, uint16(8080), "LISTEN port 8080 should be included")
}

// TestParseTCPPcblistN_AllTCPStates verifies that only TCPS_LISTEN (1) is
// included; all other known states are excluded without error.
func TestParseTCPPcblistN_AllTCPStates(t *testing.T) {
	states := []struct {
		state    int32
		name     string
		included bool
	}{
		{tcpsClosed, "CLOSED", false},
		{tcpsListen, "LISTEN", true},
		{tcpsSynSent, "SYN_SENT", false},
		{tcpsSynReceived, "SYN_RECEIVED", false},
		{tcpsEstablished, "ESTABLISHED", false},
		{5, "CLOSE_WAIT", false},
		{6, "FIN_WAIT_1", false},
		{9, "FIN_WAIT_2", false},
		{10, "TIME_WAIT", false},
	}

	for _, tc := range states {
		t.Run(tc.name, func(t *testing.T) {
			port := uint16(10000 + tc.state)

			var buf []byte
			buf = append(buf, buildXinpgen(1)...)
			buf = append(buf, buildStateGroup(port, tc.state)...)
			buf = append(buf, buildXinpgen(1)...)

			ports, err := parseTCPPcblistN(buf)
			require.NoError(t, err)
			if tc.included {
				assert.Contains(t, ports, port, "state %s should be included", tc.name)
			} else {
				assert.NotContains(t, ports, port, "state %s should not be included", tc.name)
			}
		})
	}
}

// TestParseTCPPcblistN_PortWithoutTCB verifies that an XSO_INPCB with no
// following XSO_TCB returns an error (the next XSO_INPCB arrives before TCB).
func TestParseTCPPcblistN_PortWithoutTCB(t *testing.T) {
	var buf []byte
	buf = append(buf, buildXinpgen(2)...)
	buf = append(buf, buildInpcbRecord(9999)...) // no XSO_TCB — next group starts
	buf = append(buf, buildListenGroup(8080)...)
	buf = append(buf, buildXinpgen(2)...)

	_, err := parseTCPPcblistN(buf)
	require.Error(t, err, "XSO_INPCB with no XSO_TCB should return an error")
	assert.Contains(t, err.Error(), "9999")
}

// TestParseTCPPcblistN_OutOfRangeTCBState verifies that a t_state outside the
// known valid range [0, tcpsMaxKnown] returns an error.
func TestParseTCPPcblistN_OutOfRangeTCBState(t *testing.T) {
	const bogusState int32 = 99

	var buf []byte
	buf = append(buf, buildXinpgen(1)...)
	buf = append(buf, buildInpcbRecord(7070)...)
	buf = append(buf, buildXsoTCBRecord(bogusState)...)
	buf = append(buf, buildXinpgen(1)...)

	_, err := parseTCPPcblistN(buf)
	require.Error(t, err, "out-of-range t_state should return an error")
	assert.Contains(t, err.Error(), "7070")
	assert.Contains(t, err.Error(), "99")
}

// TestParseTCPPcblistN_TCBTooSmall verifies that an XSO_TCB record too short
// to contain t_state at tcbStateOffset returns an error.
func TestParseTCPPcblistN_TCBTooSmall(t *testing.T) {
	smallTCB := func() []byte {
		recLen := uint32(tcbStateOffset) // deliberately one int short of tcbStateOffset+4
		buf := make([]byte, recLen)
		binary.LittleEndian.PutUint32(buf[0:4], recLen)
		binary.LittleEndian.PutUint32(buf[4:8], xsoTCB)
		return buf
	}

	var buf []byte
	buf = append(buf, buildXinpgen(1)...)
	buf = append(buf, buildInpcbRecord(5555)...)
	buf = append(buf, smallTCB()...)
	buf = append(buf, buildXinpgen(1)...)

	_, err := parseTCPPcblistN(buf)
	require.Error(t, err, "undersized XSO_TCB should return an error")
	assert.Contains(t, err.Error(), "5555")
}

// TestParseTCPPcblistN_GroupBoundaryReset verifies that encountering a second
// XSO_INPCB before the first group's XSO_TCB arrives is an error.
func TestParseTCPPcblistN_GroupBoundaryReset(t *testing.T) {
	// Group 1: XSO_INPCB(1111) with no XSO_TCB — error when group 2 starts.
	var buf []byte
	buf = append(buf, buildXinpgen(2)...)
	buf = append(buf, buildInpcbRecord(1111)...) // no TCB before next group
	buf = append(buf, buildListenGroup(2222)...)
	buf = append(buf, buildXinpgen(2)...)

	_, err := parseTCPPcblistN(buf)
	require.Error(t, err, "XSO_INPCB without preceding XSO_TCB should return an error")
	assert.Contains(t, err.Error(), "1111")
}

// TestParseTCPPcblistN_MultipleListeners verifies several LISTEN sockets all
// appear while non-LISTEN sockets in the same buffer are excluded.
func TestParseTCPPcblistN_MultipleListeners(t *testing.T) {
	listenPorts := []uint16{22, 80, 443, 8080, 9090}
	nonListenPorts := []uint16{54000, 54001, 54002}

	var buf []byte
	buf = append(buf, buildXinpgen(uint32(len(listenPorts)+len(nonListenPorts)))...)
	for _, p := range listenPorts {
		buf = append(buf, buildListenGroup(p)...)
	}
	for i, p := range nonListenPorts {
		buf = append(buf, buildEstablishedGroup(p, uint16(60000+i))...)
	}
	buf = append(buf, buildXinpgen(uint32(len(listenPorts)+len(nonListenPorts)))...)

	ports, err := parseTCPPcblistN(buf)
	require.NoError(t, err)

	for _, p := range listenPorts {
		assert.Contains(t, ports, p, "LISTEN port %d should be present", p)
	}
	for _, p := range nonListenPorts {
		assert.NotContains(t, ports, p, "ESTABLISHED port %d should not be present", p)
	}
}

func TestParseTCPPcblistN_Empty(t *testing.T) {
	var buf []byte
	buf = append(buf, buildXinpgen(0)...)
	buf = append(buf, buildXinpgen(0)...)

	ports, err := parseTCPPcblistN(buf)
	require.NoError(t, err)
	assert.Empty(t, ports)
}

func TestParseTCPPcblistN_TruncatedBuffer(t *testing.T) {
	ports, err := parseTCPPcblistN([]byte{1, 2})
	require.NoError(t, err)
	assert.Empty(t, ports)
}

func TestParseTCPPcblistN_ZeroPadding(t *testing.T) {
	// Zero-length records (alignment padding) should be skipped without stopping parsing.
	var buf []byte
	buf = append(buf, buildXinpgen(1)...)
	buf = append(buf, 0, 0, 0, 0) // 4-byte zero padding
	buf = append(buf, buildListenGroup(9090)...)
	buf = append(buf, buildXinpgen(1)...)

	ports, err := parseTCPPcblistN(buf)
	require.NoError(t, err)
	assert.Contains(t, ports, uint16(9090), "port 9090 should be found after skipping zero padding")
}

func TestParseUDPPcblistN_Synthetic(t *testing.T) {
	var buf []byte
	buf = append(buf, buildXinpgen(2)...)
	buf = append(buf, buildInpcbRecord(53)...)
	buf = append(buf, buildInpcbRecord(5353)...)
	buf = append(buf, buildXinpgen(2)...)

	ports, err := parseUDPPcblistN(buf)
	require.NoError(t, err)
	assert.Contains(t, ports, uint16(53))
	assert.Contains(t, ports, uint16(5353))
}

// TestTCBStateOffsetCalibration_WithConnection extends the calibration test
// to also verify ESTABLISHED state is correctly identified. This validates
// that offset 36 is consistent across both LISTEN and ESTABLISHED sockets.
func TestTCBStateOffsetCalibration_WithConnection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("bind not permitted in this environment:", err)
	}
	defer ln.Close()
	listenPort := uint16(ln.Addr().(*net.TCPAddr).Port)

	client, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err, "failed to connect to listener")
	defer client.Close()

	server, err := ln.Accept()
	require.NoError(t, err, "failed to accept connection")
	defer server.Close()

	clientPort := uint16(client.LocalAddr().(*net.TCPAddr).Port)
	t.Logf("listen port=%d, client port=%d", listenPort, clientPort)

	buf, err := unix.SysctlRaw("net.inet.tcp.pcblist_n")
	require.NoError(t, err)

	type portState struct {
		tstate   int32
		tcbFound bool
	}

	headerLen := int(binary.LittleEndian.Uint32(buf[:4]))
	pos := headerLen
	var lastPort uint16
	results := make(map[uint16][]portState)

	for pos+8 <= len(buf) {
		recLen := int(binary.LittleEndian.Uint32(buf[pos : pos+4]))
		if recLen == 0 {
			pos += 4
			continue
		}
		if pos+recLen > len(buf) {
			break
		}
		recKind := binary.LittleEndian.Uint32(buf[pos+4 : pos+8])

		switch recKind {
		case xsoInpcb:
			if recLen >= inpcbLportOffset+2 {
				lastPort = binary.BigEndian.Uint16(buf[pos+inpcbLportOffset : pos+inpcbLportOffset+2])
			}
		case xsoTCB:
			if lastPort == listenPort || lastPort == clientPort {
				ps := portState{tcbFound: true}
				if recLen >= tcbStateOffset+4 {
					ps.tstate = int32(binary.LittleEndian.Uint32(buf[pos+tcbStateOffset : pos+tcbStateOffset+4]))
				}
				results[lastPort] = append(results[lastPort], ps)
			}
		}
		pos += recLen
	}

	// listenPort should have at least one LISTEN entry and may have ESTABLISHED entries.
	listenStates := results[listenPort]
	require.NotEmpty(t, listenStates, "no XSO_TCB records found for listen port %d", listenPort)

	hasListen := false
	for _, s := range listenStates {
		t.Logf("listen port %d: tstate=%d", listenPort, s.tstate)
		if s.tstate == int32(tcpsListen) {
			hasListen = true
		}
	}
	assert.True(t, hasListen,
		"listen port %d should have a TCPS_LISTEN entry; found states: %v", listenPort, listenStates)

	// clientPort should only appear as ESTABLISHED.
	for _, s := range results[clientPort] {
		t.Logf("client port %d: tstate=%d", clientPort, s.tstate)
		assert.Equal(t, int32(tcpsEstablished), s.tstate,
			"client port %d should be ESTABLISHED", clientPort)
	}
}

// ============================================================================
// Live kernel tests — use real sockets with the full sysctl path
// ============================================================================

// TestReadTCPListeningPorts_Live starts a real TCP listener and verifies it
// shows up in the sysctl pcblist.
func TestReadTCPListeningPorts_Live(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("bind not permitted in this environment:", err)
	}
	defer ln.Close()

	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	t.Logf("TCP listener on port %d", port)

	ports, err := readTCPListeningPorts()
	require.NoError(t, err)

	_, found := ports[port]
	assert.True(t, found, "expected TCP port %d to be in listening ports: %v", port, ports)
}

// TestReadUDPListeningPorts_Live starts a real UDP listener and verifies it
// shows up in the sysctl pcblist.
func TestReadUDPListeningPorts_Live(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skip("bind not permitted in this environment:", err)
	}
	defer conn.Close()

	port := uint16(conn.LocalAddr().(*net.UDPAddr).Port)
	t.Logf("UDP listener on port %d", port)

	ports, err := readUDPListeningPorts()
	require.NoError(t, err)

	_, found := ports[port]
	assert.True(t, found, "expected UDP port %d to be in listening ports: %v", port, ports)
}

// TestReadTCPListeningPorts_EstablishedNotPresent verifies two key properties:
//  1. The server's LISTEN socket IS in the results.
//  2. The accepted server-side connection socket (same local port, ESTABLISHED)
//     does NOT result in a duplicate — there is exactly the one LISTEN entry.
//  3. The client's ephemeral source port is NOT in the results.
func TestReadTCPListeningPorts_EstablishedNotPresent(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("bind not permitted in this environment:", err)
	}
	defer ln.Close()

	listenPort := uint16(ln.Addr().(*net.TCPAddr).Port)
	t.Logf("TCP listener on port %d", listenPort)

	client, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err, "failed to connect to listener")
	defer client.Close()

	server, err := ln.Accept()
	require.NoError(t, err, "failed to accept connection")
	defer server.Close()

	clientPort := uint16(client.LocalAddr().(*net.TCPAddr).Port)
	t.Logf("client ephemeral port %d", clientPort)

	ports, err := readTCPListeningPorts()
	require.NoError(t, err)

	_, foundListen := ports[listenPort]
	assert.True(t, foundListen,
		"the original LISTEN socket on port %d must be present", listenPort)

	_, foundClient := ports[clientPort]
	assert.False(t, foundClient,
		"client ephemeral port %d must NOT appear in listening ports", clientPort)
}

// TestReadPortsDarwin_EphemeralTCPListenerIncluded verifies that a TCP socket
// listening on an ephemeral port IS included by readPortsDarwin. TCP ports are
// not filtered by the ephemeral range because the XSO_TCB LISTEN-state check
// already guarantees the socket is a genuine server socket.
func TestReadPortsDarwin_EphemeralTCPListenerIncluded(t *testing.T) {
	ephFirst, ephLast := ephemeralPortRange()
	t.Logf("system ephemeral port range: %d-%d", ephFirst, ephLast)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("bind not permitted in this environment:", err)
	}
	defer ln.Close()

	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	t.Logf("listener on port %d (ephemeral=%v)", port, isEphemeralPort(port, ephFirst, ephLast))

	if !isEphemeralPort(port, ephFirst, ephLast) {
		t.Skipf("OS assigned non-ephemeral port %d; cannot test ephemeral TCP inclusion", port)
	}

	cfg := config.New()
	cfg.CollectTCPv4Conns = true
	cfg.CollectTCPv6Conns = false
	cfg.CollectUDPv4Conns = false
	cfg.CollectUDPv6Conns = false

	ports, err := readPortsDarwin(cfg)
	require.NoError(t, err)
	_, found := ports[boundPortsKey{network.TCP, port}]
	assert.True(t, found, "readPortsDarwin should include ephemeral TCP listener on port %d", port)
}

// TestReadTCPListeningPorts_ClosedNotPresent ensures a closed listener's port
// does not appear.
func TestReadTCPListeningPorts_ClosedNotPresent(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("bind not permitted in this environment:", err)
	}

	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	ln.Close()

	ports, err := readTCPListeningPorts()
	require.NoError(t, err)

	_, found := ports[port]
	assert.False(t, found, "closed TCP port %d should not be in listening ports", port)
}

// TestReadTCPListeningPorts_MultiplePorts verifies multiple TCP listeners are
// all found.
func TestReadTCPListeningPorts_MultiplePorts(t *testing.T) {
	var listeners []net.Listener
	var expectedPorts []uint16

	for i := 0; i < 3; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Skip("bind not permitted in this environment:", err)
		}
		listeners = append(listeners, ln)
		expectedPorts = append(expectedPorts, uint16(ln.Addr().(*net.TCPAddr).Port))
	}
	defer func() {
		for _, ln := range listeners {
			ln.Close()
		}
	}()

	t.Logf("expecting TCP ports: %v", expectedPorts)

	var ports map[uint16]struct{}
	var missing []uint16
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(50 * time.Millisecond)
		}
		var err error
		ports, err = readTCPListeningPorts()
		require.NoError(t, err)

		missing = nil
		for _, p := range expectedPorts {
			if _, found := ports[p]; !found {
				missing = append(missing, p)
			}
		}
		if len(missing) == 0 {
			return
		}
	}

	t.Errorf("after retries, still missing TCP ports %v (found %d total ports)", missing, len(ports))
}

// TestReadTCPListeningPorts_IPv6 verifies IPv6 TCP listeners are also found.
func TestReadTCPListeningPorts_IPv6(t *testing.T) {
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 not available:", err)
	}
	defer ln.Close()

	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	t.Logf("TCP6 listener on port %d", port)

	ports, err := readTCPListeningPorts()
	require.NoError(t, err)

	_, found := ports[port]
	assert.True(t, found, "expected TCP6 port %d to be in listening ports: %v", port, ports)
}

// TestReadTCPListeningPorts_SpecificPort uses a specific port to verify
// the port number parsing is correct (catches byte-order issues).
func TestReadTCPListeningPorts_SpecificPort(t *testing.T) {
	// Use a port where the two bytes differ to catch endianness bugs.
	// Port 0x1234 = 4660.
	candidates := []int{4660, 5170, 6180}
	var ln net.Listener
	var port int
	for _, p := range candidates {
		var err error
		ln, err = net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
		if err == nil {
			port = p
			break
		}
	}
	if ln == nil {
		t.Skip("could not bind to any candidate port")
	}
	defer ln.Close()

	t.Logf("TCP listener on specific port %d (0x%04x)", port, port)

	ports, err := readTCPListeningPorts()
	require.NoError(t, err)

	_, found := ports[uint16(port)]
	assert.True(t, found, "expected TCP port %d to be in listening ports", port)
}

// ============================================================================
// BoundPorts integration test
// ============================================================================

// TestBoundPorts_Find verifies the full BoundPorts.Find() API correctly reports
// a listening TCP port as bound, and excludes non-listening ports and wrong protocols.
func TestBoundPorts_Find(t *testing.T) {
	// Use a specific low-numbered port to stay outside the ephemeral range.
	candidates := []int{7777, 7778, 7779, 7780}
	var ln net.Listener
	var listenPort uint16

	for _, p := range candidates {
		var err error
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			listenPort = uint16(p)
			break
		}
	}
	if ln == nil {
		t.Skip("could not bind to any test port")
	}
	defer ln.Close()

	cfg := config.New()
	cfg.CollectTCPv4Conns = true
	cfg.CollectTCPv6Conns = false
	cfg.CollectUDPv4Conns = false
	cfg.CollectUDPv6Conns = false

	bp := NewBoundPorts(cfg)
	err := bp.Start()
	require.NoError(t, err)
	defer bp.Stop()

	assert.True(t, bp.Find(network.TCP, listenPort),
		"BoundPorts.Find should return true for listening port %d", listenPort)
	assert.False(t, bp.Find(network.TCP, 1),
		"BoundPorts.Find should return false for an unused port")
	assert.False(t, bp.Find(network.UDP, listenPort),
		"BoundPorts.Find should return false for the wrong protocol")
}
