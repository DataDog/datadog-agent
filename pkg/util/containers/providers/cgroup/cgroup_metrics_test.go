// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package cgroup

import (
	"math"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCPU(t *testing.T) {
	tempFolder, err := newTempFolder("cpu-stats")
	assert.Nil(t, err)
	defer tempFolder.removeAll()

	cpuacctStats := dummyCgroupStat{
		"user":   64140,
		"system": 18327,
	}
	tempFolder.add("cpuacct/cpuacct.stat", cpuacctStats.String())
	tempFolder.add("cpuacct/cpuacct.usage", "915266418275")
	tempFolder.add("cpu/cpu.shares", "1024")

	cgroup := newDummyContainerCgroup(tempFolder.RootPath, "cpuacct", "cpu")

	timeStat, err := cgroup.CPU()
	assert.Nil(t, err)
	assert.Equal(t, timeStat.User, uint64(64140))
	assert.Equal(t, timeStat.System, uint64(18327))
	assert.Equal(t, timeStat.Shares, uint64(1024))
	assert.InDelta(t, timeStat.UsageTotal, 91526.6418275, 0.0000001)
}

func TestCPUNrThrottled(t *testing.T) {
	tempFolder, err := newTempFolder("cpu-throttled")
	assert.Nil(t, err)
	defer tempFolder.removeAll()

	cgroup := newDummyContainerCgroup(tempFolder.RootPath, "cpu")

	// No file
	value, err := cgroup.CPUNrThrottled()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(0))

	// Invalid file
	tempFolder.add("cpu/cpu.stat", "200")
	_, err = cgroup.CPUNrThrottled()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(0))

	// Valid file
	cpuStats := dummyCgroupStat{
		"nr_periods":     0,
		"nr_throttled":   10,
		"throttled_time": 18327,
	}
	tempFolder.add("cpu/cpu.stat", cpuStats.String())
	value, err = cgroup.CPUNrThrottled()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(10))
}

func TestMemLimit(t *testing.T) {
	tempFolder, err := newTempFolder("mem-limit")
	assert.Nil(t, err)
	defer tempFolder.removeAll()

	cgroup := newDummyContainerCgroup(tempFolder.RootPath, "memory")

	// No file
	value, err := cgroup.MemLimit()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(0))

	// Invalid file
	tempFolder.add("memory/memory.limit_in_bytes", "ab")
	value, err = cgroup.MemLimit()
	assert.NotNil(t, err)
	assert.IsType(t, err, &strconv.NumError{})
	assert.Equal(t, value, uint64(0))

	// Overflow value
	tempFolder.add("memory/memory.limit_in_bytes", strconv.Itoa(int(math.Pow(2, 61))))
	value, err = cgroup.MemLimit()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(0))

	// Valid value
	tempFolder.add("memory/memory.limit_in_bytes", "1234")
	value, err = cgroup.MemLimit()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(1234))
}

func TestSoftMemLimit(t *testing.T) {
	tempFolder, err := newTempFolder("soft-mem-limit")
	assert.Nil(t, err)
	defer tempFolder.removeAll()

	cgroup := newDummyContainerCgroup(tempFolder.RootPath, "memory")

	// No file
	value, err := cgroup.SoftMemLimit()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(0))

	// Invalid file
	tempFolder.add("memory/memory.soft_limit_in_bytes", "ab")
	value, err = cgroup.SoftMemLimit()
	assert.NotNil(t, err)
	assert.IsType(t, err, &strconv.NumError{})
	assert.Equal(t, value, uint64(0))

	// Overflow value
	tempFolder.add("memory/memory.soft_limit_in_bytes", strconv.Itoa(int(math.Pow(2, 61))))
	value, err = cgroup.SoftMemLimit()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(0))

	// Valid value
	tempFolder.add("memory/memory.soft_limit_in_bytes", "1234")
	value, err = cgroup.SoftMemLimit()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(1234))
}

func TestParseSingleStat(t *testing.T) {
	tempFolder, err := newTempFolder("test-parse-single-stat")
	assert.Nil(t, err)
	defer tempFolder.removeAll()

	cgroup := newDummyContainerCgroup(tempFolder.RootPath, "cpu")

	// No file
	_, err = cgroup.ParseSingleStat("cpu", "notfound")
	assert.NotNil(t, err)
	assert.True(t, os.IsNotExist(err))

	// Several lines
	tempFolder.add("cpu/cpu.test", "1234\nbla")
	_, err = cgroup.ParseSingleStat("cpu", "cpu.test")
	assert.NotNil(t, err)
	t.Log(err)
	assert.Contains(t, err.Error(), "wrong file format")

	// Not int
	tempFolder.add("cpu/cpu.test", "1234bla")
	_, err = cgroup.ParseSingleStat("cpu", "cpu.test")
	assert.NotNil(t, err)
	t.Log(err)
	assert.Equal(t, err.Error(), "strconv.ParseUint: parsing \"1234bla\": invalid syntax")

	// Valid file
	tempFolder.add("cpu/cpu.test", "1234")
	value, err := cgroup.ParseSingleStat("cpu", "cpu.test")
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(1234))
}

func TestThreadLimit(t *testing.T) {
	tempFolder, err := newTempFolder("thread-limit")
	assert.Nil(t, err)
	defer tempFolder.removeAll()

	cgroup := newDummyContainerCgroup(tempFolder.RootPath, "pids")

	// No file
	value, err := cgroup.ThreadLimit()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(0))

	// Invalid file
	tempFolder.add("pids/pids.max", "ab")
	value, err = cgroup.ThreadLimit()
	assert.NotNil(t, err)
	assert.IsType(t, err, &strconv.NumError{})
	assert.Equal(t, value, uint64(0))

	// No limit
	tempFolder.add("pids/pids.max", "max")
	value, err = cgroup.ThreadLimit()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(0))

	// Valid value
	tempFolder.add("pids/pids.max", "1234")
	value, err = cgroup.ThreadLimit()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(1234))
}

func TestThreadCount(t *testing.T) {
	tempFolder, err := newTempFolder("thread-count")
	assert.Nil(t, err)
	defer tempFolder.removeAll()

	cgroup := newDummyContainerCgroup(tempFolder.RootPath, "pids")

	// No file
	value, err := cgroup.ThreadCount()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(0))

	// Invalid file
	tempFolder.add("pids/pids.current", "ab")
	value, err = cgroup.ThreadCount()
	assert.NotNil(t, err)
	assert.IsType(t, err, &strconv.NumError{})
	assert.Equal(t, value, uint64(0))

	// Valid value
	tempFolder.add("pids/pids.current", "123")
	value, err = cgroup.ThreadCount()
	assert.Nil(t, err)
	assert.Equal(t, value, uint64(123))
}
