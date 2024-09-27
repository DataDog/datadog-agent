// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// chunkProcessesBySizeAndWeight chunks `model.Process` payloads by max allowed size and max allowed weight of a chunk
func chunkProcessesBySizeAndWeight(procs []*model.Process, ctr *model.Container, maxChunkSize, maxChunkWeight int, chunker *util.ChunkAllocator[model.CollectorProc, *model.Process]) {
	if ctr != nil && len(procs) == 0 {
		// can happen in three scenarios, and we still need to report the container
		// a) if a process is skipped (e.g. disallowlisted)
		// b) if process <=> container mapping cannot be established (e.g. Docker on Windows).
		// c) if no processes were collected from the container (e.g. pidMode not set to "task" on ECS Fargate)
		appendContainerWithoutProcesses(ctr, chunker)
		return
	}

	// Use the last available chunk as it may have some space for payloads
	chunker.SetLastChunk()

	// Processes that have a related container will add this container to every chunk they are split across
	// This may result in the same container being sent in multiple payloads from the agent
	// Note that this is necessary because container process tags (sent within `model.Container`) are only resolved from
	// containers seen within the same `model.ContainerProc` as processes.
	chunker.OnAccept = func(t *model.CollectorProc) {
		if ctr != nil {
			t.Containers = append(t.Containers, ctr)
		}
	}
	list := &util.PayloadList[*model.Process]{
		Items: procs,
		WeightAt: func(i int) int {
			return weighProcess(procs[i])
		},
	}
	util.ChunkPayloadsBySizeAndWeight[model.CollectorProc, *model.Process](list, chunker, maxChunkSize, maxChunkWeight)
}

func appendContainerWithoutProcesses(ctr *model.Container, chunker *util.ChunkAllocator[model.CollectorProc, *model.Process]) {
	collectorProcs := chunker.GetChunks()
	if len(*collectorProcs) == 0 {
		chunker.Accept([]*model.Process{}, 0)
	}
	collectorProc := &(*collectorProcs)[len(*collectorProcs)-1]
	collectorProc.Containers = append(collectorProc.Containers, ctr)
}

var (
	// procSizeofSampleProcess is a sample process used in sizeof/weight calculations
	procSizeofSampleProcess = &model.Process{
		Memory:   &model.MemoryStat{},
		Cpu:      &model.CPUStat{},
		IoStat:   &model.IOStat{},
		Networks: &model.ProcessNetworks{},
	}
	// procSizeofProto is a size of the empty process
	procSizeofProto = procSizeofSampleProcess.Size()
)

// weighProcess weighs `model.Process` payloads using an approximation of a serialized size of the proto message
func weighProcess(proc *model.Process) int {
	weight := procSizeofProto
	if proc.Command != nil {
		weight += len(proc.Command.Exe) + len(proc.Command.Cwd) + len(proc.Command.Root)
		for _, arg := range proc.Command.Args {
			weight += len(arg)
		}
	}
	if proc.User != nil {
		weight += len(proc.User.Name)
	}
	weight += len(proc.ContainerId)
	return weight
}
