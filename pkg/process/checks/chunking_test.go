// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"fmt"
	"strings"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
)

func TestChunkProcessesBySizeAndWeight(t *testing.T) {
	ctr := func(id string) *model.Container {
		return &model.Container{
			Id: id,
		}
	}
	proc := func(pid int32, exe string, args ...string) *model.Process {
		return &model.Process{
			Pid: pid,
			Command: &model.Command{
				Exe:  exe,
				Args: args,
			},
		}
	}

	tests := []struct {
		name           string
		containers     []*model.Container
		procsByCtr     map[string][]*model.Process
		maxChunkSize   int
		maxChunkWeight int
		expectedChunks []model.CollectorProc
	}{
		{
			name: "chunk by size only",
			containers: []*model.Container{
				ctr("foo"),
				ctr("bar"),
				ctr("baz"),
			},
			procsByCtr: map[string][]*model.Process{
				"foo": {
					proc(1, "foo"),
					proc(2, "foo2"),
				},
				"bar": {
					proc(3, "bar"),
					proc(4, "bar2"),
				},
				"baz": {
					proc(5, "baz"),
				},
			},
			maxChunkSize:   3,
			maxChunkWeight: 1000,
			expectedChunks: []model.CollectorProc{
				{
					Containers: []*model.Container{
						ctr("foo"),
						ctr("bar"),
					},
					Processes: []*model.Process{
						proc(1, "foo"),
						proc(2, "foo2"),
						proc(3, "bar"),
					},
				},
				{
					Containers: []*model.Container{
						ctr("bar"),
						ctr("baz"),
					},
					Processes: []*model.Process{
						proc(4, "bar2"),
						proc(5, "baz"),
					},
				},
			},
		},
		{
			name: "chunk by size and weight",
			containers: []*model.Container{
				ctr("foo"),
				ctr("bar"),
				ctr("baz"),
			},
			procsByCtr: map[string][]*model.Process{
				"": {
					proc(100, "top"),
				},
				"foo": {
					proc(2, "foo", strings.Repeat("-", 970)),
				},
				"bar": {
					proc(3, "bar"),
					proc(4, "bar2"),
				},
				"baz": {
					proc(5, "baz"),
				},
			},
			maxChunkSize:   3,
			maxChunkWeight: 1000,
			expectedChunks: []model.CollectorProc{
				{
					Containers: []*model.Container{
						ctr("foo"),
					},
					Processes: []*model.Process{
						proc(100, "top"),
						proc(2, "foo", strings.Repeat("-", 970)),
					},
				},
				{
					Containers: []*model.Container{
						ctr("bar"),
						ctr("baz"),
					},
					Processes: []*model.Process{
						proc(3, "bar"),
						proc(4, "bar2"),
						proc(5, "baz"),
					},
				},
			},
		},
		{
			name: "chunk by size and weight excess",
			containers: []*model.Container{
				ctr("foo"),
				ctr("bar"),
				ctr("baz"),
			},
			procsByCtr: map[string][]*model.Process{
				"": {
					proc(100, "top"),
				},
				"foo": {
					proc(2, "foo", strings.Repeat("-", 1000)),
				},
				"bar": {
					proc(3, "bar"),
					proc(4, "bar2"),
				},
				"baz": {
					proc(5, "baz"),
				},
			},
			maxChunkSize:   3,
			maxChunkWeight: 1000,
			expectedChunks: []model.CollectorProc{
				{
					Processes: []*model.Process{
						proc(100, "top"),
					},
				},
				{
					Containers: []*model.Container{
						ctr("foo"),
					},
					Processes: []*model.Process{
						proc(2, "foo", strings.Repeat("-", 1000)),
					},
				},
				{
					Containers: []*model.Container{
						ctr("bar"),
						ctr("baz"),
					},
					Processes: []*model.Process{
						proc(3, "bar"),
						proc(4, "bar2"),
						proc(5, "baz"),
					},
				},
			},
		},
		{
			name: "containers with no processes",
			containers: []*model.Container{
				ctr("foo"),
				ctr("bar"),
				ctr("baz"),
				ctr("qux"),
			},
			procsByCtr: map[string][]*model.Process{
				"": {
					proc(100, "top"),
				},
				"foo": {
					proc(2, "foo", strings.Repeat("-", 970)),
				},
				"bar": {
					proc(3, "bar"),
					proc(4, "bar2"),
				},
			},
			maxChunkSize:   3,
			maxChunkWeight: 1000,
			expectedChunks: []model.CollectorProc{
				{
					Containers: []*model.Container{
						ctr("foo"),
					},
					Processes: []*model.Process{
						proc(100, "top"),
						proc(2, "foo", strings.Repeat("-", 970)),
					},
				},
				{
					Containers: []*model.Container{
						ctr("bar"),
						ctr("baz"),
						ctr("qux"),
					},
					Processes: []*model.Process{
						proc(3, "bar"),
						proc(4, "bar2"),
					},
				},
			},
		},
		{
			name: "first container with no processes",
			containers: []*model.Container{
				ctr("qux"),
				ctr("foo"),
			},
			procsByCtr: map[string][]*model.Process{
				"foo": {
					proc(2, "foo", strings.Repeat("-", 970)),
				},
			},
			maxChunkSize:   3,
			maxChunkWeight: 1000,
			expectedChunks: []model.CollectorProc{
				{
					Containers: []*model.Container{
						ctr("qux"),
						ctr("foo"),
					},
					Processes: []*model.Process{
						proc(2, "foo", strings.Repeat("-", 970)),
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chunks, totalProcs, totalContainers := chunkProcessesAndContainers(tc.procsByCtr, tc.containers, tc.maxChunkSize, tc.maxChunkWeight)
			assert.Equal(t, tc.expectedChunks, *chunks)

			expectedProcs := 0

			for _, procs := range tc.procsByCtr {
				expectedProcs += len(procs)
			}
			assert.Equal(t, expectedProcs, totalProcs)
			assert.Equal(t, len(tc.containers), totalContainers)
		})
	}
}

func testProcessWithStrings(size int) *model.Process {
	s := strings.Repeat("x", size)
	return &model.Process{
		User: &model.ProcessUser{
			Name: s,
		},
		Command: &model.Command{
			Args: []string{
				s,
				s,
				s,
				s,
				s,
			},
			Cwd:  s,
			Exe:  s,
			Root: s,
		},
		ContainerId: s,
	}
}

func TestWeightProcess(t *testing.T) {
	const (
		allowedPctDelta = 2.
		allowedMinDelta = 20
	)
	strSizes := []int{
		0, 10, 100, 1000, 10000, 100000,
	}

	for i := range strSizes {
		p := testProcessWithStrings(strSizes[i])
		actualWeight := weighProcess(p)
		t.Run(fmt.Sprintf("case %d weight %d", i, actualWeight), func(t *testing.T) {
			serialized, err := p.Marshal()
			assert.NoError(t, err)

			expectedWeight := len(serialized)
			assert.Equal(t, expectedWeight, p.Size())
			allowedDelta := int(float64(actualWeight) * allowedPctDelta / 100.)
			if allowedDelta < allowedMinDelta {
				allowedDelta = allowedMinDelta
			}

			withinLimits := expectedWeight-allowedDelta <= actualWeight && actualWeight <= expectedWeight+allowedDelta
			assert.True(
				t,
				withinLimits,
				"Expected weight to be within allowed delta (%d) of %d, actual %d",
				allowedDelta,
				expectedWeight,
				actualWeight,
			)
		})
	}
}
