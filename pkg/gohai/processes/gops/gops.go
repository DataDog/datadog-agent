// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build linux || darwin

package gops

import (
	"sort"
)

// TopRSSProcessGroups returns an ordered slice of the process groups that use the most RSS
func TopRSSProcessGroups(limit int) (ProcessNameGroups, error) {
	procs, err := GetProcesses()
	if err != nil {
		return nil, err
	}

	procGroups := ByRSSDesc{GroupByName(procs)}

	sort.Sort(procGroups)

	return procGroups.ProcessNameGroups[:min(limit, len(procGroups.ProcessNameGroups))], nil
}
