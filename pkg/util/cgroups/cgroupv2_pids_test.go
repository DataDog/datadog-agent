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
	sampleCgroupV2PidsCurrent      = "12"
	sampleCgroupV2PidsMaxUnlimited = "max"
	sampleCgroupV2PidsMax          = "42"
)

func createCgroupV2FakePIDFiles(cfs *cgroupMemoryFS, cg *cgroupV2) {
	cfs.setCgroupV2File(cg, "pids.current", sampleCgroupV2PidsCurrent)
	cfs.setCgroupV2File(cg, "pids.max", sampleCgroupV2PidsMax)
}

func TestCgroupV2PIDStats(t *testing.T) {
	cfs := newCgroupMemoryFS("/test/fs/cgroup")

	var err error
	stats := &PIDStats{}

	// Test failure if controller missing (pids is missing)
	tr.reset()
	cgFoo1 := cfs.createCgroupV2("foo1", containerCgroupKubePod(true))
	err = cgFoo1.GetPIDStats(stats)
	assert.ErrorIs(t, err, &ControllerNotFoundError{Controller: "pids"})

	// Test reading files in pids controller, all files missing
	tr.reset()
	cfs.enableControllers("pids")
	err = cgFoo1.GetPIDStats(stats)
	assert.NoError(t, err)
	assert.Equal(t, len(tr.errors), 2)
	assert.Empty(t, cmp.Diff(PIDStats{}, *stats))

	// Test reading files in pids controller, all files present
	tr.reset()
	createCgroupV2FakePIDFiles(cfs, cgFoo1)
	err = cgFoo1.GetPIDStats(stats)
	assert.NoError(t, err)
	assert.Empty(t, cmp.Diff(PIDStats{
		HierarchicalThreadCount: pointer.Ptr(uint64(12)),
		HierarchicalThreadLimit: pointer.Ptr(uint64(42)),
	}, *stats))

	// Test reading pids.max string value (max)
	tr.reset()
	cfs.setCgroupV2File(cgFoo1, "pids.max", sampleCgroupV2PidsMaxUnlimited)
	stats = &PIDStats{}
	err = cgFoo1.GetPIDStats(stats)
	assert.NoError(t, err)
	assert.Equal(t, "", cmp.Diff(PIDStats{
		HierarchicalThreadCount: pointer.Ptr(uint64(12)),
		HierarchicalThreadLimit: nil,
	}, *stats))
}
