// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package cgroups

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

const (
	sampleCgroupV2IOStat = `259:0 rbytes=278528 wbytes=11623899136 rios=6 wios=2744940 dbytes=0 dios=0
8:16 rbytes=278528 wbytes=11623899136 rios=6 wios=2744940 dbytes=0 dios=0`
	sampleCgroupV2IOMax     = "8:16 rbps=2097152 wbps=max riops=max wiops=120"
	sampleCroupV2IOPressure = `some avg10=0.00 avg60=0.00 avg300=0.00 total=0
full avg10=0.00 avg60=0.00 avg300=0.00 total=0`
)

func createCgroupV2FakeIOFiles(cfs *cgroupMemoryFS, cg *cgroupV2) {
	cfs.setCgroupV2File(cg, "io.stat", sampleCgroupV2IOStat)
	cfs.setCgroupV2File(cg, "io.max", sampleCgroupV2IOMax)
	cfs.setCgroupV2File(cg, "io.pressure", sampleCroupV2IOPressure)
}

func TestCgroupV2IOStats(t *testing.T) {
	cfs := newCgroupMemoryFS("/test/fs/cgroup")

	var err error
	stats := &IOStats{}

	// Test failure if controller missing (io is missing)
	tr.reset()
	cgFoo1 := cfs.createCgroupV2("foo1", containerCgroupKubePod(true))
	err = cgFoo1.GetIOStats(stats)
	assert.ErrorIs(t, err, &ControllerNotFoundError{Controller: "io"})

	// Test reading files in io controller, all files missing
	tr.reset()
	cfs.enableControllers("io")
	err = cgFoo1.GetIOStats(stats)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(tr.errors))
	assert.Empty(t, cmp.Diff(IOStats{}, *stats))

	// Test reading files in io controller, all files present
	tr.reset()
	createCgroupV2FakeIOFiles(cfs, cgFoo1)
	err = cgFoo1.GetIOStats(stats)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []error{}, tr.errors)
	assert.Empty(t, cmp.Diff(IOStats{
		ReadBytes:       uint64Ptr(557056),
		WriteBytes:      uint64Ptr(23247798272),
		ReadOperations:  uint64Ptr(12),
		WriteOperations: uint64Ptr(5489880),
		Devices: map[string]DeviceIOStats{
			"259:0": {
				ReadBytes:       uint64Ptr(278528),
				WriteBytes:      uint64Ptr(11623899136),
				ReadOperations:  uint64Ptr(6),
				WriteOperations: uint64Ptr(2744940),
			},
			"8:16": {
				ReadBytes:            uint64Ptr(278528),
				WriteBytes:           uint64Ptr(11623899136),
				ReadOperations:       uint64Ptr(6),
				WriteOperations:      uint64Ptr(2744940),
				ReadBytesLimit:       uint64Ptr(2097152),
				WriteOperationsLimit: uint64Ptr(120),
			},
		},
		PSISome: PSIStats{
			Avg10:  float64Ptr(0),
			Avg60:  float64Ptr(0),
			Avg300: float64Ptr(0),
			Total:  uint64Ptr(0),
		},
		PSIFull: PSIStats{
			Avg10:  float64Ptr(0),
			Avg60:  float64Ptr(0),
			Avg300: float64Ptr(0),
			Total:  uint64Ptr(0),
		},
	}, *stats))

	// Test reading empty file
	tr.reset()
	stats = &IOStats{}
	cfs.setCgroupV2File(cgFoo1, "io.stat", "")
	cfs.setCgroupV2File(cgFoo1, "io.max", "")
	err = cgFoo1.GetIOStats(stats)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []error{}, tr.errors)
	assert.Empty(t, cmp.Diff(IOStats{
		PSISome: PSIStats{
			Avg10:  float64Ptr(0),
			Avg60:  float64Ptr(0),
			Avg300: float64Ptr(0),
			Total:  uint64Ptr(0),
		},
		PSIFull: PSIStats{
			Avg10:  float64Ptr(0),
			Avg60:  float64Ptr(0),
			Avg300: float64Ptr(0),
			Total:  uint64Ptr(0),
		},
	}, *stats))
}
