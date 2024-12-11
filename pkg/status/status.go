// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package status implements the status of the agent
package status

import (
	"encoding/json"
	"expvar"
)

// GetExpvarRunnerStats grabs the status of the runner from expvar
// and puts it into a CLCChecks struct
func GetExpvarRunnerStats() (CLCChecks, error) {
	runnerStatsJSON := []byte(expvar.Get("runner").String())
	return convertExpvarRunnerStats(runnerStatsJSON)
}

func convertExpvarRunnerStats(inputJSON []byte) (CLCChecks, error) {
	runnerStats := CLCChecks{}
	err := json.Unmarshal(inputJSON, &runnerStats)
	return runnerStats, err
}
