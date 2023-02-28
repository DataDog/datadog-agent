// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

const (
	cacheValidityRT = 500 * time.Millisecond
)

// NewRTContainerCheck returns an instance of the RTContainerCheck.
func NewRTContainerCheck() *RTContainerCheck {
	return &RTContainerCheck{}
}

// RTContainerCheck collects numeric statistics about live ctrList.
type RTContainerCheck struct {
	maxBatchSize      int
	hostInfo          *HostInfo
	containerProvider util.ContainerProvider
	lastRates         map[string]*util.ContainerRateMetrics
}

// Init initializes a RTContainerCheck instance.
func (r *RTContainerCheck) Init(_ *SysProbeConfig, hostInfo *HostInfo) error {
	r.maxBatchSize = getMaxBatchSize()
	r.hostInfo = hostInfo
	r.containerProvider = util.GetSharedContainerProvider()
	return nil
}

// IsEnabled returns true if the check is enabled by configuration
func (r *RTContainerCheck) IsEnabled() bool {
	rtChecksEnabled := !ddconfig.Datadog.GetBool("process_config.disable_realtime_checks")
	return canEnableContainerChecks(ddconfig.Datadog, false) && rtChecksEnabled
}

// SupportsRunOptions returns true if the check supports RunOptions
func (r *RTContainerCheck) SupportsRunOptions() bool {
	return false
}

// Name returns the name of the RTContainerCheck.
func (r *RTContainerCheck) Name() string { return RTContainerCheckName }

// Realtime indicates if this check only runs in real-time mode.
func (r *RTContainerCheck) Realtime() bool { return true }

// ShouldSaveLastRun indicates if the output from the last run should be saved for use in flares
func (r *RTContainerCheck) ShouldSaveLastRun() bool { return true }

// Run runs the real-time container check getting container-level stats from the Cgroups and Docker APIs.
func (r *RTContainerCheck) Run(nextGroupID func() int32, _ *RunOptions) (RunResult, error) {
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
	groupID := nextGroupID()
	for i := 0; i < groupSize; i++ {
		messages = append(messages, &model.CollectorContainerRealTime{
			HostName:          r.hostInfo.HostName,
			Stats:             chunked[i],
			NumCpus:           int32(system.HostCPUCount()),
			TotalMemory:       r.hostInfo.SystemInfo.TotalMemory,
			GroupId:           groupID,
			GroupSize:         int32(groupSize),
			ContainerHostType: r.hostInfo.ContainerHostType,
		})
	}

	return StandardRunResult(messages), nil
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
