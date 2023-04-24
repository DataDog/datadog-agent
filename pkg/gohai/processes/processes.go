// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build linux || darwin
// +build linux darwin

// Package processes regroups collecting information about existing processes
package processes

import (
	"strings"
	"time"

	"github.com/DataDog/gohai/processes/gops"
)

// ProcessField is an untyped representation of a process group,
// compatible with the legacy "processes" resource check.
type ProcessField [7]interface{}

// getProcesses return a JSON payload which is compatible with
// the legacy "processes" resource check
func getProcesses(limit int) ([]interface{}, error) {
	processGroups, err := gops.TopRSSProcessGroups(limit)
	if err != nil {
		return nil, err
	}

	snapData := make([]ProcessField, len(processGroups))

	for i, processGroup := range processGroups {
		processField := ProcessField{
			strings.Join(processGroup.Usernames(), ","),
			0, // pct_cpu, requires two consecutive samples to be computed, so not fetched for now
			processGroup.PctMem(),
			processGroup.VMS(),
			processGroup.RSS(),
			processGroup.Name(),
			len(processGroup.Pids()),
		}
		snapData[i] = processField
	}

	return []interface{}{time.Now().Unix(), snapData}, nil
}
