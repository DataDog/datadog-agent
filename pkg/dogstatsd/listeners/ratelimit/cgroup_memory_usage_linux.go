// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux
// +build linux

package ratelimit

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
)

var _ memoryUsage = (*hostMemoryUsage)(nil)

// cgroupMemoryUsage provides a method to return the cgroup memory memory usage rate.
type cgroupMemoryUsage struct {
	reader *cgroups.Reader
}

func newCgroupMemoryUsage() (*cgroupMemoryUsage, error) {
	reader, err := cgroups.NewReader()
	if err != nil {
		return nil, err
	}
	limit := &cgroupMemoryUsage{
		reader: reader,
	}

	// Check if a Cgroup is defined
	if _, err := limit.getMemoryUsageRate(); err != nil {
		return nil, err
	}

	return limit, nil
}

func (c *cgroupMemoryUsage) getMemoryUsageRate() (float64, error) {
	if err := c.reader.RefreshCgroups(15 * time.Minute); err != nil {
		return 0, err
	}
	groups := c.reader.ListCgroups()
	if len(groups) != 1 {
		return 0, fmt.Errorf("cannot find a single cgroup. Found: %v groups", len(groups))
	}
	var stats cgroups.MemoryStats
	if err := groups[0].GetMemoryStats(&stats); err != nil {
		return 0, err
	}
	if stats.Limit == nil || *stats.Limit == 0 {
		return 0, errors.New("cannot get the memory `Limit`")
	}
	if stats.UsageTotal == nil || *stats.UsageTotal == 0 {
		return 0, errors.New("cannot get the memory `UsageTotal`")
	}

	return float64(*stats.UsageTotal) / float64(*stats.Limit), nil
}
