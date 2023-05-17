// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package filesystem

import "github.com/shirou/gopsutil/v3/disk"

// Disk gets information about the disk
type Disk struct{}

// NewDisk creates a new instance of Disk
func NewDisk() Disk {
	return Disk{}
}

// GetUsage gets the disk usage
func (Disk) GetUsage(path string) (*DiskUsage, error) {
	usage, err := disk.Usage(path)
	if err != nil {
		return nil, err
	}

	return &DiskUsage{
		Total:     usage.Total,
		Available: usage.Free,
	}, nil
}
