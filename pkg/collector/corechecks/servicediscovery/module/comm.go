// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"bytes"
	"os"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// ignoreComms is a list of process names that should not be reported as a service.
var ignoreComms = map[string]struct{}{
	"systemd":         {}, // manages system and service components
	"dhclient":        {}, // utility that uses the DHCP to configure network interfaces
	"local-volume-pr": {}, // 'local-volume-provisioner' manages the lifecycle of Persistent Volumes
	"sshd":            {}, // a daemon that handles secure communication
	"cilium-agent":    {}, // accepts configuration for networking, load balancing etc. (like cilium-agent)
	"kubelet":         {}, // Kubernetes agent
	"chronyd":         {}, // a daemon that implements the Network Time Protocol (NTP)
	"containerd":      {}, // engine to run containers
	"dockerd":         {}, // engine to run containers and 'docker-proxy'
	"livenessprobe":   {}, // Kubernetes tool that monitors a container's health
}

// ignoreFamily list of commands that should not be reported as a service.
var ignoreFamily = map[string]struct{}{
	"systemd":    {}, // 'systemd-networkd', 'systemd-resolved' etc
	"datadog":    {}, // datadog processes
	"containerd": {}, // 'containerd-shim...'
	"dockerd":    {}, // 'docker-proxy'
}

// shouldIgnoreComm returns true if process should be ignored
func shouldIgnoreComm(proc *process.Process) bool {
	if shouldIgnoreKernelThread(proc) {
		return true
	}
	commPath := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "comm")
	contents, err := os.ReadFile(commPath)
	if err != nil {
		return true
	}

	dash := bytes.IndexByte(contents, '-')
	if dash > 0 {
		_, found := ignoreFamily[string(contents[:dash])]
		if found {
			return true
		}
	}

	comm := strings.TrimSuffix(string(contents), "\n")
	_, found := ignoreComms[comm]

	return found
}

// shouldIgnoreKernelThread returns true if process is kernel thread
func shouldIgnoreKernelThread(proc *process.Process) bool {
	exePath := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "exe")
	_, err := os.Stat(exePath)
	if err != nil {
		// this is a kernel thread if /proc/<pid>/exe could not be found
		return true
	}
	return false
}
