// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build aix

package filesystem

import (
	gopsutildisk "github.com/shirou/gopsutil/v4/disk"
)

func getFileSystemInfo() ([]MountInfo, error) {
	partitions, err := gopsutildisk.Partitions(true)
	if err != nil {
		return nil, err
	}

	var mounts []MountInfo
	for _, part := range partitions {
		usage, err := gopsutildisk.Usage(part.Mountpoint)
		if err != nil || usage.Total == 0 {
			continue
		}
		mounts = append(mounts, MountInfo{
			Name:      part.Device,
			SizeKB:    usage.Total / 1024,
			MountedOn: part.Mountpoint,
		})
	}
	return mounts, nil
}
