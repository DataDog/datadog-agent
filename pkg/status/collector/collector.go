// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector fetch information needed to render the 'collector' section of the status page.
// This will, in time, be migrated to the collector package/comp.
package collector

import (
	"encoding/json"
	"expvar"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/status/render"
)

// GetStatusInfo retrives collector information
func GetStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	PopulateStatus(stats)

	return stats
}

// PopulateStatus populates stats with collector information
func PopulateStatus(stats map[string]interface{}) {
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
	stats["inventories"] = checkMetadata
}

// Provider provides the functionality to populate the status output with the collector information
type Provider struct{}

// Name returns the name
func (Provider) Name() string {
	return "Collector"
}

// Section return the section
func (Provider) Section() string {
	return status.CollectorSection
}

// JSON populates the status map
func (Provider) JSON(stats map[string]interface{}) error {
	PopulateStatus(stats)

	return nil
}

// Text populates the status buffer with the human readbable version
func (Provider) Text(buffer io.Writer) error {
	return render.ParseTemplate(buffer, "/collector.tmpl", GetStatusInfo())
}

// HTML populates the status buffer with the HTML version
func (Provider) HTML(buffer io.Writer) error {
	return render.ParseHTMLTemplate(buffer, "/collectorHTML.tmpl", GetStatusInfo())
}

// TextWithData allows to render the human reaadable version with custom data
// This is a hack only needed for the agent check subcommand
func (Provider) TextWithData(buffer io.Writer, data any) error {
	return render.ParseTemplate(buffer, "/collector.tmpl", data)
}
