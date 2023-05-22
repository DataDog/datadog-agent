// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build linux || darwin
// +build linux darwin

package gops

import (
	"sort"
)

// ProcessNameGroup represents a group of processes, grouped by name
type ProcessNameGroup struct {
	pids      []int32
	rss       uint64
	pctMem    float64
	vms       uint64
	name      string
	usernames map[string]bool
}

// ProcessNameGroups represents a list of ProcessNameGroup.
type ProcessNameGroups []*ProcessNameGroup

// Pids returns the list of pids in the group.
func (pg *ProcessNameGroup) Pids() []int32 {
	return pg.pids
}

// Name returns the name of the group.
func (pg *ProcessNameGroup) Name() string {
	return pg.name
}

// RSS returns the RSS used by the group.
func (pg *ProcessNameGroup) RSS() uint64 {
	return pg.rss
}

// PctMem returns the percentage of memory used by the group.
func (pg *ProcessNameGroup) PctMem() float64 {
	return pg.pctMem
}

// VMS returns the vms of the group.
func (pg *ProcessNameGroup) VMS() uint64 {
	return pg.vms
}

// Usernames returns a slice of the usernames, sorted alphabetically
func (pg *ProcessNameGroup) Usernames() []string {
	var usernameStringSlice sort.StringSlice
	for username := range pg.usernames {
		usernameStringSlice = append(usernameStringSlice, username)
	}

	sort.Sort(usernameStringSlice)

	return []string(usernameStringSlice)
}

// NewProcessNameGroup returns a new empty ProcessNameGroup
func NewProcessNameGroup() *ProcessNameGroup {
	processNameGroup := new(ProcessNameGroup)
	processNameGroup.usernames = make(map[string]bool)

	return processNameGroup
}

// GroupByName groups the processInfos by name and return a slice of ProcessNameGroup
func GroupByName(processInfos []*ProcessInfo) ProcessNameGroups {
	groupIndexByName := make(map[string]int)
	processNameGroups := make(ProcessNameGroups, 0, 10)

	for _, processInfo := range processInfos {
		if _, ok := groupIndexByName[processInfo.Name]; !ok {
			processNameGroups = append(processNameGroups, NewProcessNameGroup())
			groupIndexByName[processInfo.Name] = len(processNameGroups) - 1
		}

		processNameGroups[groupIndexByName[processInfo.Name]].add(processInfo)
	}

	return processNameGroups
}

func (pg *ProcessNameGroup) add(p *ProcessInfo) {
	pg.pids = append(pg.pids, p.PID)
	if pg.name == "" {
		pg.name = p.Name
	}
	pg.rss += p.RSS
	pg.pctMem += p.PctMem
	pg.vms += p.VMS
	pg.usernames[p.Username] = true
}

// Len returns the number of groups
func (s ProcessNameGroups) Len() int {
	return len(s)
}

// Swap swaps processes at index i and j
func (s ProcessNameGroups) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// ByRSSDesc is used to sort groups by decreasing RSS.
type ByRSSDesc struct {
	ProcessNameGroups
}

// Less returns whether the group at index i uses more RSS than the one at index j.
func (s ByRSSDesc) Less(i, j int) bool {
	return s.ProcessNameGroups[i].RSS() > s.ProcessNameGroups[j].RSS()
}
