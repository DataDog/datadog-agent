// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

// Package ebpfless contains supporting code for the ebpfless tracer
package ebpfless

import (
	"encoding/binary"
	"fmt"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Record kind constants from <netinet/in_pcb.h>, used in net.inet.{tcp,udp}.pcblist_n.
// Records are emitted in groups per socket. Each TCP group is ordered as:
//
//	XSO_INPCB → XSO_SOCKET → XSO_RCVBUF → XSO_SNDBUF → XSO_STATS → XSO_TCB
//
// XSO_INPCB is always first (it identifies the socket and acts as the group
// start), and XSO_TCB is always last. The records in between are not parsed.
const (
	xsoSocket = 0x001 // XSO_SOCKET: second record in each group (not parsed, documented for reference)
	xsoInpcb  = 0x010 // XSO_INPCB: first record in each group — contains local/foreign ports
	xsoTCB    = 0x020 // XSO_TCB: last record in each TCP group — contains t_state
)

// Byte offsets within xinpcb_n (XSO_INPCB record).
//
//	xi_len(4) + xi_kind(4) + xi_inpp(8) = 16
const (
	inpcbFportOffset = 16 // inp_fport: uint16, network byte order
	inpcbLportOffset = 18 // inp_lport: uint16, network byte order
)

// tcbStateOffset is the byte offset of t_state within xtcpcb_n (XSO_TCB record).
//
// Layout of xtcpcb_n (private kernel struct, verified empirically on macOS 15,
// and cross-checked against macOS 11–15 and macOS 26 SDK headers):
//
//	xt_len(4) + xt_kind(4) + t_segq(8) + t_dupacks(4) + t_timer[4×int](16) = 36
//
// xtcpcb_n is not in the public SDK headers but its field order matches the
// xtcpcb64 struct (which is public) and has been stable since macOS 10.8.
const tcbStateOffset = 36

// TCP connection state constants from <netinet/tcp_fsm.h>.
const (
	tcpsClosed      int32 = 0
	tcpsListen      int32 = 1
	tcpsSynSent     int32 = 2
	tcpsSynReceived int32 = 3
	tcpsEstablished int32 = 4

	// tcpsMaxKnown is the highest TCP state value defined in tcp_fsm.h
	// (TCPS_TIME_WAIT = 10 in all known XNU versions). Values outside
	// [0, tcpsMaxKnown] indicate that tcbStateOffset is likely wrong.
	tcpsMaxKnown int32 = 10
)

// NewBoundPorts returns a new BoundPorts instance
func NewBoundPorts(cfg *config.Config) *BoundPorts {
	return &BoundPorts{
		config:    cfg,
		ports:     map[boundPortsKey]struct{}{},
		stop:      make(chan struct{}),
		readPorts: readPortsDarwin,
	}
}

// ephemeralPortRange returns the system's ephemeral (dynamic) port range by
// reading net.inet.ip.portrange.{first,last} from sysctl. Falls back to the
// IANA default (49152-65535) if the sysctl call fails.
func ephemeralPortRange() (first, last uint16) {
	f, err := unix.SysctlUint32("net.inet.ip.portrange.first")
	if err != nil || f == 0 {
		f = 49152
	}
	l, err := unix.SysctlUint32("net.inet.ip.portrange.last")
	if err != nil || l == 0 {
		l = 65535
	}
	return uint16(f), uint16(l)
}

// isEphemeralPort reports whether port falls in [first, last].
func isEphemeralPort(port, first, last uint16) bool {
	return port >= first && port <= last
}

// readPortsDarwin reads listening ports using sysctl PCB lists on macOS.
func readPortsDarwin(cfg *config.Config) (map[boundPortsKey]struct{}, error) {
	ports := make(map[boundPortsKey]struct{})

	// Read the OS ephemeral range for UDP filtering. network.IsPortInEphemeralRange
	// always returns EphemeralFalse on Darwin (no Linux /proc equivalent), so we
	// read the range directly from sysctl.
	ephFirst, ephLast := ephemeralPortRange()

	if cfg.CollectTCPv4Conns || cfg.CollectTCPv6Conns {
		tcpPorts, err := readTCPListeningPorts()
		if err != nil {
			log.Warnf("failed to read TCP listening ports via sysctl: %s", err)
		} else {
			for port := range tcpPorts {
				log.Debugf("adding TCP port binding: port: %d", port)
				ports[boundPortsKey{network.TCP, port}] = struct{}{}
			}
		}
	}

	if cfg.CollectUDPv4Conns || cfg.CollectUDPv6Conns {
		udpPorts, err := readUDPListeningPorts()
		if err != nil {
			log.Warnf("failed to read UDP listening ports via sysctl: %s", err)
		} else {
			for port := range udpPorts {
				// Ignore ephemeral-range ports: more likely client sockets than servers.
				if isEphemeralPort(port, ephFirst, ephLast) {
					log.Debugf("ignoring ephemeral UDP port bind to %d", port)
					continue
				}
				ports[boundPortsKey{network.UDP, port}] = struct{}{}
			}
		}
	}

	return ports, nil
}

// readTCPListeningPorts reads TCP ports in LISTEN state from the kernel via sysctl.
func readTCPListeningPorts() (map[uint16]struct{}, error) {
	buf, err := unix.SysctlRaw("net.inet.tcp.pcblist_n")
	if err != nil {
		return nil, fmt.Errorf("sysctl net.inet.tcp.pcblist_n: %w", err)
	}
	ports, err := parseTCPPcblistN(buf)
	if err != nil {
		return nil, fmt.Errorf("parsing pcblist_n: %w", err)
	}
	return ports, nil
}

// parseTCPPcblistN parses a raw net.inet.tcp.pcblist_n buffer and returns only
// the local ports whose TCP state is TCPS_LISTEN.
//
// The buffer is a flat stream of variable-length records. On macOS 11–15 (and
// macOS 26), each TCP socket group is emitted in this order:
//
//	XSO_INPCB → XSO_SOCKET → XSO_RCVBUF → XSO_SNDBUF → XSO_STATS → XSO_TCB
//
// XSO_INPCB is always the first record in a group and XSO_TCB is always last.
// All records in between are skipped by advancing pos by recLen.
//
// An error is returned if the expected XSO_TCB record is missing for a socket,
// if it is too small to contain t_state at tcbStateOffset, or if t_state is
// outside the known valid range — any of which indicate that the kernel struct
// layout has changed and the offset constant needs updating.
func parseTCPPcblistN(buf []byte) (map[uint16]struct{}, error) {
	ports := make(map[uint16]struct{})
	if len(buf) < 4 {
		return ports, nil
	}

	headerLen := int(binary.LittleEndian.Uint32(buf[:4]))
	if headerLen > len(buf) {
		return nil, fmt.Errorf("pcblist_n header length %d exceeds buffer size %d", headerLen, len(buf))
	}
	pos := headerLen

	var pendingLport uint16
	var hasPendingPort bool

	for pos+8 <= len(buf) {
		recLen := int(binary.LittleEndian.Uint32(buf[pos : pos+4]))

		// Skip alignment padding: the kernel may insert zero bytes between
		// socket groups to maintain 8-byte alignment.
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
			// XSO_INPCB is always the first record in a socket group.
			// A new XSO_INPCB while hasPendingPort is still set means the
			// previous group had no XSO_TCB, which should never happen.
			if hasPendingPort {
				return nil, fmt.Errorf("pcblist_n: XSO_INPCB for port %d has no XSO_TCB record (kernel struct layout may have changed)", pendingLport)
			}
			if recLen >= inpcbLportOffset+2 {
				lport := binary.BigEndian.Uint16(buf[pos+inpcbLportOffset : pos+inpcbLportOffset+2])
				pendingLport = lport
				hasPendingPort = lport > 0
			}

		case xsoTCB:
			// XSO_TCB is always the last record in a TCP socket group.
			if hasPendingPort {
				if recLen < tcbStateOffset+4 {
					return nil, fmt.Errorf("pcblist_n: XSO_TCB for port %d is too small (len=%d, need at least %d) — tcbStateOffset may need updating", pendingLport, recLen, tcbStateOffset+4)
				}
				tstate := int32(binary.LittleEndian.Uint32(buf[pos+tcbStateOffset : pos+tcbStateOffset+4]))
				if tstate < tcpsClosed || tstate > tcpsMaxKnown {
					return nil, fmt.Errorf("pcblist_n: XSO_TCB for port %d has unexpected t_state=%d (valid range 0–%d) — tcbStateOffset may need updating", pendingLport, tstate, tcpsMaxKnown)
				}
				if tstate == tcpsListen {
					ports[pendingLport] = struct{}{}
				}
			}
			pendingLport = 0
			hasPendingPort = false
		}

		pos += recLen
	}

	if hasPendingPort {
		return nil, fmt.Errorf("pcblist_n: XSO_INPCB for port %d has no XSO_TCB record (truncated buffer?)", pendingLport)
	}

	return ports, nil
}

