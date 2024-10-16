// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build process && linux

package sysprobe

import "github.com/DataDog/datadog-agent/pkg/process/net"

// GetSystemProbeConntrackCached queries conntrack/cached, which uses our conntracker implementation (typically ebpf)
// to return the list of NAT'd connections
func GetSystemProbeConntrackCached(socketPath string) ([]byte, error) {
	probeUtil, err := net.GetRemoteSystemProbeUtil(socketPath)
	if err != nil {
		return nil, err
	}
	return probeUtil.GetConnTrackCached()
}

// GetSystemProbeConntrackHost queries conntrack/host, which uses netlink to return the list of NAT'd connections
func GetSystemProbeConntrackHost(socketPath string) ([]byte, error) {
	probeUtil, err := net.GetRemoteSystemProbeUtil(socketPath)
	if err != nil {
		return nil, err
	}
	return probeUtil.GetConnTrackHost()
}

// GetSystemProbeConntrackHostFull queries conntrack/host_full, which uses netlink to return the connections
func GetSystemProbeConntrackHostFull(socketPath string) ([]byte, error) {
	probeUtil, err := net.GetRemoteSystemProbeUtil(socketPath)
	if err != nil {
		return nil, err
	}
	return probeUtil.GetConnTrackHostFull()
}
