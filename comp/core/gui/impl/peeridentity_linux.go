// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package guiimpl

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/cpu"
)

// procNetTCPFiles are the kernel tables giving the UID owning a loopback TCP connection; a var so tests can point it at fixtures.
var procNetTCPFiles = []string{"/proc/net/tcp", "/proc/net/tcp6"}

// lookupLoopbackPeerIdentity finds the UID owning the loopback TCP connection by reading /proc/net/tcp{,6}, which expose the UID directly (column 8) so no PID resolution is needed.
func lookupLoopbackPeerIdentity(serverAddr net.IP, serverPort, peerPort int, peerAddr net.IP) (peerIdentity, error) {
	for _, path := range procNetTCPFiles {
		id, found, err := searchProcNetTCP(path, serverAddr, serverPort, peerPort, peerAddr)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		if found {
			return id, nil
		}
	}
	return "", fmt.Errorf("no matching TCP connection for local port %d, remote port %d", peerPort, serverPort)
}

func searchProcNetTCP(path string, serverAddr net.IP, serverPort, peerPort int, peerAddr net.IP) (peerIdentity, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Scan() // discard the header line
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		// "sl local_address rem_address st tx_queue:rx_queue tr:tm->when retrnsmt uid ..."
		if len(fields) < 8 {
			continue
		}
		localAddr, localPort, err := hexAddrPort(fields[1])
		if err != nil {
			continue
		}
		remoteAddr, remotePort, err := hexAddrPort(fields[2])
		if err != nil {
			continue
		}
		// Ports alone aren't enough: two loopback connections can share a port pair across address families, misattributing the UID.
		if localPort != peerPort || remotePort != serverPort || !localAddr.Equal(peerAddr) || !remoteAddr.Equal(serverAddr) {
			continue
		}
		uid, err := strconv.Atoi(fields[7])
		if err != nil {
			continue
		}
		return peerIdentity(strconv.Itoa(uid)), true, nil
	}
	return "", false, scanner.Err()
}

// elevatedMintIdentity stays root here: a sudo-launched xdg-open can't reach the desktop session, so mint and redeem stay consistently root.
func elevatedMintIdentity() peerIdentity {
	return rootIdentity
}

// hexAddrPort decodes a "<hex address>:<hex port>" field from /proc/net/tcp{,6}; the address is 32-bit words in host byte order, so each word is byte-swapped on little-endian hosts and left as-is on big-endian ones (else 127.0.0.1 would decode to 1.0.0.127).
func hexAddrPort(field string) (net.IP, int, error) {
	idx := strings.LastIndexByte(field, ':')
	if idx < 0 {
		return nil, 0, fmt.Errorf("malformed address field %q", field)
	}
	port, err := strconv.ParseUint(field[idx+1:], 16, 16)
	if err != nil {
		return nil, 0, err
	}
	raw, err := hex.DecodeString(field[:idx])
	if err != nil || len(raw) == 0 || len(raw)%4 != 0 {
		return nil, 0, fmt.Errorf("malformed address %q", field[:idx])
	}
	addr := make(net.IP, len(raw))
	for word := 0; word < len(raw); word += 4 {
		if cpu.IsBigEndian {
			copy(addr[word:word+4], raw[word:word+4])
		} else {
			addr[word], addr[word+1], addr[word+2], addr[word+3] = raw[word+3], raw[word+2], raw[word+1], raw[word]
		}
	}
	return addr, int(port), nil
}
