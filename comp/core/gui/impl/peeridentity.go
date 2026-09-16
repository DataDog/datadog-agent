// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package guiimpl

import (
	"errors"
	"fmt"
	"net"
	"strconv"
)

// peerIdentity identifies the OS user (UID on Unix, SID on Windows) owning one end of a loopback TCP connection; empty means unconstrained (matches anything).
type peerIdentity string

// rootIdentity is Unix root (UID 0); Windows SIDs are never a plain "0", so this constant is safe to compare against on every platform.
const rootIdentity peerIdentity = "0"

// mintTimeIdentity re-resolves a root mint-time identity per-platform (see elevatedMintIdentity), since sudo/elevated launches are sometimes redeemed by a different, unelevated identity.
func mintTimeIdentity(resolved peerIdentity) peerIdentity {
	if resolved == rootIdentity {
		return elevatedMintIdentity()
	}
	return resolved
}

// resolvePeerIdentity returns the OS identity of the peer on the other end of the loopback connection identified by serverAddr/remoteAddr, both derived by net/http from the accepted socket and so never client-spoofable.
func resolvePeerIdentity(serverAddr net.Addr, remoteAddr string) (peerIdentity, error) {
	if serverAddr == nil {
		return "", errors.New("server address unavailable")
	}
	// A Unix domain socket serverAddr won't parse as host:port; that falls back to the pre-existing TTL/single-use protection too.
	serverHost, serverPortStr, err := net.SplitHostPort(serverAddr.String())
	if err != nil {
		return "", fmt.Errorf("malformed server address %q: %w", serverAddr, err)
	}
	serverIP := net.ParseIP(serverHost)
	if serverIP == nil {
		return "", fmt.Errorf("server address %q has no parseable IP", serverAddr)
	}
	serverPort, err := strconv.Atoi(serverPortStr)
	if err != nil {
		return "", fmt.Errorf("malformed server port %q: %w", serverPortStr, err)
	}

	// A Unix domain socket or vsock (e.g. "host(2):1234") remoteAddr is rejected the same way, falling back to the pre-existing TTL/single-use protection.
	host, portStr, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return "", fmt.Errorf("malformed remote address %q: %w", remoteAddr, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("peer %q is not a loopback address", host)
	}
	peerPort, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("malformed remote port %q: %w", portStr, err)
	}

	return lookupLoopbackPeerIdentity(serverIP, serverPort, peerPort, ip)
}
