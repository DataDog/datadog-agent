// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// +build !windows

package filesystem

import "github.com/shirou/gopsutil/disk"

type Disk struct{}

func NewDisk() Disk {
	return Disk{}
}

func (Disk) GetUsage(path string) (*DiskUsage, error) {
	usage, err := disk.Usage(path)
	if err != nil {
		return nil, err
	}

	return &DiskUsage{
		Total:     usage.Total,
		Used:      usage.Used,
		Available: usage.Free,
	}, nil
}
