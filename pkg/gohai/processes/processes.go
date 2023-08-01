// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

// Package processes regroups collecting information about running processes.
package processes

import (
	"flag"
	"strings"
	"time"
)

var options struct {
	limit int
}

// Processes is the Collector type of the processes package.
type Processes struct{}

const name = "processes"

func init() {
	flag.IntVar(&options.limit, name+"-limit", 20, "Number of process groups to return")
}

// Name returns the name of the package
func (processes *Processes) Name() string {
	return name
}

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

// Get returns a list of process groups information or an error
func Get() ([]ProcessGroup, error) {
	return getProcessGroups(options.limit)
}

// ProcessField is an untyped representation of a process group,
// compatible with the legacy "processes" resource check.
type ProcessField [7]interface{}

// Collect collects the processes information.
// Returns an object which can be converted to a JSON or an error if nothing could be collected.
// Tries to collect as much information as possible.
func (processes *Processes) Collect() (result interface{}, err error) {
	processGroups, err := Get()
	if err != nil {
		return nil, err
	}

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

	return []interface{}{time.Now().Unix(), snapData}, nil
}
