// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testPayload struct {
	id     int
	weight int
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

func (ct *chunkTest) runGroup(id int, chunker *ChunkAllocator[[]*testPayload, *testPayload], g chunkGroup) {
	payloads := make([]*testPayload, len(g.weights))
	for i := range payloads {
		payloads[i] = &testPayload{
			id:     id,
			weight: g.weights[i],
		}
		id++
	}

	list := &PayloadList[*testPayload]{
		Items: payloads,
		WeightAt: func(i int) int {
			return payloads[i].weight
		},
	}

	chunker.SetActiveChunk(g.start)
	ChunkPayloadsBySizeAndWeight(list, chunker, ct.maxChunkSize, ct.maxChunkWeight)
}

func (ct *chunkTest) run(t *testing.T) {
	t.Helper()
	chunker := &ChunkAllocator[[]*testPayload, *testPayload]{
		AppendToChunk: func(c *[]*testPayload, ps []*testPayload) {
			*c = append(*c, ps...)
		},
	}

	id := 1
	for _, g := range ct.groups {
		ct.runGroup(id, chunker, g)
		id += len(g.weights)
	}
	actualIDs := make([][]int, len(chunker.chunks))
	for i := range *chunker.GetChunks() {
		for _, p := range (*chunker.GetChunks())[i] {
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
						// chop!
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
