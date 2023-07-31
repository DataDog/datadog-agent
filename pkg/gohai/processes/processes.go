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

type ProcessGroup struct {
	Usernames []string
	PctCPU    int
	PctMem    float64
	VMS       uint64
	RSS       uint64
	Name      string
	NbPids    int
}

func Get() ([]ProcessGroup, error) {
	return getProcessGroups(options.limit)
}

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
			processGroup.NbPids,
		}
		snapData[i] = processField
	}

	return []interface{}{time.Now().Unix(), snapData}, nil
}
