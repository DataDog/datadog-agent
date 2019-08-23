// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package status

// CLCChecks is used to unmarshall the runner expvar payload for CLC Runner
type CLCChecks struct {
	Checks map[string]map[string]CLCStats `json:"Checks"`
}

// CLCStats is used to unmarshall the stats needed from the runner expvar payload
type CLCStats struct {
	AverageExecutionTime int `json:"AverageExecutionTime"`
	MetricSamples        int `json:"MetricSamples"`
}
