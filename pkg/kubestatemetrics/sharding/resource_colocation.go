// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharding

import "fmt"

// BuildResourceColocation maps each configured collector to the first collector
// in its colocation group. Repeated names within a group are idempotent, but a
// collector cannot belong to multiple groups.
func BuildResourceColocation(groups [][]string, collectors []string) (map[string]string, error) {
	enabled := make(map[string]struct{}, len(collectors))
	for _, collector := range collectors {
		enabled[collector] = struct{}{}
	}

	colocation := make(map[string]string)
	groupByCollector := make(map[string]int)
	for groupIndex, group := range groups {
		if len(group) == 0 {
			continue
		}

		canonical := group[0]
		for _, collector := range group {
			if _, found := enabled[collector]; !found {
				return nil, fmt.Errorf("colocated resource %q is not an enabled collector", collector)
			}
			if previousGroup, found := groupByCollector[collector]; found {
				if previousGroup != groupIndex {
					return nil, fmt.Errorf("resource %q appears in multiple colocation groups", collector)
				}
				continue
			}

			groupByCollector[collector] = groupIndex
			colocation[collector] = canonical
		}
	}

	return colocation, nil
}