// readUDPListeningPorts reads UDP bound ports from the kernel via sysctl.
func readUDPListeningPorts() (map[uint16]struct{}, error) {
	buf, err := unix.SysctlRaw("net.inet.udp.pcblist_n")
	if err != nil {
		return nil, fmt.Errorf("sysctl net.inet.udp.pcblist_n: %w", err)
	}
	return parseUDPPcblistN(buf)
}

// parseUDPPcblistN parses a raw net.inet.udp.pcblist_n buffer.
// UDP has no connection state so all bound ports (lport > 0) are returned.
// Returns an error if the buffer is truncated or invalid (e.g. header length exceeds buffer size).
func parseUDPPcblistN(buf []byte) (map[uint16]struct{}, error) {
	ports := make(map[uint16]struct{})
	if len(buf) < 4 {
		return ports, nil
	}

	headerLen := int(binary.LittleEndian.Uint32(buf[:4]))
	if headerLen > len(buf) {
		return nil, fmt.Errorf("pcblist_n header length %d exceeds buffer size %d", headerLen, len(buf))
	}
	pos := headerLen

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
		if recKind == xsoInpcb && recLen >= inpcbLportOffset+2 {
			port := binary.BigEndian.Uint16(buf[pos+inpcbLportOffset : pos+inpcbLportOffset+2])
			if port > 0 {
				ports[port] = struct{}{}
			}
		}

		pos += recLen
	}

	return ports, nil
}
