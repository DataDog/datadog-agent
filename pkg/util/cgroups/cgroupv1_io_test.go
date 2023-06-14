// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

const (
	sampleIOEmpty        = `Total 0`
	sampleIOServiceBytes = `8:16 Read 585728
8:16 Write 0
8:16 Sync 0
8:16 Async 585728
8:16 Total 585728
8:48 Read 75057152
8:48 Write 1964221952
8:48 Sync 1964221952
8:48 Async 75057152
8:48 Total 2039279104
8:32 Read 4410880
8:32 Write 273678336
8:32 Sync 0
8:32 Async 278089216
8:32 Total 278089216
Total 2317954048`
	sampleIOServiced = `259:0 Read 38
259:0 Write 27528
259:0 Sync 27566
259:0 Async 0
259:0 Discard 0
259:0 Total 27566
Total 27566`
)

func createCgroupV1FakeIOFiles(cfs *cgroupMemoryFS, cg *cgroupV1) {
	cfs.setCgroupV1File(cg, "blkio", "blkio.throttle.io_service_bytes", sampleIOServiceBytes)
	cfs.setCgroupV1File(cg, "blkio", "blkio.throttle.io_serviced", sampleIOServiced)
}

func TestCgroupV1IOStats(t *testing.T) {
	cfs := newCgroupMemoryFS("/test/fs/cgroup")

	var err error
	stats := &IOStats{}

	// Test failure if controller missing (blkio is missing)
	tr.reset()
	cgFoo1 := cfs.createCgroupV1("foo1", containerCgroupKubePod(false))
	err = cgFoo1.GetIOStats(stats)
	assert.ErrorIs(t, err, &ControllerNotFoundError{Controller: "blkio"})

	// Test reading files in blkio controller, all files missing
	tr.reset()
	cfs.enableControllers("blkio")
	err = cgFoo1.GetIOStats(stats)
	assert.NoError(t, err)
	assert.Equal(t, len(tr.errors), 2)
	assert.Empty(t, cmp.Diff(IOStats{}, *stats))

	// Test reading files in blkio controller, all files present
	tr.reset()
	createCgroupV1FakeIOFiles(cfs, cgFoo1)
	err = cgFoo1.GetIOStats(stats)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []error{}, tr.errors)
	assert.Empty(t, cmp.Diff(IOStats{
		ReadBytes:       pointer.Ptr(uint64(80053760)),
		WriteBytes:      pointer.Ptr(uint64(2237900288)),
		ReadOperations:  pointer.Ptr(uint64(38)),
		WriteOperations: pointer.Ptr(uint64(27528)),
		Devices: map[string]DeviceIOStats{
			"8:16": {
				ReadBytes:  pointer.Ptr(uint64(585728)),
				WriteBytes: pointer.Ptr(uint64(0)),
			},
			"8:32": {
				ReadBytes:  pointer.Ptr(uint64(4410880)),
				WriteBytes: pointer.Ptr(uint64(273678336)),
			},
			"8:48": {
				ReadBytes:  pointer.Ptr(uint64(75057152)),
				WriteBytes: pointer.Ptr(uint64(1964221952)),
			},
			"259:0": {
				ReadOperations:  pointer.Ptr(uint64(38)),
				WriteOperations: pointer.Ptr(uint64(27528)),
			},
		},
	}, *stats))

	// Test reading files in blkio controller, empty file
	tr.reset()
	cfs.setCgroupV1File(cgFoo1, "blkio", "blkio.throttle.io_service_bytes", sampleIOEmpty)
	cfs.setCgroupV1File(cgFoo1, "blkio", "blkio.throttle.io_serviced", sampleIOEmpty)
	stats = &IOStats{}
	err = cgFoo1.GetIOStats(stats)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []error{}, tr.errors)
	assert.Empty(t, cmp.Diff(IOStats{
		ReadBytes:       pointer.Ptr(uint64(0)),
		WriteBytes:      pointer.Ptr(uint64(0)),
		ReadOperations:  pointer.Ptr(uint64(0)),
		WriteOperations: pointer.Ptr(uint64(0)),
		Devices:         nil,
	}, *stats))
}
