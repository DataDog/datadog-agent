// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package containers

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
)

// SetCgroups has to be called when creating the Container, in order to
// be able to enable FillCgroupMetrics and FillNetworkMetrics to works
func (c *Container) SetCgroups(cgroup *metrics.ContainerCgroup) error {
	c.cgroup = cgroup
	c.Pids = c.cgroup.Pids
	return nil
}

// FillCgroupLimits fills the resource limits for a Container, based on the
// associated cgroups. This can be called once if the limits are assumed constant.
func (c *Container) FillCgroupLimits() error {
	if c.cgroup == nil {
		return errors.New("no cgroup for this container")
	}
	var err error

	c.CPULimit, err = c.cgroup.CPULimit()
	if err != nil {
		return fmt.Errorf("cpu limit: %s", err)
	}
	c.MemLimit, err = c.cgroup.MemLimit()
	if err != nil {
		return fmt.Errorf("mem limit: %s", err)
	}
	c.KernMemUsage, err = c.cgroup.KernelMemoryUsage()
	if err != nil {
		return fmt.Errorf("kernel mem usage: %s", err)
	}
	c.SoftMemLimit, err = c.cgroup.SoftMemLimit()
	if err != nil {
		return fmt.Errorf("soft mem limit: %s", err)
	}
	c.MemFailCnt, err = c.cgroup.FailedMemoryCount()
	if err != nil {
		return fmt.Errorf("failed mem count: %s", err)
	}
	c.ThreadLimit, err = c.cgroup.ThreadLimit()
	if err != nil {
		return fmt.Errorf("thread limit: %s", err)
	}

	return nil
}

// FillCgroupMetrics fills the performance metrics for a Container, based on the
// associated cgroups. Network metrics are handled by FillNetworkMetrics
func (c *Container) FillCgroupMetrics() error {
	if c.cgroup == nil {
		return errors.New("no cgroup for this container")
	}
	var err error

	c.Memory, err = c.cgroup.Mem()
	if err != nil {
		return fmt.Errorf("memory: %s", err)
	}
	c.CPU, err = c.cgroup.CPU()
	if err != nil {
		return fmt.Errorf("cpu: %s", err)
	}
	c.CPUNrThrottled, err = c.cgroup.CPUNrThrottled()
	if err != nil {
		return fmt.Errorf("cpuNrThrottled: %s", err)
	}
	c.IO, err = c.cgroup.IO()
	if err != nil {
		return fmt.Errorf("i/o: %s", err)
	}
	c.StartedAt, err = c.cgroup.ContainerStartTime()
	if err != nil {
		return fmt.Errorf("start time: %s", err)
	}
	c.ThreadCount, err = c.cgroup.ThreadCount()
	if err != nil {
		return fmt.Errorf("thread count: %s", err)
	}

	return nil
}

// FillNetworkMetrics fills the network metrics for a Container,
// based on the associated cgroups.
func (c *Container) FillNetworkMetrics(networks map[string]string) error {
	if c.cgroup == nil {
		return errors.New("no cgroup for this container")
	}
	if len(c.cgroup.Pids) == 0 {
		return errors.New("no pid for this container")
	}
	var err error
	c.Network, err = metrics.CollectNetworkStats(int(c.cgroup.Pids[0]), networks)
	if err != nil {
		return fmt.Errorf("Could not collect network stats for container %s: %s", c.ID, err)
	}
	return nil
}
