// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package ratelimit

import (
	"github.com/DataDog/gopsutil/mem"
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

	return float64(memoryStats.Used), float64(memoryStats.Total), nil
}
