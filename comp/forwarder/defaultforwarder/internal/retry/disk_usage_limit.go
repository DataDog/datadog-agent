// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	math "math"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// DiskUsageLimit provides `computeAvailableSpace` which returns
// the amount of disk space that can be used to store transactions.
type DiskUsageLimit struct {
	diskPath       string
	maxSizeInBytes int64
	disk           diskUsageRetriever
	maxDiskRatio   float64
}

type diskUsageRetriever interface {
	GetUsage(path string) (*filesystem.DiskUsage, error)
}

// NewDiskUsageLimit creates a new instance of NewDiskUsageLimit
func NewDiskUsageLimit(
	diskPath string,
	disk diskUsageRetriever,
	maxSizeInBytes int64,
	maxDiskRatio float64) *DiskUsageLimit {
	return &DiskUsageLimit{
		diskPath:       diskPath,
		maxSizeInBytes: maxSizeInBytes,
		disk:           disk,
		maxDiskRatio:   maxDiskRatio,
	}
}

func (s *DiskUsageLimit) computeAvailableSpace(currentSize int64) (int64, error) {
	usage, err := s.disk.GetUsage(s.diskPath)
	if err != nil {
		return 0, err
	}
	diskReserved := float64(usage.Total) * (1 - s.maxDiskRatio)
	availableDiskUsage := int64(usage.Available) - int64(math.Ceil(diskReserved))

	return min(s.maxSizeInBytes, currentSize+availableDiskUsage), nil
}

func (s *DiskUsageLimit) getMaxSizeInBytes() int64 {
	return s.maxSizeInBytes
}
