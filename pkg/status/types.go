// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
)

// CLCChecks is used to unmarshall the runner expvar payload for CLC Runner
type CLCChecks struct {
	Checks  map[string]map[string]CLCStats `json:"Checks"`
	Workers Workers                        `json:"Workers"`
}

// CLCStats is used to unmarshall the stats needed from the runner expvar payload
type CLCStats struct {
	AverageExecutionTime int    `json:"AverageExecutionTime"`
	MetricSamples        int    `json:"MetricSamples"`
	HistogramBuckets     int    `json:"HistogramBuckets"`
	Events               int    `json:"Events"`
	ServiceChecks        int    `json:"ServiceChecks"`
	LastExecFailed       bool   `json:"LastExecFailed"`
	LastError            string `json:"LastError"`
	TotalRuns            uint64 `json:"TotalRuns"`
	TotalErrors          uint64 `json:"TotalErrors"`
	TotalMetricSamples   uint64 `json:"TotalMetricSamples"`
	TotalEvents          uint64 `json:"TotalEvents"`
	TotalServiceChecks   uint64 `json:"TotalServiceChecks"`
	LastSuccessDate      int64  `json:"LastSuccessDate"`
	LastExecutionDate    int64  `json:"LastExecutionDate"`
}

// Workers is used to unmarshall the workers info needed from the runner expvar payload
type Workers struct {
	Count     int                   `json:"Count"`
	Instances map[string]WorkerInfo `json:"Instances"`
}

// WorkerInfo is used to unmarshall the workers info needed from the runner expvar payload
type WorkerInfo struct {
	Utilization float64 `json:"Utilization"`
}

// UnmarshalJSON overwrites the unmarshall method for CLCStats
func (d *CLCStats) UnmarshalJSON(data []byte) error {
	var stats stats.Stats
	if err := json.Unmarshal(data, &stats); err != nil {
		return err
	}
	d.AverageExecutionTime = int(stats.AverageExecutionTime)
	d.MetricSamples = int(stats.MetricSamples)
	d.HistogramBuckets = int(stats.HistogramBuckets)
	d.Events = int(stats.Events)
	d.ServiceChecks = int(stats.ServiceChecks)
	d.LastError = stats.LastError
	d.LastExecFailed = stats.LastError != ""
	d.TotalRuns = stats.TotalRuns
	d.TotalErrors = stats.TotalErrors
	d.TotalMetricSamples = stats.TotalMetricSamples
	d.TotalEvents = stats.TotalEvents
	d.TotalServiceChecks = stats.TotalServiceChecks
	d.LastSuccessDate = stats.LastSuccessDate
	d.LastExecutionDate = stats.UpdateTimestamp.UnixMilli()

	return nil
}
