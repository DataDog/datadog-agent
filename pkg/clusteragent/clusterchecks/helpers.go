// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

var (
	checkExecutionTimeWeight           = 0.0
	checkMetricSamplesWeight   float64 = 1
	checkHistogramBucketWeight float64 = 1
	checkEventsWeight                  = 0.2
)

// makeConfigArray flattens a map of configs into a slice. Creating a new slice
// allows for thread-safe usage by other external, as long as the field values in
// the config objects are not modified.
func makeConfigArray(configMap map[string]integration.Config) []integration.Config {
	configSlice := make([]integration.Config, 0, len(configMap))
	for _, c := range configMap {
		configSlice = append(configSlice, c)
	}
	return configSlice
}

func timestampNowNano() int64 {
	return time.Now().UnixNano()
}

// timestampNow provides a consistent way to keep a seconds timestamp
func timestampNow() int64 {
	return time.Now().Unix()
}

// calculateBusyness returns the busyness value of a node
func calculateBusyness(checkStats types.CLCRunnersStats) int {
	busyness := 0
	for _, stats := range checkStats {
		busyness += busynessFunc(stats)
	}
	return busyness
}

// busynessFunc returns the weight of a check
func busynessFunc(s types.CLCRunnerStats) int {
	if s.LastExecFailed {
		// The check is failing, its weight is 0
		return 0
	}
	return int(checkExecutionTimeWeight*float64(s.AverageExecutionTime) +
		checkMetricSamplesWeight*float64(s.MetricSamples) +
		checkHistogramBucketWeight*float64(s.HistogramBuckets) +
		checkEventsWeight*float64(s.Events))
}

// orderedKeys sorts the keys of a map and return them in a slice
func orderedKeys(m map[string]int) []string {
	keys := []string{}
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
