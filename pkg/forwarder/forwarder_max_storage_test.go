// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forwarder

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/stretchr/testify/require"
)

type diskUsageRetrieverMock struct {
	diskUsage *filesystem.DiskUsage
}

func (m diskUsageRetrieverMock) GetUsage(path string) (*filesystem.DiskUsage, error) {
	return m.diskUsage, nil
}

func TestComputeMaxStorage(t *testing.T) {
	r := require.New(t)
	disk := diskUsageRetrieverMock{
		diskUsage: &filesystem.DiskUsage{
			Available: 30,
			Total:     100,
		}}
	maxSizeInBytes := int64(30)
	storage, err := newForwarderMaxStorage("", disk, maxSizeInBytes, 0.9)
	r.NoError(err)

	max, err := storage.computeMaxStorage(10)
	r.NoError(err)
	r.Equal(maxSizeInBytes, max)

	max, err = storage.computeMaxStorage(5)
	r.NoError(err)
	r.Equal(30-int64(100*(1-0.9))+5, max)
}
