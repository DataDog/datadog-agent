// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package ratelimit

import (
	"github.com/shirou/gopsutil/v3/mem"
)

var _ memoryUsage = (*hostMemoryUsage)(nil)

type hostMemoryUsage struct{}

func newHostMemoryUsage() *hostMemoryUsage {
	return &hostMemoryUsage{}
}

func (m *hostMemoryUsage) getMemoryStats() (float64, float64, error) {
	memoryStats, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, err
	}

	// using shirou/gopsutil results in a different Used value, see
	// https://github.com/shirou/gopsutil/commit/4510db20dbab6fdd64061e5d6c7e93015c115fd5
	return float64(memoryStats.Used), float64(memoryStats.Total), nil
}
