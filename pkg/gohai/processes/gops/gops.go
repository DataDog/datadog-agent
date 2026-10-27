// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build linux || darwin

package gops

import (
	"cmp"
	"slices"
)

// TopRSSProcessGroups returns an ordered slice of the process groups that use the most RSS
func TopRSSProcessGroups(limit int) (ProcessNameGroups, error) {
	procs, err := GetProcesses()
	if err != nil {
		return nil, err
	}

	procGroups := GroupByName(procs)

	slices.SortFunc(procGroups, func(a, b *ProcessNameGroup) int { return cmp.Compare(b.RSS(), a.RSS()) })

	return procGroups[:min(limit, len(procGroups))], nil
}
