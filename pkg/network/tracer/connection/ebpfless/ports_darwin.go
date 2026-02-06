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

// pcblist_n record kinds from <netinet/in_pcb.h>
const (
	xsoInpcb = 0x010 // Internet PCB record (contains port info)
	xsoTcpcb = 0x020 // TCP control block record (contains state)

	// TCP states from <netinet/tcp_fsm.h>
	tcpsListen = 1

	// Offset of inp_lport (uint16, network byte order) within xinpcb_n struct:
	//   xi_len(4) + xi_kind(4) + xi_inpp(8) + inp_fport(2) = offset 18
	inpcbLportOffset = 18

	// Offset of t_state (int32) within xtcpcb_n struct:
	//   xt_len(4) + xt_kind(4) + t_segq(8) + t_dupacks(4) + t_timer[4](16) = offset 36
	tcpcbStateOffset = 36
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

// readPortsDarwin reads listening ports using sysctl PCB lists on macOS
func readPortsDarwin(cfg *config.Config) (map[boundPortsKey]struct{}, error) {
	ports := make(map[boundPortsKey]struct{})

	// Read TCP listening ports
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

	// Read UDP listening ports
	if cfg.CollectUDPv4Conns || cfg.CollectUDPv6Conns {
		udpPorts, err := readUDPListeningPorts()
		if err != nil {
			log.Warnf("failed to read UDP listening ports via sysctl: %s", err)
		} else {
			for port := range udpPorts {
				// ignore ephemeral port binds as they are more likely to be from
				// clients calling bind with port 0
				if network.IsPortInEphemeralRange(network.AFINET, network.UDP, port) == network.EphemeralTrue {
					log.Debugf("ignoring ephemeral UDP port bind to %d", port)
					continue
				}
				log.Debugf("adding UDP port binding: port: %d", port)
				ports[boundPortsKey{network.UDP, port}] = struct{}{}
			}
		}
	}

	return ports, nil
}

// readTCPListeningPorts reads TCP listening ports from the kernel via sysctl.
// Uses net.inet.tcp.pcblist_n which returns variable-length records for each socket.
// Records of kind XSO_INPCB contain the port, and XSO_TCPCB contains the TCP state.
func readTCPListeningPorts() (map[uint16]struct{}, error) {
	buf, err := unix.SysctlRaw("net.inet.tcp.pcblist_n")
	if err != nil {
		return nil, fmt.Errorf("sysctl net.inet.tcp.pcblist_n: %w", err)
	}

	ports := make(map[uint16]struct{})

	// Skip xinpgen header
	if len(buf) < 4 {
		return ports, nil
	}
	headerLen := binary.LittleEndian.Uint32(buf[:4])
	pos := int(headerLen)

	// Track the last local port seen from an INPCB record.
	// Within each socket group, INPCB comes before TCPCB,
	// so when we see a TCPCB with LISTEN state we use the last port.
	var lastLport uint16

	for pos+8 <= len(buf) {
		recLen := int(binary.LittleEndian.Uint32(buf[pos : pos+4]))

		// Skip alignment padding: the kernel may insert zero bytes between
		// socket groups to maintain 8-byte alignment (e.g. after xtcpcb_n
		// which is 204 bytes, not a multiple of 8)
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
			// Reset after processing a complete socket group
			lastLport = 0
		}

		pos += recLen
	}

	return ports, nil
}

// readUDPListeningPorts reads UDP bound ports from the kernel via sysctl.
// Uses net.inet.udp.pcblist_n which returns variable-length records for each socket.
// UDP has no state, so all bound ports are returned.
func readUDPListeningPorts() (map[uint16]struct{}, error) {
	buf, err := unix.SysctlRaw("net.inet.udp.pcblist_n")
	if err != nil {
		return nil, fmt.Errorf("sysctl net.inet.udp.pcblist_n: %w", err)
	}

	ports := make(map[uint16]struct{})

	// Skip xinpgen header
	if len(buf) < 4 {
		return ports, nil
	}
	headerLen := binary.LittleEndian.Uint32(buf[:4])
	pos := int(headerLen)

	for pos+8 <= len(buf) {
		recLen := int(binary.LittleEndian.Uint32(buf[pos : pos+4]))

		// Skip alignment padding (see readTCPListeningPorts for details)
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

	return ports, nil
}
