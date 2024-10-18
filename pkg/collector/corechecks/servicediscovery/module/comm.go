// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"github.com/shirou/gopsutil/v3/process"
)

// ignoreComms is a list of process names that should not be reported as a service.
var ignoreComms = map[string]struct{}{
	"sshd":           {}, // a daemon that handles secure communication
	"dhclient":       {}, // utility that uses the DHCP to configure network interfaces
	"systemd":        {}, // manages system and service components for Linux
	"systemd-resolv": {}, // 'systemd-resolved' a system service that provides network name resolution for local applications
	"systemd-networ": {}, // 'systemd-networkd' manages network configurations for Linux system
	"datadog-agent":  {}, // datadog core agent
	"livenessprobe":  {}, // Kubernetes tool that monitors a container's health
	"docker-proxy":   {}, // forwards traffic to containers
	"cilium-agent":   {}, // accepts configuration for networking, load balancing etc.
	"kubelet":        {}, // Kubernetes agent
	"chronyd":        {}, // a daemon that implements the Network Time Protocol (NTP)
	"containerd":     {}, // engine to run containers
	"dockerd":        {}, // engine to run containers
	"local-volume-p": {}, // 'local-volume-provisioner' manages the lifecycle of Persistent Volumes
}

// ignoreComm returns true if process should be ignored
func ignoreComm(proc *process.Process) bool {
	comm, err := proc.Name()
	if err != nil {
		return false
	}
	// The proc.Name() function returns an unpredictable length string when command name is long.
	// It most likely returns the full command name, taken from /proc/<pid>/cmdline,
	// but it may occasionally return a 15-byte name, maybe /proc/<pid>/cmdline is not ready yet.
	// search first 15 bytes of returned command.
	found := func(comm string, m map[string]struct{}) bool {
		if len(comm) > 15 {
			comm = comm[:15]
		}
		_, found := m[comm]
		return found
	}(comm, ignoreComms)

	return found
}
