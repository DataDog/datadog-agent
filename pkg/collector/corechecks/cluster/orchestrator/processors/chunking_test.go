// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build orchestrator

package processors

import (
	"testing"

	"github.com/stretchr/testify/assert"

	model "github.com/DataDog/agent-payload/v5/process"
)

func TestChunkOrchestratorMetadataBySizeAndWeight(t *testing.T) {
	type Item struct {
		UID string
	}

	// orchestratorResources UID slice order match the orchestratorYaml slice order
	orchestratorResources := []interface{}{
		Item{UID: "1"},
		Item{UID: "2"},
		Item{UID: "3"},
		Item{UID: "4"},
		Item{UID: "5"},
	}
	tests := []struct {
		name                  string
		maxChunkSize          int
		maxChunkWeight        int
		orchestratorResources []interface{}
		orchestratorYaml      []interface{}
		expectedChunks        [][]interface{}
	}{
		{
			name:                  "chunk by size and weight, one high weight",
			maxChunkSize:          3,
			maxChunkWeight:        1000,
			orchestratorResources: orchestratorResources,
			orchestratorYaml: []interface{}{
				&model.Manifest{
					Uid:     "1",
					Content: make([]byte, 1001),
				},
				&model.Manifest{
					Uid:     "2",
					Content: make([]byte, 100),
				},
				&model.Manifest{
					Uid:     "3",
					Content: make([]byte, 100),
				},
				&model.Manifest{
					Uid:     "4",
					Content: make([]byte, 100),
				},
				&model.Manifest{
					Uid:     "5",
					Content: make([]byte, 100),
				},
			},
			// UID 1 is over 1000 and therefore gets its own slice, while 2,3,4 are getting into one due to the maxSize
			expectedChunks: [][]interface{}{
				{Item{UID: "1"}},
				{Item{UID: "2"}, Item{UID: "3"}, Item{UID: "4"}},
				{Item{UID: "5"}},
			},
		},
		{
			name:                  "chunk by size and weight, weight exceeded",
			maxChunkSize:          3,
			maxChunkWeight:        1000,
			orchestratorResources: orchestratorResources,
			orchestratorYaml: []interface{}{
				&model.Manifest{
					Uid:     "1",
					Content: make([]byte, 2000),
				},
				&model.Manifest{
					Uid:     "2",
					Content: make([]byte, 2000),
				},
				&model.Manifest{
					Uid:     "3",
					Content: make([]byte, 2000),
				},
				&model.Manifest{
					Uid:     "4",
					Content: make([]byte, 2000),
				},
				&model.Manifest{
					Uid:     "5",
					Content: make([]byte, 2000),
				},
			},
			// Each of the items is over 1000 and therefore get its own slice
			expectedChunks: [][]interface{}{
				{Item{UID: "1"}},
				{Item{UID: "2"}},
				{Item{UID: "3"}},
				{Item{UID: "4"}},
				{Item{UID: "5"}},
			},
		},
		{
			name:                  "chunk by size and weight, low weight",
			maxChunkSize:          3,
			maxChunkWeight:        1000,
			orchestratorResources: orchestratorResources,
			orchestratorYaml: []interface{}{
				&model.Manifest{
					Uid:     "1",
					Content: make([]byte, 100),
				},
				&model.Manifest{
					Uid:     "2",
					Content: make([]byte, 100),
				},
				&model.Manifest{
					Uid:     "3",
					Content: make([]byte, 100),
				},
				&model.Manifest{
					Uid:     "4",
					Content: make([]byte, 100),
				},
				&model.Manifest{
					Uid:     "5",
					Content: make([]byte, 100),
				},
			},
			// UID 1,2,3 get into one slice due to maxChunkSize as the sum of their wight is below 1000
			expectedChunks: [][]interface{}{
				{Item{UID: "1"}, Item{UID: "2"}, Item{UID: "3"}},
				{Item{UID: "4"}, Item{UID: "5"}},
			},
		},
		{
			name:                  "chunk by size and weight, mixed",
			maxChunkSize:          3,
			maxChunkWeight:        1000,
			orchestratorResources: orchestratorResources,
			orchestratorYaml: []interface{}{
				&model.Manifest{
					Uid:     "1",
					Content: make([]byte, 200),
				},
				&model.Manifest{
					Uid:     "2",
					Content: make([]byte, 400),
				},
				&model.Manifest{
					Uid:     "3",
					Content: make([]byte, 800),
				},
				&model.Manifest{
					Uid:     "4",
					Content: make([]byte, 300),
				},
				&model.Manifest{
					Uid:     "5",
					Content: make([]byte, 2000),
				},
			},
			// UID 1,2 get into one slice because adding UID 3 can make wight over 1000. Same reason for UID 4 and 5
			expectedChunks: [][]interface{}{
				{Item{UID: "1"}, Item{UID: "2"}},
				{Item{UID: "3"}},
				{Item{UID: "4"}},
				{Item{UID: "5"}},
			},
		},
		{
			name:                  "chunk by size and weight, include limit itself",
			maxChunkSize:          3,
			maxChunkWeight:        1000,
			orchestratorResources: orchestratorResources,
			orchestratorYaml: []interface{}{
				&model.Manifest{
					Uid:     "1",
					Content: make([]byte, 500),
				},
				&model.Manifest{
					Uid:     "2",
					Content: make([]byte, 300),
				},
				&model.Manifest{
					Uid:     "3",
					Content: make([]byte, 200),
				},
				&model.Manifest{
					Uid:     "4",
					Content: make([]byte, 500),
				},
				&model.Manifest{
					Uid:     "5",
					Content: make([]byte, 500),
				},
			},
			// UID 1,2,3 get into one slice as their wight is equal to 1000. Same reason for UID 4 and 5
			expectedChunks: [][]interface{}{
				{Item{UID: "1"}, Item{UID: "2"}, Item{UID: "3"}},
				{Item{UID: "4"}, Item{UID: "5"}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chunks := chunkOrchestratorPayloadsBySizeAndWeight(tc.orchestratorResources, tc.orchestratorYaml, tc.maxChunkSize, tc.maxChunkWeight)
			assert.Equal(t, tc.expectedChunks, chunks)
		})
	}
}
