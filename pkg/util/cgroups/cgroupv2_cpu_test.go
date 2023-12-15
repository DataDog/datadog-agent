// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

const (
	sampleCgroupV2CpuStat = `usage_usec 5616647428
user_usec 3569637760
system_usec 2047009667
nr_periods 0
nr_throttled 0
throttled_usec 0`
	sampleCgroupV2CpuWeight       = "16"
	sampleCgroupV2CpuMax          = "40000 100000"
	sampleCgroupV2CpuPressure     = "some avg10=42.64 avg60=43.72 avg300=25.76 total=114289003"
	sampleCgroupV2CpuSetEffective = "0-3"
)

func createCgroupV2FakeCPUFiles(cfs *cgroupMemoryFS, cg *cgroupV2) {
	cfs.setCgroupV2File(cg, "cpu.stat", sampleCgroupV2CpuStat)
	cfs.setCgroupV2File(cg, "cpu.weight", sampleCgroupV2CpuWeight)
	cfs.setCgroupV2File(cg, "cpu.max", sampleCgroupV2CpuMax)
	cfs.setCgroupV2File(cg, "cpu.pressure", sampleCgroupV2CpuPressure)
	cfs.setCgroupV2File(cg, "cpuset.cpus.effective", sampleCgroupV2CpuSetEffective)
}

func TestCgroupV2CPUStats(t *testing.T) {
	cfs := newCgroupMemoryFS("/test/fs/cgroup")

	var err error
	stats := &CPUStats{}

	// Test failure if controller missing (cpu is missing)
	tr.reset()
	cgFoo1 := cfs.createCgroupV2("foo1", containerCgroupKubePod(true))
	err = cgFoo1.GetCPUStats(stats)
	assert.ErrorIs(t, err, &ControllerNotFoundError{Controller: "cpu"})

	// Test reading files in CPU controllers, all files missing
	tr.reset()
	cfs.enableControllers("cpu", "cpuset")
	err = cgFoo1.GetCPUStats(stats)
	assert.NoError(t, err)
	assert.Equal(t, len(tr.errors), 5)
	assert.Empty(t, cmp.Diff(CPUStats{}, *stats))

	// Test reading files in CPU controllers, all files present
	tr.reset()
	createCgroupV2FakeCPUFiles(cfs, cgFoo1)
	err = cgFoo1.GetCPUStats(stats)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []error{}, tr.errors)
	assert.Empty(t, cmp.Diff(CPUStats{
		User:             pointer.Ptr(3569637760 * uint64(time.Microsecond)),
		System:           pointer.Ptr(2047009667 * uint64(time.Microsecond)),
		Total:            pointer.Ptr(5616647428 * uint64(time.Microsecond)),
		Weight:           pointer.Ptr(uint64(16)),
		ElapsedPeriods:   pointer.Ptr(uint64(0)),
		ThrottledPeriods: pointer.Ptr(uint64(0)),
		ThrottledTime:    pointer.Ptr(uint64(0)),
		SchedulerPeriod:  pointer.Ptr(100000 * uint64(time.Microsecond)),
		SchedulerQuota:   pointer.Ptr(40000 * uint64(time.Microsecond)),
		CPUCount:         pointer.Ptr(uint64(4)),
		PSISome: PSIStats{
			Avg10:  pointer.Ptr(42.64),
			Avg60:  pointer.Ptr(43.72),
			Avg300: pointer.Ptr(25.76),
			Total:  pointer.Ptr(uint64(114289003)),
		},
	}, *stats))

	// Test reading files in CPU controllers, all files present except 1 (cpu.shares)
	tr.reset()
	cfs.deleteCgroupV2File(cgFoo1, "cpuset.cpus.effective")
	stats = &CPUStats{}
	err = cgFoo1.GetCPUStats(stats)
	assert.NoError(t, err)
	assert.Equal(t, len(tr.errors), 1)
	assert.Empty(t, cmp.Diff(CPUStats{
		User:             pointer.Ptr(3569637760 * uint64(time.Microsecond)),
		System:           pointer.Ptr(2047009667 * uint64(time.Microsecond)),
		Total:            pointer.Ptr(5616647428 * uint64(time.Microsecond)),
		Weight:           pointer.Ptr(uint64(16)),
		ElapsedPeriods:   pointer.Ptr(uint64(0)),
		ThrottledPeriods: pointer.Ptr(uint64(0)),
		ThrottledTime:    pointer.Ptr(uint64(0)),
		SchedulerPeriod:  pointer.Ptr(100000 * uint64(time.Microsecond)),
		SchedulerQuota:   pointer.Ptr(40000 * uint64(time.Microsecond)),
		PSISome: PSIStats{
			Avg10:  pointer.Ptr(42.64),
			Avg60:  pointer.Ptr(43.72),
			Avg300: pointer.Ptr(25.76),
			Total:  pointer.Ptr(uint64(114289003)),
		},
	}, *stats))
}

func TestParseV2CPUStat(t *testing.T) {
	tests := []struct {
		name     string
		inputKey string
		inputVal string
		want     *CPUStats
		wantErr  error
	}{
		{
			name:     "simple test",
			inputKey: "usage_usec",
			inputVal: "100",
			want: &CPUStats{
				Total: pointer.Ptr(uint64(100000)),
			},
			wantErr: nil,
		},
		{
			name:     "unknown key",
			inputKey: "usage_usec_sec_sec",
			inputVal: "100",
			want:     &CPUStats{},
			wantErr:  nil,
		},
		{
			name:     "impossible to parse value",
			inputKey: "usage_usec",
			inputVal: "100foobar",
			want:     &CPUStats{},
			wantErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := CPUStats{}
			err := parseV2CPUStat(&stats)(tt.inputKey, tt.inputVal)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, cmp.Diff(tt.want, &stats))
		})
	}
}

func TestParseV2CPUMax(t *testing.T) {
	tests := []struct {
		name     string
		inputKey string
		inputVal string
		want     *CPUStats
		wantErr  error
	}{
		{
			name:     "simple test",
			inputKey: "max",
			inputVal: "100",
			want: &CPUStats{
				SchedulerPeriod: pointer.Ptr(uint64(100000)),
			},
			wantErr: nil,
		},
		{
			name:     "wrong scheduler period",
			inputKey: "100",
			inputVal: "foobar",
			want:     &CPUStats{},
			wantErr:  strconv.ErrSyntax,
		},
		{
			name:     "impossible to parse quota",
			inputKey: "foo",
			inputVal: "1000",
			want: &CPUStats{
				SchedulerPeriod: pointer.Ptr(uint64(1000000)),
			},
			wantErr: strconv.ErrSyntax,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := CPUStats{}
			err := parseV2CPUMax(&stats)(tt.inputKey, tt.inputVal)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, cmp.Diff(tt.want, &stats))
		})
	}
}
