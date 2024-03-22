// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpfcheck

import (
	"fmt"
	"slices"
	"strings"

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
				allMaps[j].Name = fmt.Sprintf("%s_%d", allMaps[j].Name, j-i+1)
			}
			i = j
			continue
		}
		i++
	}
}
