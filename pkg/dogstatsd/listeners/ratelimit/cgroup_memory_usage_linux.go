// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux
// +build linux

package ratelimit

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	rate, err := limit.getMemoryUsageRate()
	if err != nil {
		return nil, err
	}
	fmt.Println("Initial memory rate is: ", rate)
	return limit, nil
}

func (c *cgroupMemoryUsage) getMemoryUsageRate() (float64, error) {
	usageBytes, err := os.ReadFile("/sys/fs/cgroup/memory/memory.usage_in_bytes")
	if err != nil {
		return 0, log.Errorf("Cannot read memory.usage_in_bytes %v", err)
	}

	limitBytes, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	if err != nil {
		return 0, log.Errorf("Cannot read memory.limit_in_bytes %v", err)
	}

	usageStr := strings.TrimSpace(string(usageBytes))
	usage, err := strconv.Atoi(usageStr)
	if err != nil {
		return 0, log.Errorf("Usage invalid number %v %v", usageStr, err)
	}

	limitStr := strings.TrimSpace(string(limitBytes))
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		return 0, log.Errorf("Limit invalid number %v %v", limitStr, err)
	}

	return float64(usage) / float64(limit), nil
}
