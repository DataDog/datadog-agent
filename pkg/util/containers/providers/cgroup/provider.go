// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inmetrics.

// +build linux

package cgroup

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
)

// Provider is a Cgroup implementation of the ContainerImplementation interface
type Provider struct {
	cgroups map[string]*ContainerCgroup
}

// Prefetch gets data from all cgroups in one go
// If not successful all other calls will fail
func (mp *Provider) Prefetch() error {
	var err error
	mp.cgroups, err = scrapeAllCgroups()
	return err
}

// ContainerExists returns true if a cgroup exists for this containerID
func (mp *Provider) ContainerExists(containerID string) bool {
	_, err := mp.getCgroup(containerID)
	return err == nil
}

// GetContainerStartTime returns container start time
func (mp *Provider) GetContainerStartTime(containerID string) (int64, error) {
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
func (mp *Provider) GetContainerMetrics(containerID string) (*metrics.ContainerMetrics, error) {
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
	metrics.CPU.NrThrottled, err = cg.CPUNrThrottled()
	if err != nil {
		return nil, fmt.Errorf("cpuNrThrottled: %s", err)
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
func (mp *Provider) GetContainerLimits(containerID string) (*metrics.ContainerLimits, error) {
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
func (mp *Provider) GetNetworkMetrics(containerID string, networks map[string]string) (metrics.ContainerNetStats, error) {
	cg, ok := mp.cgroups[containerID]
	if !ok || cg == nil {
		return nil, fmt.Errorf("Cgroup not found for container: %s", containerID[:12])
	}

	if len(cg.Pids) == 0 {
		return nil, errors.New("no pid for this container")
	}

	metrics, err := collectNetworkStats(int(cg.Pids[0]), networks)
	if err != nil {
		return nil, fmt.Errorf("Could not collect network stats for container %s: %s", containerID[:12], err)
	}

	return metrics, nil
}

// GetAgentCID returns the container ID where the current agent is running
func (mp *Provider) GetAgentCID() (string, error) {
	prefix := config.Datadog.GetString("container_cgroup_prefix")
	cID, _, err := readCgroupsForPath("/proc/self/cgroup", prefix)
	if err != nil {
		return "", err
	}
	return cID, err
}

// ContainerIDForPID is a lighter version of CgroupsForPids to only retrieve the
// container ID for origin detection. Returns container id as a string, empty if
// the PID is not in a container.
//
// Matching is tested for docker on known cgroup variations, and
// containerd / cri-o default Kubernetes cgroups
func (mp *Provider) ContainerIDForPID(pid int) (string, error) {
	cgPath := hostProc(strconv.Itoa(pid), "cgroup")
	prefix := config.Datadog.GetString("container_cgroup_prefix")

	containerID, _, err := readCgroupsForPath(cgPath, prefix)

	return containerID, err
}

// DetectNetworkDestinations lists all the networks available
// to a given PID and parses them in NetworkInterface objects
func (mp *Provider) DetectNetworkDestinations(pid int) ([]containers.NetworkDestination, error) {
	return detectNetworkDestinations(pid)
}

func (mp *Provider) getCgroup(containerID string) (*ContainerCgroup, error) {
	cg, ok := mp.cgroups[containerID]
	if !ok || cg == nil {
		return nil, fmt.Errorf("Cgroup not found for container: %s", containerID[:12])
	}

	return cg, nil
}
