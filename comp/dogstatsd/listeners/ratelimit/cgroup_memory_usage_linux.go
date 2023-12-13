// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux

package ratelimit

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
)

var _ memoryUsage = (*hostMemoryUsage)(nil)

// cgroupMemoryUsage provides a method to return the cgroup memory memory usage rate.
type cgroupMemoryUsage struct {
	cgroup cgroups.Cgroup
}

func newCgroupMemoryUsage() (*cgroupMemoryUsage, error) {
	selfReader, err := cgroups.NewSelfReader("/proc", config.IsContainerized())
	if err != nil {
		return nil, err
	}

	cgroup := selfReader.GetCgroup(cgroups.SelfCgroupIdentifier)
	if cgroup == nil {
		return nil, errors.New("cannot get cgroup")
	}

	cgroupMemoryUsage := &cgroupMemoryUsage{
		cgroup: cgroup,
	}

	// Make sure cgroup is available
	if _, _, err := cgroupMemoryUsage.getMemoryStats(); err != nil {
		return nil, err
	}

	return cgroupMemoryUsage, nil
}

func (c *cgroupMemoryUsage) getMemoryStats() (float64, float64, error) {
	var stats cgroups.MemoryStats
	if err := c.cgroup.GetMemoryStats(&stats); err != nil {
		return 0, 0, err
	}
	if stats.Limit == nil {
		return 0, 0, errors.New("cannot get the memory `Limit`")
	}
	if stats.UsageTotal == nil {
		return 0, 0, errors.New("cannot get the memory `UsageTotal`")
	}

	return float64(*stats.UsageTotal), float64(*stats.Limit), nil
}
