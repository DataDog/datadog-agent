// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpfcheck

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck/model"
)

// deduplicateMapNames disambiguates ebpf maps by adding a numeric ID if necessary
func deduplicateMapNames(stats *model.EBPFStats) {
	allMaps := make([]*model.EBPFMapStats, 0, len(stats.Maps))
	for i := range stats.Maps {
		allMaps = append(allMaps, &stats.Maps[i])
	}

	cmpFunc := func(a, b *model.EBPFMapStats) int {
		x := strings.Compare(a.Name, b.Name)
		if x == 0 {
			x = strings.Compare(a.Module, b.Module)
			if x == 0 {
				return int(a.ID - b.ID)
			}
		}
		return x
	}
	slices.SortStableFunc(allMaps, cmpFunc)

	// if map name is a duplicate, we need to disambiguate by adding a numeric ID
	for i := 0; i < len(allMaps)-1; {
		origName := allMaps[i].Name
		origModule := allMaps[i].Module
		// if we have a series of at least two entries
		if allMaps[i+1].Name == origName && allMaps[i+1].Module == origModule {
			// start with i, so we overwrite the first entry in series
			j := i
			for ; j < len(allMaps) && allMaps[j].Name == origName && allMaps[j].Module == origModule; j++ {
				allMaps[j].Name = allMaps[j].Name + "_" + strconv.Itoa(j-i+1)
			}
			i = j
			continue
		}
		i++
	}
}

type programCounters struct {
	runtime         time.Duration
	runCount        uint64
	recursionMisses uint64
}

func (c programCounters) plus(o programCounters) programCounters {
	return programCounters{c.runtime + o.runtime, c.runCount + o.runCount, c.recursionMisses + o.recursionMisses}
}

func (c programCounters) since(prev programCounters) programCounters {
	// a decrease means that the ID now belongs to another program
	if c.runtime < prev.runtime || c.runCount < prev.runCount || c.recursionMisses < prev.recursionMisses {
		return c
	}
	return programCounters{c.runtime - prev.runtime, c.runCount - prev.runCount, c.recursionMisses - prev.recursionMisses}
}

// programAggregator merges the programs that share a key into the entry of the first one. The counters of the entry
// add up the increase of each program between flushes rather than its current value: a sum of current values would
// drop, and look like a counter reset, whenever one of the programs is unloaded.
type programAggregator struct {
	last   map[ebpf.ProgramID]programCounters
	next   map[ebpf.ProgramID]programCounters
	totals map[programKey]programCounters
}

func newProgramAggregator() programAggregator {
	return programAggregator{
		last:   make(map[ebpf.ProgramID]programCounters),
		next:   make(map[ebpf.ProgramID]programCounters),
		totals: make(map[programKey]programCounters),
	}
}

func (a *programAggregator) add(stats *model.EBPFStats, uniquePrograms map[programKey]ebpf.ProgramID, ps *model.EBPFProgramStats) {
	id := ebpf.ProgramID(ps.ID)
	counters := programCounters{ps.Runtime, ps.RunCount, ps.RecursionMisses}
	if counters != (programCounters{}) {
		a.next[id] = counters
	}
	key := progKey(ps)
	a.totals[key] = a.totals[key].plus(counters.since(a.last[id]))

	if _, ok := uniquePrograms[key]; ok {
		return
	}
	uniquePrograms[key] = id
	stats.Programs = append(stats.Programs, *ps)
}

func (a *programAggregator) flush(stats *model.EBPFStats, uniquePrograms map[programKey]ebpf.ProgramID) {
	for key := range a.totals {
		if _, ok := uniquePrograms[key]; !ok {
			delete(a.totals, key)
		}
	}
	for i := range stats.Programs {
		ps := &stats.Programs[i]
		total := a.totals[progKey(ps)]
		ps.Runtime, ps.RunCount, ps.RecursionMisses = total.runtime, total.runCount, total.recursionMisses
	}
	a.last, a.next = a.next, a.last
	clear(a.next)
}
