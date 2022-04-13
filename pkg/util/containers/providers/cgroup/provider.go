// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inmetrics.

//go:build linux
// +build linux

package cgroup

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/providers"
)

// provider is a Cgroup implementation of the ContainerImplementation interface
type provider struct {
	cgroups map[string]*ContainerCgroup
	lock    sync.RWMutex
}

func init() {
	providers.Register(&provider{})
}

// Prefetch gets data from all cgroups in one go
// If not successful all other calls will fail
func (mp *provider) Prefetch() error {
	mp.lock.Lock()
	defer mp.lock.Unlock()

	var err error
	mp.cgroups, err = scrapeAllCgroups()
	return err
}

// ContainerExists returns true if a cgroup exists for this containerID
func (mp *provider) ContainerExists(containerID string) bool {
	_, err := mp.getCgroup(containerID)
	return err == nil
}

// GetContainerStartTime returns container start time
func (mp *provider) GetContainerStartTime(containerID string) (int64, error) {
	cg, err := mp.getCgroup(containerID)
	if err != nil {
		return 0, err
	}

	startedAt, err := cg.ContainerStartTime()
	if err != nil {
		return 0, err
	}

	return startedAt, nil
}

// GetContainerMetrics returns CPU, IO and Memory metrics
func (mp *provider) GetContainerMetrics(containerID string) (*metrics.ContainerMetrics, error) {
	cg, err := mp.getCgroup(containerID)
	if err != nil {
		return nil, err
	}

	var metrics metrics.ContainerMetrics
	metrics.Memory, err = cg.Mem()
	if err != nil {
		return nil, fmt.Errorf("memory: %s", err)
	}
	metrics.Memory.KernMemUsage, err = cg.KernelMemoryUsage()
	if err != nil {
		return nil, fmt.Errorf("kernel mem usage: %s", err)
	}
	metrics.Memory.SoftMemLimit, err = cg.SoftMemLimit()
	if err != nil {
		return nil, fmt.Errorf("soft mem limit: %s", err)
	}
	metrics.Memory.MemFailCnt, err = cg.FailedMemoryCount()
	if err != nil {
		return nil, fmt.Errorf("failed mem count: %s", err)
	}
	metrics.CPU, err = cg.CPU()
	if err != nil {
		return nil, fmt.Errorf("cpu: %s", err)
	}
	metrics.CPU.NrThrottled, metrics.CPU.ThrottledTime, err = cg.CPUPeriods()
	if err != nil {
		return nil, fmt.Errorf("cpu nr: %s", err)
	}
	metrics.CPU.ThreadCount, err = cg.ThreadCount()
	if err != nil {
		return nil, fmt.Errorf("thread count: %s", err)
	}
	metrics.IO, err = cg.IO()
	if err != nil {
		return nil, fmt.Errorf("i/o: %s", err)
	}

	return &metrics, nil
}

// GetContainerLimits returns CPU, Thread and Memory limits
func (mp *provider) GetContainerLimits(containerID string) (*metrics.ContainerLimits, error) {
	cg, err := mp.getCgroup(containerID)
	if err != nil {
		return nil, err
	}

	var limits metrics.ContainerLimits
	limits.CPULimit, err = cg.CPULimit()
	if err != nil {
		return nil, fmt.Errorf("cpu limit: %s", err)
	}
	limits.MemLimit, err = cg.MemLimit()
	if err != nil {
		return nil, fmt.Errorf("mem limit: %s", err)
	}
	limits.ThreadLimit, err = cg.ThreadLimit()
	if err != nil {
		return nil, fmt.Errorf("thread limit: %s", err)
	}

	return &limits, nil
}

// GetNetworkMetrics return network metrics for all PIDs in container
func (mp *provider) GetNetworkMetrics(containerID string, networks map[string]string) (metrics.ContainerNetStats, error) {
	cg, err := mp.getCgroup(containerID)
	if err != nil {
		return nil, err
	}

	if len(cg.Pids) == 0 {
		return nil, errors.New("no pid for this container")
	}

	metrics, err := collectNetworkStats(int(cg.Pids[len(cg.Pids)-1]), networks)
	if err != nil {
		return nil, fmt.Errorf("Could not collect network stats for container %s: %s", containerID[:12], err)
	}

	return metrics, nil
}

// GetAgentCID returns the container ID where the current agent is running
func (mp *provider) GetAgentCID() (string, error) {
	prefix := config.Datadog.GetString("container_cgroup_prefix")
	cID, _, err := readCgroupsForPath("/proc/self/cgroup", prefix)
	if err != nil {
		return "", err
	}
	return cID, err
}

// GetPIDs returns all PIDs running in the current container
func (mp *provider) GetPIDs(containerID string) ([]int32, error) {
	cg, err := mp.getCgroup(containerID)
	if err != nil {
		return nil, err
	}

	return cg.Pids, nil
}

// ContainerIDForPID is a lighter version of CgroupsForPids to only retrieve the
// container ID for origin detection. Returns container id as a string, empty if
// the PID is not in a container.
//
// Matching is tested for docker on known cgroup variations, and
// containerd / cri-o default Kubernetes cgroups
func (mp *provider) ContainerIDForPID(pid int) (string, error) {
	cgPath := hostProc(strconv.Itoa(pid), "cgroup")
	prefix := config.Datadog.GetString("container_cgroup_prefix")

	containerID, _, err := readCgroupsForPath(cgPath, prefix)

	return containerID, err
}

// DetectNetworkDestinations lists all the networks available
// to a given PID and parses them in NetworkInterface objects
func (mp *provider) DetectNetworkDestinations(pid int) ([]containers.NetworkDestination, error) {
	return detectNetworkDestinations(pid)
}

// GetDefaultGateway returns the default gateway used by container implementation
func (mp *provider) GetDefaultGateway() (net.IP, error) {
	return defaultGateway()
}

// GetDefaultHostIPs returns the IP addresses bound to the default network interface.
// The default network interface is the one connected to the network gateway, and it is determined
// by parsing the routing table file in the proc file system.
func (mp *provider) GetDefaultHostIPs() ([]string, error) {
	return defaultHostIPs()
}

// GetNumFileDescriptors returns the number of open file descriptors for a given
// pid
func (mp *provider) GetNumFileDescriptors(pid int) (int, error) {
	return GetFileDescriptorLen(pid)
}

func (mp *provider) getCgroup(containerID string) (*ContainerCgroup, error) {
	mp.lock.RLock()
	defer mp.lock.RUnlock()

	cg, ok := mp.cgroups[containerID]
	if !ok || cg == nil {
		return nil, fmt.Errorf("Cgroup not found for container: %s", containerID[:12])
	}

	return cg, nil
}
