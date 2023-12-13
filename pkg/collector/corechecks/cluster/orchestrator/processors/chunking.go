// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package processors

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// chunkOrchestratorPayloadsBySizeAndWeight chunks orchestrator payloads by max allowed size and max allowed weight of a chunk
// We use yaml size as payload weight
func chunkOrchestratorPayloadsBySizeAndWeight(orchestratorPayloads []interface{}, orchestratorYaml []interface{}, maxChunkSize, maxChunkWeight int) [][]interface{} {
	if len(orchestratorPayloads) == 0 {
		return make([][]interface{}, 0)
	}

	chunker := &util.ChunkAllocator[[]interface{}, interface{}]{
		AppendToChunk: func(chunk *[]interface{}, payloads []interface{}) {
			*chunk = append(*chunk, payloads...)
		},
	}

	list := &util.PayloadList[interface{}]{
		Items: orchestratorPayloads,
		WeightAt: func(i int) int {
			if i >= len(orchestratorYaml) {
				return 0
			}
			return len(orchestratorYaml[i].(*model.Manifest).Content)
		},
	}

	util.ChunkPayloadsBySizeAndWeight[[]interface{}, interface{}](list, chunker, maxChunkSize, maxChunkWeight)

	return *chunker.GetChunks()
}
