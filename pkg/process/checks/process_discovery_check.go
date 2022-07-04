// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"fmt"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// ProcessDiscovery is a ProcessDiscoveryCheck singleton. ProcessDiscovery should not be instantiated elsewhere.
var ProcessDiscovery = &ProcessDiscoveryCheck{}

// ProcessDiscoveryCheck is a check that gathers basic process metadata.
// It uses its own ProcessDiscovery payload.
// The goal of this check is to collect information about possible integrations that may be enabled by the end user.
type ProcessDiscoveryCheck struct {
	probe      procutil.Probe
	info       *model.SystemInfo
	initCalled bool

	maxBatchSize int
}

// Init initializes the ProcessDiscoveryCheck. It is a runtime error to call Run without first having called Init.
func (d *ProcessDiscoveryCheck) Init(_ *config.AgentConfig, info *model.SystemInfo) {
	d.info = info
	d.initCalled = true
	d.probe = getProcessProbe()

	d.maxBatchSize = getMaxBatchSize()
}

// Name returns the name of the ProcessDiscoveryCheck.
func (d *ProcessDiscoveryCheck) Name() string { return config.DiscoveryCheckName }

// RealTime returns a value that says whether this check should be run in real time.
func (d *ProcessDiscoveryCheck) RealTime() bool { return false }

// Run collects process metadata, and packages it into a CollectorProcessDiscovery payload to be sent.
// It is a runtime error to call Run without first having called Init.
func (d *ProcessDiscoveryCheck) Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
	if !d.initCalled {
		return nil, fmt.Errorf("ProcessDiscoveryCheck.Run called before Init")
	}

	// Does not need to collect process stats, only metadata
	procs, err := d.probe.ProcessesByPID(time.Now(), false)
	if err != nil {
		return nil, err
	}

	host := &model.Host{
		Name:        cfg.HostName,
		NumCpus:     calculateNumCores(d.info),
		TotalMemory: d.info.TotalMemory,
	}
	procDiscoveryChunks := chunkProcessDiscoveries(pidMapToProcDiscoveries(procs), d.maxBatchSize)
	payload := make([]model.MessageBody, len(procDiscoveryChunks))
	for i, procDiscoveryChunk := range procDiscoveryChunks {
		payload[i] = &model.CollectorProcDiscovery{
			HostName:           cfg.HostName,
			GroupId:            groupID,
			GroupSize:          int32(len(procDiscoveryChunks)),
			ProcessDiscoveries: procDiscoveryChunk,
			Host:               host,
		}
	}

	return payload, nil
}

// Cleanup frees any resource held by the ProcessDiscoveryCheck before the agent exits
func (d *ProcessDiscoveryCheck) Cleanup() {}

func pidMapToProcDiscoveries(pidMap map[int32]*procutil.Process) []*model.ProcessDiscovery {
	pd := make([]*model.ProcessDiscovery, 0, len(pidMap))
	for _, proc := range pidMap {
		pd = append(pd, &model.ProcessDiscovery{
			Pid:        proc.Pid,
			NsPid:      proc.NsPid,
			Command:    formatCommand(proc),
			User:       formatUser(proc),
			CreateTime: proc.Stats.CreateTime,
		})
	}

	return pd
}

// chunkProcessDiscoveries split non-container processes into chunks and return a list of chunks
// This function is patiently awaiting go to support generics, so that we don't need two chunkProcesses functions :)
func chunkProcessDiscoveries(procs []*model.ProcessDiscovery, size int) [][]*model.ProcessDiscovery {
	chunkCount := len(procs) / size
	if chunkCount*size < len(procs) {
		chunkCount++
	}
	chunks := make([][]*model.ProcessDiscovery, 0, chunkCount)

	for i := 0; i < len(procs); i += size {
		end := i + size
		if end > len(procs) {
			end = len(procs)
		}
		chunks = append(chunks, procs[i:end])
	}

	return chunks
}

// Needed to calculate the correct normalized cpu metric value
// On linux, the cpu array contains an entry per logical core.
// On windows, the cpu array contains an entry per physical core, with correct logical core counts.
func calculateNumCores(info *model.SystemInfo) (numCores int32) {
	for _, cpu := range info.Cpus {
		numCores += cpu.Cores
	}

	return numCores
}
