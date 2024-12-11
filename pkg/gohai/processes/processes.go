// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

// Package processes regroups collecting information about running processes.
package processes

import (
	"strings"
	"time"
)

// limit is the number of processes to collect by default
const limit = 20

// Info represents a list of process groups
type Info []ProcessGroup

// ProcessGroup represents the information about a single process group
type ProcessGroup struct {
	// Usernames is the sorted list of usernames of running processes in that groups.
	Usernames []string
	// PctCPU is the percentage of cpu used by the group.
	PctCPU int
	// PctMem is the percentage of memory used by the group.
	PctMem float64
	// VMS is the vms of the group.
	VMS uint64
	// RSS is the RSS used by the group.
	RSS uint64
	// Name is the name of the group.
	Name string
	// Pids is the list of pids in the group.
	Pids []int32
}

// CollectInfo returns a list of process groups information or an error
func CollectInfo() (Info, error) {
	return getProcessGroups(limit)
}

// ProcessField is an untyped representation of a process group,
// compatible with the legacy "processes" resource check.
type ProcessField [7]interface{}

// AsJSON collects the processes information.
// Returns an object which can be converted to a JSON or an error if nothing could be collected.
// Tries to collect as much information as possible.
func (processGroups Info) AsJSON() (interface{}, []string, error) {
	snapData := make([]ProcessField, len(processGroups))

	for i, processGroup := range processGroups {
		processField := ProcessField{
			strings.Join(processGroup.Usernames, ","),
			processGroup.PctCPU,
			processGroup.PctMem,
			processGroup.VMS,
			processGroup.RSS,
			processGroup.Name,
			len(processGroup.Pids),
		}
		snapData[i] = processField
	}

	// with the current implementation no warning can be returned
	warnings := []string{}

	return []interface{}{time.Now().Unix(), snapData}, warnings, nil
}
