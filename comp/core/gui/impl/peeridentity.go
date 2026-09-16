// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package guiimpl

import (
	"fmt"
	"net"
	"strconv"
)

// peerIdentity identifies the OS user (UID on Unix, SID on Windows) that
// owns one end of a loopback TCP connection. The zero value means
// "unconstrained": either the platform doesn't implement peer identity
// resolution, or the connection wasn't a resolvable loopback TCP connection.
// Callers treat an unconstrained identity as matching anything, which
// preserves the pre-existing TTL-only protection wherever peer identity
// can't be established, instead of breaking the feature outright.
type peerIdentity string

// resolvePeerIdentity returns the identity of the process holding the local
// end of the loopback TCP connection whose remote address (as observed by
// our own server) is remoteAddr, and whose local address is serverAddr: the
// specific address of the connection our server actually accepted, not just
// the port it's configured to listen on. Checking the full address, not just
// the port, matters because both the GUI and CMD API servers can be
// configured to bind to any local address, and other loopback addresses
// (e.g. 127.0.0.2) can host unrelated servers on the same port number.
//
// remoteAddr must come from http.Request.RemoteAddr, and serverAddr from
// http.Request.Context().Value(http.LocalAddrContextKey): net/http always
// derives both from the accepted socket itself, never from a
// client-controlled header, so neither can be spoofed by the request itself.
func resolvePeerIdentity(serverAddr net.Addr, remoteAddr string) (peerIdentity, error) {
	if serverAddr == nil {
		return "", fmt.Errorf("server address unavailable")
	}
	// serverAddr won't parse as host:port if our own server is instead
	// listening on a Unix domain socket; that's also handled here, by simply
	// falling back to the pre-existing TTL/single-use protection.
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

	// remoteAddr won't parse as host:port if the CMD API server is instead
	// listening on a Unix domain socket. A vsock address (e.g. "host(2):1234")
	// does parse as host:port, but is rejected just below since its host
	// portion isn't a valid IP; either way, resolution falls back to the
	// pre-existing TTL/single-use protection.
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
