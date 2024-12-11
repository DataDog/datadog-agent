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
	samplePidsCurrent      = "12"
	samplePidsMaxUnlimited = "max"
	samplePidsMax          = "42"
)

func createCgroupV1FakePIDFiles(cfs *cgroupMemoryFS, cg *cgroupV1) {
	cfs.setCgroupV1File(cg, "pids", "pids.current", samplePidsCurrent)
	cfs.setCgroupV1File(cg, "pids", "pids.max", samplePidsMax)
}

func TestCgroupV1PIDStats(t *testing.T) {
	cfs := newCgroupMemoryFS("/test/fs/cgroup")

	var err error
	stats := &PIDStats{}

	// Test failure if controller missing (pids is missing)
	tr.reset()
	cgFoo1 := cfs.createCgroupV1("foo1", containerCgroupKubePod(false))
	err = cgFoo1.GetPIDStats(stats)
	assert.ErrorIs(t, err, &ControllerNotFoundError{Controller: "pids"})

	// Test reading files in pids controller, all files missing
	tr.reset()
	cfs.enableControllers("pids")
	err = cgFoo1.GetPIDStats(stats)
	assert.NoError(t, err)
	assert.Equal(t, len(tr.errors), 2)
	assert.Equal(t, "", cmp.Diff(PIDStats{}, *stats))

	// Test reading files in pids controller, all files present
	tr.reset()
	createCgroupV1FakePIDFiles(cfs, cgFoo1)
	err = cgFoo1.GetPIDStats(stats)
	assert.NoError(t, err)
	assert.Equal(t, "", cmp.Diff(PIDStats{
		HierarchicalThreadCount: pointer.Ptr(uint64(12)),
		HierarchicalThreadLimit: pointer.Ptr(uint64(42)),
	}, *stats))

	// Test reading pids.max string value (max)
	tr.reset()
	cfs.setCgroupV1File(cgFoo1, "pids", "pids.max", samplePidsMaxUnlimited)
	stats = &PIDStats{}
	err = cgFoo1.GetPIDStats(stats)
	assert.NoError(t, err)
	assert.Equal(t, "", cmp.Diff(PIDStats{
		HierarchicalThreadCount: pointer.Ptr(uint64(12)),
		HierarchicalThreadLimit: nil,
	}, *stats))
}
