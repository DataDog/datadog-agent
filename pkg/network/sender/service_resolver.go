// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sender

import (
	"net/netip"
)

// listenKey identifies a listening socket by its bind IP and port. Two distinct
// processes can listen on the same port if they bind to different interfaces
// (e.g. one on 127.0.0.1, another on a LAN IP), so a port number alone is not
// a unique key.
type listenKey struct {
	IP   netip.Addr
	Port int32
}

// Wildcard bind addresses. A listener bound here accepts on all interfaces of
// the corresponding family.
var (
	wildcardV4 = netip.IPv4Unspecified()
	wildcardV6 = netip.IPv6Unspecified()
)

// serviceResolver resolves remote service tags for intra-host connections.
type serviceResolver struct {
	// GetServiceContext returns service context tags (e.g. DD_SERVICE) for a PID.
	GetServiceContext func(pid int32) []string
	// GetProcessTags returns process-level tags for a PID (from tagger or process cache).
	GetProcessTags func(pid int32) []string
	// GetIISTags returns IIS-specific tags for a (remotePort, localPort) pair. May be nil.
	GetIISTags func(remotePort, localPort int32) []string
	// Listeners maps listening (IP, port) pairs to their owning PIDs.
	Listeners map[listenKey]int32
}

// listenerPID returns the PID listening on the given (ip, port). If no exact
// match exists it falls back to wildcard listeners (0.0.0.0 for IPv4, :: for
// IPv6, which may also serve IPv4-mapped addresses on dual-stack sockets).
func (r *serviceResolver) listenerPID(ip netip.Addr, port int32) (int32, bool) {
	if pid, ok := r.Listeners[listenKey{IP: ip, Port: port}]; ok {
		return pid, true
	}
	// IPv6 addresses contain a colon; IPv4 does not.
	if ip.Is6() {
		if pid, ok := r.Listeners[listenKey{IP: wildcardV6, Port: port}]; ok {
			return pid, true
		}
	} else {
		if pid, ok := r.Listeners[listenKey{IP: wildcardV4, Port: port}]; ok {
			return pid, true
		}
		// A dual-stack :: listener also accepts IPv4-mapped connections.
		if pid, ok := r.Listeners[listenKey{IP: wildcardV6, Port: port}]; ok {
			return pid, true
		}
	}
	return 0, false
}

// Resolve returns the remote service tags for an intra-host connection.
// pid is the local PID, remoteIP/remotePort and localPort are the connection's
// remote endpoint and local port. Returns nil if no remote tags can be resolved.
func (r *serviceResolver) Resolve(pid int32, remoteIP netip.Addr, remotePort, localPort int32) []string {
	var remoteTags []string

	// Try IIS tags first (Windows only, nil on Linux)
	if r.GetIISTags != nil {
		if iisTags := r.GetIISTags(remotePort, localPort); len(iisTags) > 0 {
			remoteTags = append(remoteTags, iisTags...)
		}
	}

	// Fallback: resolve by destination PID
	if len(remoteTags) == 0 {
		destPID, ok := r.listenerPID(remoteIP, remotePort)
		if !ok || destPID == pid {
			return nil
		}
		if r.GetServiceContext != nil {
			remoteTags = append(remoteTags, r.GetServiceContext(destPID)...)
		}
		if r.GetProcessTags != nil {
			if pidTags := r.GetProcessTags(destPID); len(pidTags) > 0 {
				remoteTags = append(remoteTags, pidTags...)
			}
		}
	}

	if len(remoteTags) == 0 {
		return nil
	}
	return remoteTags
}
