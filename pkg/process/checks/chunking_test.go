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

type testPayload struct {
	id     int
	weight int
}

type testPayloadList struct {
	payloads []*testPayload
	chunker  testPayloadChunker
}

type testPayloadChunker interface {
	Accept(payloads []*testPayload, weight int)
}

func (l *testPayloadList) Len() int {
	return len(l.payloads)
}

func (l *testPayloadList) WeightAt(idx int) int {
	if idx >= len(l.payloads) {
		return 0
	}
	return l.payloads[idx].weight
}

func (l *testPayloadList) ToChunk(start, end int, weight int) {
	l.chunker.Accept(l.payloads[start:end], weight)
}

type testChunker struct {
	chunkPropsTracker
	chunks [][]*testPayload
}

func (c *testChunker) Accept(payloads []*testPayload, weight int) {
	if c.idx >= len(c.chunks) {
		c.chunks = append(c.chunks, []*testPayload{})
		c.props = append(c.props, chunkProps{})
	}

	c.chunks[c.idx] = append(c.chunks[c.idx], payloads...)
	c.props[c.idx].size += len(payloads)
	c.props[c.idx].weight += weight
}

type chunkGroup struct {
	weights []int
	start   int
}

type chunkTest struct {
	maxChunkWeight int
	maxChunkSize   int
	groups         []chunkGroup

	expectedIDs [][]int
}

func (ct *chunkTest) runGroup(id int, chunker *testChunker, g chunkGroup) {
	payloads := make([]*testPayload, len(g.weights))
	for i := range payloads {
		payloads[i] = &testPayload{
			id:     id,
			weight: g.weights[i],
		}
		id++
	}

	list := &testPayloadList{
		payloads: payloads,
		chunker:  chunker,
	}

	chunker.idx = g.start
	chunkPayloadsBySizeAndWeight(list, chunker, ct.maxChunkSize, ct.maxChunkWeight)
}

func (ct *chunkTest) run(t *testing.T) {
	t.Helper()
	chunker := &testChunker{}

	id := 1
	for _, g := range ct.groups {
		ct.runGroup(id, chunker, g)
		id += len(g.weights)
	}
	actualIDs := make([][]int, len(chunker.chunks))
	for i := range chunker.chunks {
		for _, p := range chunker.chunks[i] {
			actualIDs[i] = append(actualIDs[i], p.id)
		}
	}

	assert.Equal(t, ct.expectedIDs, actualIDs)
}

func TestChunkPayloadsBySizeAndWeightSingleRun(t *testing.T) {
	tests := []chunkTest{
		{
			maxChunkWeight: 4,
			maxChunkSize:   3,
			groups: []chunkGroup{
				{
					weights: []int{
						5,
						// chop!
						1,
						1,
						1,
						// chop!
						1,
					},
				},
			},
			expectedIDs: [][]int{
				{
					1,
				},
				{
					2, 3, 4,
				},
				{
					5,
				},
			},
		},
		{
			maxChunkWeight: 4,
			maxChunkSize:   3,
			groups: []chunkGroup{
				{
					weights: []int{
						1,
						3,
						// chop!
						1,
						1,
						1,
						1,
						// chop!
						5,
						// chop!
						2,
					},
				},
			},
			expectedIDs: [][]int{
				{
					1, 2,
				},
				{
					3, 4, 5,
				},
				{
					6,
				},
				{
					7,
				},
				{
					8,
				},
			},
		},
		{
			maxChunkWeight: 4,
			maxChunkSize:   3,
			groups: []chunkGroup{
				{
					weights: []int{
						2,
						// chop!
						3,
						1,
						// chop!
						5,
						// chop!
						1,
						1,
						1,
						// chop!
						1,
						2,
						// chop!
						4,
					},
				},
			},

			expectedIDs: [][]int{
				{
					1,
				},
				{
					2, 3,
				},
				{
					4,
				},
				{
					5, 6, 7,
				},
				{
					8, 9,
				},
				{
					10,
				},
			},
		},
	}

	for n, tc := range tests {
		t.Run(fmt.Sprintf("case-%d", n), func(t *testing.T) {
			tc.run(t)
		})
	}
}

