// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package cgroup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
)

type DiskMappingTestSuite struct {
	suite.Suite
	proc *tempFolder
}

func (s *DiskMappingTestSuite) SetupTest() {
	var err error
	s.proc, err = newTempFolder("test-disk-mapping")
	assert.NoError(s.T(), err)
	config.Datadog.SetDefault("container_proc_root", s.proc.RootPath)
}

func (s *DiskMappingTestSuite) TearDownTest() {
	cache.Cache.Delete(diskMappingCacheKey)
	config.Datadog.SetDefault("container_proc_root", "/proc")
	s.proc.removeAll()
	s.proc = nil
}

func (s *DiskMappingTestSuite) TestParsing() {
	s.proc.add("diskstats", detab(`
        7       0 loop0 0 0 0 0 0 0 0 0 0 0 0
        7       1 loop1 0 0 0 0 0 0 0 0 0 0 0
        invalidline
        8       0 sda 24398 2788 1317975 40488 25201 46267 1584744 142336 0 22352 182660
        8       1 sda1 24232 2788 1312025 40376 25201 46267 1584744 142336 0 22320 182552
        8      16 sdb 189 0 4063 220 0 0 0 0 0 112 204
    `))

	expectedMap := map[string]string{
		"7:0":  "loop0",
		"7:1":  "loop1",
		"8:0":  "sda",
		"8:1":  "sda1",
		"8:16": "sdb",
	}

	mapping, err := getDiskDeviceMapping()
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), expectedMap, mapping.idToName)

	cached, ok := cache.Cache.Get(diskMappingCacheKey)
	assert.True(s.T(), ok)
	assert.EqualValues(s.T(), &diskDeviceMapping{expectedMap}, cached)
}

func (s *DiskMappingTestSuite) TestNotFound() {
	mapping, err := getDiskDeviceMapping()
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "no such file or directory")
	assert.Nil(s.T(), mapping)
}

func (s *DiskMappingTestSuite) TestCached() {
	cachedMapping := &diskDeviceMapping{
		idToName: map[string]string{
			"1:2": "one",
			"2:3": "two",
		},
	}
	cache.Cache.Set(diskMappingCacheKey, cachedMapping, time.Minute)

	mapping, err := getDiskDeviceMapping()
	require.NoError(s.T(), err)
	assert.Equal(s.T(), cachedMapping, mapping)
}

func (s *DiskMappingTestSuite) TestContainerCgroupIO() {
	s.proc.add("diskstats", detab(`
        7       0 loop0 0 0 0 0 0 0 0 0 0 0 0
        7       1 loop1 0 0 0 0 0 0 0 0 0 0 0
        8       0 sda 24398 2788 1317975 40488 25201 46267 1584744 142336 0 22352 182660
        8       1 sda1 24232 2788 1312025 40376 25201 46267 1584744 142336 0 22320 182552
        8      16 sdb 189 0 4063 220 0 0 0 0 0 112 204
    `))

	tempFolder, err := newTempFolder("io-stats")
	assert.Nil(s.T(), err)
	defer tempFolder.removeAll()

	// 8:0  is sda
	// 8:16 is sdb
	// 55:0 is unknown, don't report per-device but keep in sum
	tempFolder.add("blkio/blkio.throttle.io_service_bytes", detab(`
		8:16 Read 1130496
		8:16 Write 0
		8:16 Sync 1130496
		8:16 Async 0
		8:16 Total 1130496
		8:0 Read 37858816
		8:0 Write 671846400
		8:0 Sync 262450688
		8:0 Async 447254528
		8:0 Total 709705216
		55:0 Read 55
		55:0 Write 55
		55:0 Sync 55
		55:0 Async 55
		55:0 Total 55
	`))

	cgroup := newDummyContainerCgroup(tempFolder.RootPath, "blkio", "blkio")

	expectedStats := &metrics.ContainerIOStats{
		ReadBytes:  uint64(1130496 + 37858816 + 55),
		WriteBytes: uint64(0 + 671846400 + 55),
		DeviceReadBytes: map[string]uint64{
			"sda": 37858816,
			"sdb": 1130496,
		},
		DeviceWriteBytes: map[string]uint64{
			"sda": 671846400,
			"sdb": 0,
		},
	}

	ioStat, err := cgroup.IO()
	assert.Nil(s.T(), err)
	assert.EqualValues(s.T(), expectedStats, ioStat)
}

func (s *DiskMappingTestSuite) TestContainerCgroupIOFailedMapping() {
	tempFolder, err := newTempFolder("io-stats")
	assert.Nil(s.T(), err)
	defer tempFolder.removeAll()

	// 8:0  is sda
	// 8:16 is sdb
	// 55:0 is unknown, don't report per-device but keep in sum
	tempFolder.add("blkio/blkio.throttle.io_service_bytes", detab(`
		8:16 Read 1130496
		8:16 Write 0
		8:16 Sync 1130496
		8:16 Async 0
		8:16 Total 1130496
		8:0 Read 37858816
		8:0 Write 671846400
		8:0 Sync 262450688
		8:0 Async 447254528
		8:0 Total 709705216
		55:0 Read 55
		55:0 Write 55
		55:0 Sync 55
		55:0 Async 55
		55:0 Total 55
	`))

	cgroup := newDummyContainerCgroup(tempFolder.RootPath, "blkio", "blkio")

	expectedStats := &metrics.ContainerIOStats{
		ReadBytes:        uint64(1130496 + 37858816 + 55),
		WriteBytes:       uint64(0 + 671846400 + 55),
		DeviceReadBytes:  map[string]uint64{},
		DeviceWriteBytes: map[string]uint64{},
	}

	ioStat, err := cgroup.IO()
	assert.Nil(s.T(), err)
	assert.EqualValues(s.T(), expectedStats, ioStat)
}

func TestDiskMappingTestSuite(t *testing.T) {
	suite.Run(t, new(DiskMappingTestSuite))
}
