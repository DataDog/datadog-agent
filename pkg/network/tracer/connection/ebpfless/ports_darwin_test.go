// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package ebpfless

import (
	"encoding/binary"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadTCPListeningPorts_Live starts a real TCP listener and verifies it
// shows up in the sysctl pcblist.
func TestReadTCPListeningPorts_Live(t *testing.T) {
	// Listen on a random available port
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
	// Listen on a random available port
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skip("bind not permitted in this environment:", err)
	}
	defer conn.Close()

	portStr := conn.LocalAddr().(*net.UDPAddr).Port
	port := uint16(portStr)
	t.Logf("UDP listener on port %d", port)

	ports, err := readUDPListeningPorts()
	require.NoError(t, err)

	_, found := ports[port]
	assert.True(t, found, "expected UDP port %d to be in listening ports: %v", port, ports)
}

// TestReadTCPListeningPorts_ClosedNotPresent ensures a closed listener's port
// does not appear.
func TestReadTCPListeningPorts_ClosedNotPresent(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("bind not permitted in this environment:", err)
	}

	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	ln.Close() // close before reading

	ports, err := readTCPListeningPorts()
	require.NoError(t, err)

	_, found := ports[port]
	assert.False(t, found, "closed TCP port %d should not be in listening ports", port)
}

// TestReadTCPListeningPorts_MultiplePorts verifies multiple TCP listeners are
// all found. Uses a small retry to handle kernel PCB list propagation timing.
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

	// Retry a few times; the kernel PCB list snapshot may lag slightly
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
	// Use a port where the two bytes differ, to catch endianness bugs.
	// Port 0x1234 = 4660; try a few in case they're in use.
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

// --- Synthetic binary parsing tests ---
// These construct fake pcblist_n buffers to verify the parser handles
// the binary format correctly, independent of the live kernel.

// buildXinpgen builds a minimal xinpgen header/trailer.
func buildXinpgen(count uint32) []byte {
	// xinpgen: xig_len(4) + xig_count(4) + xig_gen(8) + xig_sogen(8) = 24
	buf := make([]byte, 24)
	binary.LittleEndian.PutUint32(buf[0:4], 24) // xig_len
	binary.LittleEndian.PutUint32(buf[4:8], count)
	return buf
}

// buildInpcbRecord builds a fake XSO_INPCB record with the given local port.
func buildInpcbRecord(lport uint16) []byte {
	recLen := uint32(64) // plenty of room
	buf := make([]byte, recLen)
	binary.LittleEndian.PutUint32(buf[0:4], recLen)           // xi_len
	binary.LittleEndian.PutUint32(buf[4:8], xsoInpcb)         // xi_kind
	binary.BigEndian.PutUint16(buf[inpcbLportOffset:], lport) // inp_lport (network byte order)
	return buf
}

// buildTcpcbRecord builds a fake XSO_TCPCB record with the given state.
func buildTcpcbRecord(state int32) []byte {
	recLen := uint32(128) // plenty of room
	buf := make([]byte, recLen)
	binary.LittleEndian.PutUint32(buf[0:4], recLen)                      // xt_len
	binary.LittleEndian.PutUint32(buf[4:8], xsoTcpcb)                    // xt_kind
	binary.LittleEndian.PutUint32(buf[tcpcbStateOffset:], uint32(state)) // t_state
	return buf
}

func TestParseTCPListeningPorts_Synthetic(t *testing.T) {
	// Build a fake pcblist_n with:
	// - header
	// - socket 1: INPCB(port 80) + TCPCB(LISTEN)  -> should be found
	// - socket 2: INPCB(port 443) + TCPCB(state=4/ESTABLISHED) -> should NOT be found
	// - socket 3: INPCB(port 8080) + TCPCB(LISTEN) -> should be found
	// - trailer
	var buf []byte
	buf = append(buf, buildXinpgen(3)...)
	buf = append(buf, buildInpcbRecord(80)...)
	buf = append(buf, buildTcpcbRecord(tcpsListen)...)
	buf = append(buf, buildInpcbRecord(443)...)
	buf = append(buf, buildTcpcbRecord(4)...) // ESTABLISHED
	buf = append(buf, buildInpcbRecord(8080)...)
	buf = append(buf, buildTcpcbRecord(tcpsListen)...)
	buf = append(buf, buildXinpgen(3)...) // trailer

	// Parse using the same logic as readTCPListeningPorts but on our buffer
	ports := parseTCPPcblistN(buf)

	assert.Contains(t, ports, uint16(80), "port 80 should be found (LISTEN)")
	assert.NotContains(t, ports, uint16(443), "port 443 should not be found (ESTABLISHED)")
	assert.Contains(t, ports, uint16(8080), "port 8080 should be found (LISTEN)")
}

