// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcscrape

import (
	"cmp"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// debouncer tracks messages from the dd-trace-go callback and coalesces them
// into a single update for a process. We have to do this because old tracer
// versions didn't have a sentinel method we could probe to figure out when the
// complete set of remote config configuration has been scraped (newer tracers
// do have such a method, but we haven't started using it yet). Instead, we rely
// on an idle period to determine when we've seen all the configs for a process.
type debouncer struct {
	idlePeriod time.Duration
	processes  map[actuator.ProcessID]*debouncerProcess
}

func makeDebouncer(idlePeriod time.Duration) debouncer {
	return debouncer{
		idlePeriod: idlePeriod,
		processes:  make(map[actuator.ProcessID]*debouncerProcess),
	}
}

type debouncerProcess struct {
	// lastUpdate is the last time an update for the process was received; a
	// ProcessUpdate is returned when some updates are pending, but no new one
	// has been received for a while.
	lastUpdated time.Time
	// configuration accumulated over a debounce window
	updates      []remoteConfigFile
	symdbEnabled bool
}

func (c *debouncer) clear(processID actuator.ProcessID) {
	delete(c.processes, processID)
}

func (c *debouncer) addUpdate(
	now time.Time,
	processID actuator.ProcessID,
	file remoteConfigFile,
) {
	p := c.getOrInsertProcess(processID)
	p.lastUpdated = now
	if file.ConfigContent == "" {
		return
	}
	p.updates = append(p.updates, file)
}

func (c *debouncer) addSymdbEnabled(
	now time.Time,
	processID actuator.ProcessID,
	symdbEnabled bool,
) {
	p := c.getOrInsertProcess(processID)
	p.lastUpdated = now
	p.symdbEnabled = symdbEnabled
}

func (c *debouncer) getOrInsertProcess(processID actuator.ProcessID) *debouncerProcess {
	p, ok := c.processes[processID]
	if !ok {
		p = &debouncerProcess{}
		c.processes[processID] = p
	}
	return p
}

// getUpdates returns the state of processes that have pending updates but
// have been quiesced for a bit (thereby ending a debounce window). Processes
// for which an updated state is returned are removed from c.processes.
func (c *debouncer) getUpdates(now time.Time) []accumulatedState {
	var res []accumulatedState
	for procID, process := range c.processes {
		if process.lastUpdated.Add(c.idlePeriod).After(now) {
			continue
		}
		res = append(res, accumulatedState{
			procID:       procID,
			probes:       computeProbeDefinitions(procID, process.updates),
			symdbEnabled: process.symdbEnabled,
		})
		delete(c.processes, procID)
	}
	return res
}

type accumulatedState struct {
	procID       actuator.ProcessID
	probes       []ir.ProbeDefinition
	symdbEnabled bool
}

func computeProbeDefinitions(
	procID actuator.ProcessID,
	files []remoteConfigFile,
) []ir.ProbeDefinition {
	slices.SortFunc(files, func(a, b remoteConfigFile) int {
		return cmp.Compare(a.ConfigPath, b.ConfigPath)
	})
	files = slices.CompactFunc(files, sameConfigPath)
	probes := make([]ir.ProbeDefinition, 0, len(files))
	for _, file := range files {
		// TODO: Optimize away this copy of the underlying data by either
		// using unsafe or changing rcjson to use an io.Reader and reusing
		// a strings.Reader.
		probe, err := rcjson.UnmarshalProbe([]byte(file.ConfigContent))
		if err != nil {
			// TODO: Rate limit this warning in some form.
			log.Warnf(
				"process %v: failed to unmarshal probe %s: %v",
				procID, file.ConfigPath, err,
			)
			continue
		}
		probes = append(probes, probe)
	}
	// Collapse duplicates if they somehow showed up.
	slices.SortFunc(probes, ir.CompareProbeIDs)
	probes = slices.CompactFunc(probes, eqProbeIDs)
	return probes
}

func sameConfigPath(a, b remoteConfigFile) bool {
	return a.ConfigPath == b.ConfigPath
}

func eqProbeIDs[A, B ir.ProbeIDer](a A, b B) bool {
	return ir.CompareProbeIDs(a, b) == 0
}
