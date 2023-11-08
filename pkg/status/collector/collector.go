// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package collector fetch information from different agent chekcs from expvar
package collector

import (
	"encoding/json"
	"expvar"
)

// UpdateStatus populates stats with collector information
func UpdateStatus(stats map[string]interface{}) {
	status := GetStatus()
	for key, value := range status {
		stats[key] = value
	}
}

// GetStatus retrives collector information
func GetStatus() map[string]interface{} {
	stats := make(map[string]interface{})

	runnerStatsJSON := []byte(expvar.Get("runner").String())
	runnerStats := make(map[string]interface{})
	json.Unmarshal(runnerStatsJSON, &runnerStats) //nolint:errcheck
	stats["runnerStats"] = runnerStats

	autoConfigStatsJSON := []byte(expvar.Get("autoconfig").String())
	autoConfigStats := make(map[string]interface{})
	json.Unmarshal(autoConfigStatsJSON, &autoConfigStats) //nolint:errcheck
	stats["autoConfigStats"] = autoConfigStats

	checkSchedulerStatsJSON := []byte(expvar.Get("CheckScheduler").String())
	checkSchedulerStats := make(map[string]interface{})
	json.Unmarshal(checkSchedulerStatsJSON, &checkSchedulerStats) //nolint:errcheck
	stats["checkSchedulerStats"] = checkSchedulerStats

	pyLoaderData := expvar.Get("pyLoader")
	if pyLoaderData != nil {
		pyLoaderStatsJSON := []byte(pyLoaderData.String())
		pyLoaderStats := make(map[string]interface{})
		json.Unmarshal(pyLoaderStatsJSON, &pyLoaderStats) //nolint:errcheck
		stats["pyLoaderStats"] = pyLoaderStats
	} else {
		stats["pyLoaderStats"] = nil
	}

	pythonInitData := expvar.Get("pythonInit")
	if pythonInitData != nil {
		pythonInitJSON := []byte(pythonInitData.String())
		pythonInit := make(map[string]interface{})
		json.Unmarshal(pythonInitJSON, &pythonInit) //nolint:errcheck
		stats["pythonInit"] = pythonInit
	} else {
		stats["pythonInit"] = nil
	}

	inventories := expvar.Get("inventories")
	var inventoriesStats map[string]interface{}
	if inventories != nil {
		inventoriesStatsJSON := []byte(inventories.String())
		json.Unmarshal(inventoriesStatsJSON, &inventoriesStats) //nolint:errcheck
	}

	checkMetadata := map[string]map[string]string{}
	if data, ok := inventoriesStats["check_metadata"]; ok {
		for _, instances := range data.(map[string]interface{}) {
			for _, instance := range instances.([]interface{}) {
				metadata := map[string]string{}
				checkHash := ""
				for k, v := range instance.(map[string]interface{}) {
					if vStr, ok := v.(string); ok {
						if k == "config.hash" {
							checkHash = vStr
						} else if k != "config.provider" {
							metadata[k] = vStr
						}
					}
				}
				if checkHash != "" && len(metadata) != 0 {
					checkMetadata[checkHash] = metadata
				}
			}
		}
	}
	stats["checkMetadata"] = checkMetadata
	if data, ok := inventoriesStats["agent_metadata"]; ok {
		stats["agent_metadata"] = data
	} else {
		stats["agent_metadata"] = map[string]string{}
	}

	return stats
}
