// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package checks

import (
	"fmt"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/shirou/gopsutil/v4/cpu"

	"github.com/DataDog/datadog-agent/pkg/process/net"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (p *ProcessCheck) run(groupID int32, collectRealTime bool) (RunResult, error) {
	start := time.Now()
	cpuTimes, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}
	if len(cpuTimes) == 0 {
		return nil, errEmptyCPUTime
	}

	procs, err := p.probe.ProcessesByPID(time.Now(), true)
	if err != nil {
		return nil, err
	}

	// stores lastPIDs to be used by RTProcess
	p.lastPIDs = p.lastPIDs[:0]
	for pid := range procs {
		p.lastPIDs = append(p.lastPIDs, pid)
	}

	if p.sysprobeClient != nil && p.sysProbeConfig.ProcessModuleEnabled {
		pStats, err := net.GetProcStats(p.sysprobeClient, p.lastPIDs)
		if err == nil {
			mergeProcWithSysprobeStats(procs, pStats)
		} else {
			log.Debugf("cannot do GetProcStats from system-probe for process check: %s", err)
		}
	}

	var containers []*model.Container
	var pidToCid map[int]string
	var lastContainerRates map[string]*proccontainers.ContainerRateMetrics
	cacheValidity := cacheValidityNoRT
	if collectRealTime {
		cacheValidity = cacheValidityRT
	}

	containers, lastContainerRates, pidToCid, err = p.containerProvider.GetContainers(cacheValidity, p.lastContainerRates)
	if err == nil {
		p.lastContainerRates = lastContainerRates
	} else {
		log.Debugf("Unable to gather stats for containers, err: %v", err)
	}

	// Notify the workload meta extractor that the mapping between pid and cid has changed
	if p.workloadMetaExtractor != nil {
		p.workloadMetaExtractor.SetLastPidToCid(pidToCid)
	}

	for _, extractor := range p.extractors {
		extractor.Extract(procs)
	}

	// End check early if this is our first run.
	if p.lastProcs == nil {
		p.lastProcs = procs
		p.lastCPUTime = cpuTimes[0]
		p.lastRun = time.Now()

		if collectRealTime {
			p.realtimeLastCPUTime = p.lastCPUTime
			p.realtimeLastProcs = procsToStats(p.lastProcs)
			p.realtimeLastRun = p.lastRun
		}
		return CombinedRunResult{}, nil
	}

	collectorProcHints := p.generateHints()
	p.checkCount++

	pidToGPUTags := p.gpuSubscriber.GetGPUTags()

	procsByCtr := fmtProcesses(p.scrubber, p.disallowList, procs, p.lastProcs, pidToCid, cpuTimes[0], p.lastCPUTime, p.lastRun, p.lookupIdProbe, p.ignoreZombieProcesses, p.serviceExtractor, pidToGPUTags)
	messages, totalProcs, totalContainers := createProcCtrMessages(p.hostInfo, procsByCtr, containers, p.maxBatchSize, p.maxBatchBytes, groupID, p.networkID, collectorProcHints)

	// Store the last state for comparison on the next run.
	// Note: not storing the filtered in case there are new processes that haven't had a chance to show up twice.
	p.lastProcs = procs
	p.lastCPUTime = cpuTimes[0]
	p.lastRun = time.Now()

	result := &CombinedRunResult{
		Standard: messages,
	}
	if collectRealTime {
		stats := procsToStats(p.lastProcs)

		if p.realtimeLastProcs != nil {
			// TODO: deduplicate chunking with RT collection
			chunkedStats := fmtProcessStats(p.maxBatchSize, stats, p.realtimeLastProcs, pidToCid, cpuTimes[0], p.realtimeLastCPUTime, p.realtimeLastRun)
			groupSize := len(chunkedStats)
			chunkedCtrStats := convertAndChunkContainers(containers, groupSize)

			messages := make([]model.MessageBody, 0, groupSize)
			for i := 0; i < groupSize; i++ {
				messages = append(messages, &model.CollectorRealTime{
					HostName:          p.hostInfo.HostName,
					Stats:             chunkedStats[i],
					ContainerStats:    chunkedCtrStats[i],
					GroupId:           groupID,
					GroupSize:         int32(groupSize),
					NumCpus:           int32(len(p.hostInfo.SystemInfo.Cpus)),
					TotalMemory:       p.hostInfo.SystemInfo.TotalMemory,
					ContainerHostType: p.hostInfo.ContainerHostType,
				})
			}
			result.Realtime = messages
		}

		p.realtimeLastCPUTime = p.lastCPUTime
		p.realtimeLastProcs = stats
		p.realtimeLastRun = p.lastRun
	}

	agentNameTag := fmt.Sprintf("agent:%s", flavor.GetFlavor())
	_ = p.statsd.Gauge("datadog.process.containers.host_count", float64(totalContainers), []string{agentNameTag}, 1)
	_ = p.statsd.Gauge("datadog.process.processes.host_count", float64(totalProcs), []string{agentNameTag}, 1)
	log.Debugf("collected processes in %s", time.Since(start))

	return result, nil
}