func TestParseUDPListeningPorts_Synthetic(t *testing.T) {
	// Build a fake pcblist_n with:
	// - header
	// - INPCB(port 53)
	// - INPCB(port 5353)
	// - trailer
	var buf []byte
	buf = append(buf, buildXinpgen(2)...)
	buf = append(buf, buildInpcbRecord(53)...)
	buf = append(buf, buildInpcbRecord(5353)...)
	buf = append(buf, buildXinpgen(2)...) // trailer

	ports := parseUDPPcblistN(buf)

	assert.Contains(t, ports, uint16(53))
	assert.Contains(t, ports, uint16(5353))
}

func TestParseTCPPcblistN_Empty(t *testing.T) {
	// Just a header and trailer, no sockets
	var buf []byte
	buf = append(buf, buildXinpgen(0)...)
	buf = append(buf, buildXinpgen(0)...)

	ports := parseTCPPcblistN(buf)
	assert.Empty(t, ports)
}

func TestParseTCPPcblistN_TruncatedBuffer(t *testing.T) {
	// Buffer too short to contain even a header
	ports := parseTCPPcblistN([]byte{1, 2})
	assert.Empty(t, ports)
}

func TestParseTCPPcblistN_ZeroPadding(t *testing.T) {
	// Zero-length records (alignment padding) should be skipped, not stop parsing
	var buf []byte
	buf = append(buf, buildXinpgen(1)...)
	// 4 bytes of zero padding (like kernel alignment after xtcpcb_n)
	buf = append(buf, 0, 0, 0, 0)
	// Then a real INPCB + TCPCB
	buf = append(buf, buildInpcbRecord(9090)...)
	buf = append(buf, buildTcpcbRecord(tcpsListen)...)
	buf = append(buf, buildXinpgen(1)...) // trailer

	ports := parseTCPPcblistN(buf)
	assert.Contains(t, ports, uint16(9090), "port 9090 should be found after skipping zero padding")
}

// --- Helper parsing functions that operate on raw buffers ---
// These mirror the logic in readTCPListeningPorts / readUDPListeningPorts
// but accept a byte slice directly (no sysctl call).

func parseTCPPcblistN(buf []byte) map[uint16]struct{} {
	ports := make(map[uint16]struct{})
	if len(buf) < 4 {
		return ports
	}
	headerLen := binary.LittleEndian.Uint32(buf[:4])
	pos := int(headerLen)

	var lastLport uint16
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
			if pos+inpcbLportOffset+2 <= len(buf) {
				lastLport = binary.BigEndian.Uint16(buf[pos+inpcbLportOffset : pos+inpcbLportOffset+2])
			}
		case xsoTcpcb:
			if pos+tcpcbStateOffset+4 <= len(buf) {
				state := int32(binary.LittleEndian.Uint32(buf[pos+tcpcbStateOffset : pos+tcpcbStateOffset+4]))
				if state == tcpsListen && lastLport > 0 {
					ports[lastLport] = struct{}{}
				}
			}
			lastLport = 0
		}
		pos += recLen
	}
	return ports
}

func parseUDPPcblistN(buf []byte) map[uint16]struct{} {
	ports := make(map[uint16]struct{})
	if len(buf) < 4 {
		return ports
	}
	headerLen := binary.LittleEndian.Uint32(buf[:4])
	pos := int(headerLen)

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
		if recKind == xsoInpcb && pos+inpcbLportOffset+2 <= len(buf) {
			port := binary.BigEndian.Uint16(buf[pos+inpcbLportOffset : pos+inpcbLportOffset+2])
			if port > 0 {
				ports[port] = struct{}{}
			}
		}
		pos += recLen
	}
	return ports
}
