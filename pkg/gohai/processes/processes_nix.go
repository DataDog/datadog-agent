// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build linux || darwin

// Package processes regroups collecting information about existing processes
package processes

import (
	"github.com/DataDog/datadog-agent/pkg/gohai/processes/gops"
)

func getProcessGroups(limit int) ([]ProcessGroup, error) {
	processGroups, err := gops.TopRSSProcessGroups(limit)
	if err != nil {
		return nil, err
	}

	snapData := make([]ProcessGroup, len(processGroups))
	for i, processGroup := range processGroups {
		processGroup := ProcessGroup{
			processGroup.Usernames(),
			0, // pct_cpu, requires two consecutive samples to be computed, so not fetched for now
			processGroup.PctMem(),
			processGroup.VMS(),
			processGroup.RSS(),
			processGroup.Name(),
			processGroup.Pids(),
		}
		snapData[i] = processGroup
	}

	return snapData, nil
}
