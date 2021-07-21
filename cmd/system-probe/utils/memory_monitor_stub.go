// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !linux

package utils

// MemoryMonitor monitors memory cgroup usage
type MemoryMonitor struct{}

// NewMemoryMonitor instantiates a new memory monitor
func NewMemoryMonitor(pressureLevels map[string]string, thresholds map[string]string) (*MemoryMonitor, error) {
	return &MemoryMonitor{}, nil
}

// Start monitoring memory
func (mm *MemoryMonitor) Start() {}

// Stop monitoring memory
func (mm *MemoryMonitor) Stop() {}
