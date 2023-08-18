// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package system

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

type DiskMappingTestSuite struct {
	suite.Suite
	proc *testutil.TempFolder
}

func (s *DiskMappingTestSuite) SetupTest() {
	var err error
	s.proc, err = testutil.NewTempFolder("test-disk-mapping")
	assert.NoError(s.T(), err)
}

func (s *DiskMappingTestSuite) TearDownTest() {
	cache.Cache.Delete(diskMappingCacheKey)
	s.proc.RemoveAll()
	s.proc = nil
}

func (s *DiskMappingTestSuite) TestParsing() {
	s.proc.Add("diskstats", testutil.Detab(`
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

	mapping, err := GetDiskDeviceMapping(s.proc.RootPath)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), expectedMap, mapping)

	cached, ok := cache.Cache.Get(diskMappingCacheKey)
	assert.True(s.T(), ok)
	assert.EqualValues(s.T(), expectedMap, cached)
}

func (s *DiskMappingTestSuite) TestNotFound() {
	mapping, err := GetDiskDeviceMapping(s.proc.RootPath)
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "no such file or directory")
	assert.Nil(s.T(), mapping)
}

func (s *DiskMappingTestSuite) TestCached() {
	cachedMapping := map[string]string{
		"1:2": "one",
		"2:3": "two",
	}
	cache.Cache.Set(diskMappingCacheKey, cachedMapping, time.Minute)

	mapping, err := GetDiskDeviceMapping(s.proc.RootPath)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), cachedMapping, mapping)
}

func TestDiskMappingTestSuite(t *testing.T) {
	suite.Run(t, new(DiskMappingTestSuite))
}
