// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpfcheck

import (
	"fmt"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck/model"
)

// deduplicateProgramNames disambiguates ebpf programs by adding a numeric ID if necessary
func deduplicateProgramNames(stats *model.EBPFStats) {
	slices.SortStableFunc(stats.Programs, func(a, b model.EBPFProgramStats) bool {
		x := strings.Compare(a.Name, b.Name)
		if x == 0 {
			x = strings.Compare(a.Module, b.Module)
			if x == 0 {
				return a.ID < b.ID
			}
		}
		return x == -1
	})
	// if program name is a duplicate, we need to disambiguate by adding a numeric ID
	for i := 0; i < len(stats.Programs)-1; {
		origName := stats.Programs[i].Name
		origModule := stats.Programs[i].Module
		// if we have a series of at least two entries
		if stats.Programs[i+1].Name == origName && stats.Programs[i+1].Module == origModule {
			// start with i, so we overwrite the first entry in series
			j := i
			for ; j < len(stats.Programs) && stats.Programs[j].Name == origName && stats.Programs[j].Module == origModule; j++ {
				stats.Programs[j].Name = fmt.Sprintf("%s_%d", stats.Programs[j].Name, j-i+1)
			}
			i = j
			continue
		}

		i++
	}
}

// deduplicateMapNames disambiguates ebpf maps by adding a numeric ID if necessary
func deduplicateMapNames(stats *model.EBPFStats) {
	allMaps := make([]*model.EBPFMapStats, 0, len(stats.Maps)+len(stats.PerfBuffers))
	for i := range stats.Maps {
		allMaps = append(allMaps, &stats.Maps[i])
	}
	for i := range stats.PerfBuffers {
		allMaps = append(allMaps, &stats.PerfBuffers[i].EBPFMapStats)
	}

	cmpFunc := func(a, b *model.EBPFMapStats) bool {
		x := strings.Compare(a.Name, b.Name)
		if x == 0 {
			x = strings.Compare(a.Module, b.Module)
			if x == 0 {
				return a.ID < b.ID
			}
		}
		return x == -1
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
				allMaps[j].Name = fmt.Sprintf("%s_%d", allMaps[j].Name, j-i+1)
			}
			i = j
			continue
		}
		i++
	}
}