func TestChunkPayloadsBySizeAndWeightMultipleRuns(t *testing.T) {
	tests := []chunkTest{
		{
			maxChunkWeight: 4,
			maxChunkSize:   3,
			groups: []chunkGroup{
				{
					weights: []int{
						1, // id = 1
						1,
						1,
						1,
						1,
					},
				},
				{
					start: 0,
					weights: []int{
						2, // id = 6
						1,
						1,
					},
				},
			},
			expectedIDs: [][]int{
				{
					1, 2, 3,
				},
				{
					4, 5, 6,
				},
				{
					7, 8,
				},
			},
		},
		{
			maxChunkWeight: 4,
			maxChunkSize:   3,
			groups: []chunkGroup{
				{
					weights: []int{
						3, // id = 1
						2,
						1,
						1,
						1,
					},
				},
				{
					start: 0,
					weights: []int{
						1, // id = 6
						1,
						1,
					},
				},
			},
			expectedIDs: [][]int{
				{
					1, 6,
				},
				{
					2, 3, 4,
				},
				{
					5, 7, 8,
				},
			},
		},
		{
			maxChunkWeight: 4,
			maxChunkSize:   3,
			groups: []chunkGroup{
				{
					weights: []int{
						1, // id = 1
						1,
						3,
						3,
						2,
					},
				},
				{
					start: 1,
					// first run should have [id=1 (w=1), id=2 (w=1)] [id=3 (w=3)] [id=4 (w=3)] [id=5 (w=2)]
					weights: []int{
						1, // id = 6 (should fit in chunk at 1, we start filling at 1)
						2,
						1,
					},
				},
			},
			expectedIDs: [][]int{
				{
					1, 2,
				},
				{
					3, 6,
				},
				{
					4,
				},
				{
					5, 7,
				},
				{
					8,
				},
			},
		},
		{
			maxChunkWeight: 4,
			maxChunkSize:   3,
			groups: []chunkGroup{
				{
					weights: []int{
						1, // id = 1
						1,
						3,
						3,
						2,
					},
				},
				{
					start: 2, // first run should have [id=1 (w=1), id=2 (w=1)] [id=3 (w=3)] [id=4 (w=3)] [id=5 (w=2)]
					weights: []int{
						1, // id = 6 (should fit in chunk at 2, we start filling at 2)
						2,
						1,
					},
				},
			},
			expectedIDs: [][]int{
				{
					1, 2,
				},
				{
					3,
				},
				{
					4, 6,
				},
				{
					5, 7,
				},
				{
					8,
				},
			},
		},
		{
			maxChunkWeight: 4,
			maxChunkSize:   3,
			groups: []chunkGroup{
				{
					weights: []int{
						1, // id = 1
						1,
						3,
						4,
						1,
					},
				},
				{
					start: 2, // first run should have (1, 1) (3) (4) (1)
					weights: []int{
						4, // id = 6 (should result in append - at max chunk weight)
						2,
						1,
					},
				},
			},
			expectedIDs: [][]int{
				{
					1, 2,
				},
				{
					3,
				},
				{
					4,
				},
				{
					5,
				},
				{
					6,
				},
				{
					7, 8,
				},
			},
		},
	}

	for n, tc := range tests {
		t.Run(fmt.Sprintf("case-%d", n), func(t *testing.T) {
			tc.run(t)
		})
	}
}

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
		expectedChunks []*model.CollectorProc
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
			expectedChunks: []*model.CollectorProc{
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
					proc(2, "foo", strings.Repeat("-", 600)),
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
			expectedChunks: []*model.CollectorProc{
				{
					Containers: []*model.Container{
						ctr("foo"),
					},
					Processes: []*model.Process{
						proc(100, "top"),
						proc(2, "foo", strings.Repeat("-", 600)),
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
			expectedChunks: []*model.CollectorProc{
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chunks, totalProcs, totalContainers := chunkProcessesAndContainers(tc.procsByCtr, tc.containers, tc.maxChunkSize, tc.maxChunkWeight)
			assert.Equal(t, tc.expectedChunks, chunks)
			expectedProcs := 0

			for _, procs := range tc.procsByCtr {
				expectedProcs += len(procs)
			}
			assert.Equal(t, expectedProcs, totalProcs)
			assert.Equal(t, len(tc.containers), totalContainers)
		})
	}
}
