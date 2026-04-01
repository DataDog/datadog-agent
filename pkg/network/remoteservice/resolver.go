// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remoteservice provides shared logic for resolving remote service tags
// on intra-host connections. It is used by both the process-agent (net.go) and
// the system-probe direct sender to enrich connections with RemoteServiceTagsIdx.
package remoteservice

// Resolver resolves remote service tags for intra-host connections.
type Resolver struct {
	// GetServiceContext returns service context tags (e.g. DD_SERVICE) for a PID.
	GetServiceContext func(pid int32) []string
	// GetProcessTags returns process-level tags for a PID (from tagger or process cache).
	GetProcessTags func(pid int32) []string
	// GetIISTags returns IIS-specific tags for a (remotePort, localPort) pair. May be nil.
	GetIISTags func(remotePort, localPort int32) []string
	// PortToPID maps listening ports to their owning PIDs.
	PortToPID map[int32]int32
}

// Resolve returns the remote service tags for an intra-host connection.
// pid is the local PID, remotePort and localPort are the connection's remote/local ports.
// Returns nil if no remote tags can be resolved.
func (r *Resolver) Resolve(pid int32, remotePort, localPort int32) []string {
	var remoteTags []string

	// Try IIS tags first (Windows only, nil on Linux)
	if r.GetIISTags != nil {
		if iisTags := r.GetIISTags(remotePort, localPort); len(iisTags) > 0 {
			remoteTags = append(remoteTags, iisTags...)
		}
	}

	// Fallback: resolve by destination PID
	if len(remoteTags) == 0 {
		destPID, ok := r.PortToPID[remotePort]
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
