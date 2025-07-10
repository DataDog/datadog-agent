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
// into a single update for a process. We have to do this because there is
// no sentinel message when the iteration through the configs begins or ends.
// Instead, we rely on the idle period to determine when we've seen all the
// configs for a process.
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
	processID   actuator.ProcessID
	executable  actuator.Executable
	runtimeID   string
	lastUpdated time.Time
	files       []remoteConfigFile
}

func (c *debouncer) track(
	processID actuator.ProcessID,
	executable actuator.Executable,
) {
	c.processes[processID] = &debouncerProcess{
		processID:  processID,
		executable: executable,
	}
}

func (c *debouncer) untrack(processID actuator.ProcessID) {
	delete(c.processes, processID)
}

func (c *debouncer) addInFlight(
	now time.Time,
	processID actuator.ProcessID,
	file remoteConfigFile,
) (err error) {
	p, ok := c.processes[processID]
	if !ok {
		// Update corresponds to an untracked process.
		return
	}
	p.lastUpdated = now
	if p.runtimeID != "" && p.runtimeID != file.RuntimeID {
		log.Warnf(
			"rcscrape: process %v: runtime ID mismatch: %s != %s",
			p.processID, p.runtimeID, file.RuntimeID,
		)
		clear(p.files)
	}
	p.runtimeID = file.RuntimeID
	p.files = append(p.files, file)
	if log.ShouldLog(log.TraceLvl) {
		log.Tracef(
			"rcscrape: process %v: got update for %s",
			p.processID, file.ConfigPath,
		)
	}
	return nil
}

func (c *debouncer) coalesceInFlight(now time.Time) []ProcessUpdate {
	var updates []ProcessUpdate

	for procID, process := range c.processes {
		if process.lastUpdated.IsZero() ||
			process.lastUpdated.Add(c.idlePeriod).After(now) {
			continue
		}
		delete(c.processes, procID)
		slices.SortFunc(process.files, func(a, b remoteConfigFile) int {
			return cmp.Compare(a.ConfigPath, b.ConfigPath)
		})
		process.files = slices.CompactFunc(process.files, sameConfigPath)
		probes := make([]ir.ProbeDefinition, 0, len(process.files))
		for _, file := range process.files {
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
		updates = append(updates, ProcessUpdate{
			ProcessUpdate: actuator.ProcessUpdate{
				ProcessID:  procID,
				Executable: process.executable,
				Probes:     probes,
			},
			RuntimeID: process.runtimeID,
		})
	}
	slices.SortFunc(updates, func(a, b ProcessUpdate) int {
		return cmp.Compare(a.ProcessID.PID, b.ProcessID.PID)
	})
	return updates
}

func sameConfigPath(a, b remoteConfigFile) bool {
	return a.ConfigPath == b.ConfigPath
}

func eqProbeIDs[A, B ir.ProbeIDer](a A, b B) bool {
	return ir.CompareProbeIDs(a, b) == 0
}
