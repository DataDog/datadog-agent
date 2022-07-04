// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

const (
	cacheValidityRT = 500 * time.Millisecond
)

// RTContainer is a singleton RTContainerCheck.
var RTContainer = &RTContainerCheck{}

// RTContainerCheck collects numeric statistics about live ctrList.
type RTContainerCheck struct {
	maxBatchSize      int
	sysInfo           *model.SystemInfo
	containerProvider util.ContainerProvider
	lastRates         map[string]*util.ContainerRateMetrics
}

// Init initializes a RTContainerCheck instance.
func (r *RTContainerCheck) Init(_ *config.AgentConfig, sysInfo *model.SystemInfo) {
	r.maxBatchSize = getMaxBatchSize()
	r.sysInfo = sysInfo
	r.containerProvider = util.GetSharedContainerProvider()
}

// Name returns the name of the RTContainerCheck.
func (r *RTContainerCheck) Name() string { return config.RTContainerCheckName }

// RealTime indicates if this check only runs in real-time mode.
func (r *RTContainerCheck) RealTime() bool { return true }

// Run runs the real-time container check getting container-level stats from the Cgroups and Docker APIs.
func (r *RTContainerCheck) Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
	var err error
	var containers []*model.Container
	var lastRates map[string]*util.ContainerRateMetrics
	containers, lastRates, _, err = r.containerProvider.GetContainers(cacheValidityRT, r.lastRates)
	if err == nil {
		r.lastRates = lastRates
	} else {
		log.Debugf("Unable to gather stats for containers, err: %v", err)
	}

	if len(containers) == 0 {
		log.Trace("No containers found")
		return nil, nil
	}

	groupSize := len(containers) / r.maxBatchSize
	if len(containers)%r.maxBatchSize != 0 {
		groupSize++
	}
	chunked := convertAndChunkContainers(containers, groupSize)
	messages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorContainerRealTime{
			HostName:          cfg.HostName,
			Stats:             chunked[i],
			NumCpus:           int32(system.HostCPUCount()),
			TotalMemory:       r.sysInfo.TotalMemory,
			GroupId:           groupID,
			GroupSize:         int32(groupSize),
			ContainerHostType: cfg.ContainerHostType,
		})
	}

	return messages, nil
}

// Cleanup frees any resource held by the RTContainerCheck before the agent exits
func (r *RTContainerCheck) Cleanup() {}

func convertAndChunkContainers(containers []*model.Container, chunks int) [][]*model.ContainerStat {
	perChunk := (len(containers) / chunks) + 1
	chunked := make([][]*model.ContainerStat, chunks)
	chunk := make([]*model.ContainerStat, 0, perChunk)
	chunkIdx := 0

	for _, ctr := range containers {
		chunk = append(chunk, convertToContainerStat(ctr))
		if len(chunk) == perChunk {
			chunked[chunkIdx] = chunk
			chunkIdx++
			chunk = make([]*model.ContainerStat, 0, perChunk)
		}
	}
	if len(chunk) > 0 {
		chunked[chunkIdx] = chunk
	}
	return chunked
}

func convertToContainerStat(container *model.Container) *model.ContainerStat {
	return &model.ContainerStat{
		Id:          container.Id,
		UserPct:     container.UserPct,
		SystemPct:   container.SystemPct,
		TotalPct:    container.TotalPct,
		CpuLimit:    container.CpuLimit,
		MemRss:      container.MemRss,
		MemCache:    container.MemCache,
		MemLimit:    container.MemoryLimit,
		Rbps:        container.Rbps,
		Wbps:        container.Wbps,
		NetRcvdPs:   container.NetRcvdPs,
		NetSentPs:   container.NetSentPs,
		NetRcvdBps:  container.NetRcvdBps,
		NetSentBps:  container.NetSentBps,
		State:       container.State,
		Health:      container.Health,
		Started:     container.Started,
		ThreadCount: container.ThreadCount,
		ThreadLimit: container.ThreadLimit,
	}
}
