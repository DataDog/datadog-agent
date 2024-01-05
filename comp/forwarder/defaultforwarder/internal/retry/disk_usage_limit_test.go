// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

type diskUsageRetrieverMock struct {
	diskUsage *filesystem.DiskUsage
}

func (m diskUsageRetrieverMock) GetUsage(_ string) (*filesystem.DiskUsage, error) {
	return m.diskUsage, nil
}

func TestComputeAvailableSpace(t *testing.T) {
	r := require.New(t)
	disk := diskUsageRetrieverMock{
		diskUsage: &filesystem.DiskUsage{
			Available: 30,
			Total:     100,
		}}
	maxSizeInBytes := int64(30)
	diskUsageLimit := NewDiskUsageLimit("", disk, maxSizeInBytes, 0.9)

	max, err := diskUsageLimit.computeAvailableSpace(10)
	r.NoError(err)
	r.Equal(maxSizeInBytes, max)

	max, err = diskUsageLimit.computeAvailableSpace(5)
	r.NoError(err)
	r.Equal(30-int64(100*(1-0.9))+5, max)
}
