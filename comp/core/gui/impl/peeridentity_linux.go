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
)

// procNetTCPFiles are the kernel-exposed tables consulted to resolve the OS UID owning a loopback TCP connection; a var so tests can point it at fixture files instead of the real /proc.
var procNetTCPFiles = []string{"/proc/net/tcp", "/proc/net/tcp6"}

// lookupLoopbackPeerIdentity finds the UID owning the loopback TCP connection by reading /proc/net/tcp{,6}, which expose the owning UID directly (column 8) and are world-readable, so no PID resolution step is needed.
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
		// Matching on ports alone isn't enough: two loopback connections can share a port pair across address families (e.g. 127.0.0.1 vs ::1), misattributing an unrelated connection's UID.
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

// hexAddrPort decodes a "<hex address>:<hex port>" field from /proc/net/tcp{,6}, whose address is one (IPv4) or four (IPv6) 32-bit words, each byte-swapped to native order (e.g. loopback 127.0.0.1 is "0100007F").
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
		addr[word], addr[word+1], addr[word+2], addr[word+3] = raw[word+3], raw[word+2], raw[word+1], raw[word]
	}
	return addr, int(port), nil
}
