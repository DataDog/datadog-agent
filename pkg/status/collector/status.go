// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector fetch information needed to render the 'collector' section of the status page.
// This will, in time, be migrated to the collector package/comp.
package collector

import (
	"embed"
	"encoding/json"
	"expvar"
	"io"
	"sort"

	collectorcomp "github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// Provider provides the functionality to populate the status output with the collector information
type Provider struct {
	coll   collectorcomp.Component
	config pkgconfigmodel.Reader
}

// NewProvider creates a Provider that reads check metadata directly from the collector
// instead of relying on the inventories expvar published by the inventorychecks component.
func NewProvider(coll collectorcomp.Component, config pkgconfigmodel.Reader) Provider {
	return Provider{coll: coll, config: config}
}

// GetStatusInfo retrieves collector information
func (p Provider) GetStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})
	p.PopulateStatus(stats)
	return stats
}

// PopulateStatus populates stats with collector information
func (p Provider) PopulateStatus(stats map[string]interface{}) {
	runnerVar := expvar.Get("runner")
	if runnerVar != nil {
		runnerStatsJSON := []byte(runnerVar.String())
		runnerStats := make(map[string]interface{})
		_ = json.Unmarshal(runnerStatsJSON, &runnerStats)
		stats["runnerStats"] = runnerStats

		// Extract worker utilization data if available
		if workersData, ok := runnerStats["Workers"]; ok {
			workerStats := workersData.(map[string]interface{})

			// Calculate average utilization and sort workers by utilization
			if instancesData, ok := workerStats["Instances"]; ok {
				instances := instancesData.(map[string]interface{})
				totalUtilization := 0.0
				workerCount := 0

				// Create a slice to hold worker data for sorting
				type workerInfo struct {
					Name        string
					Utilization float64
					Data        map[string]interface{}
				}
				var workers []workerInfo

				// Tally up utilization and populate the workers slice
				for workerName, workerData := range instances {
					if worker, ok := workerData.(map[string]interface{}); ok {
						if util, ok := worker["Utilization"].(float64); ok {
							totalUtilization += util
							workerCount++
							workers = append(workers, workerInfo{
								Name:        workerName,
								Utilization: util,
								Data:        worker,
							})
						}
					}
				}

				if workerCount > 0 {
					avgUtilization := totalUtilization / float64(workerCount)
					workerStats["AverageUtilization"] = avgUtilization

					// Sort workers by utilization in descending order
					sort.Slice(workers, func(i, j int) bool {
						return workers[i].Utilization > workers[j].Utilization
					})

					// Keep only top 25 workers
					maxWorkers := 25
					if len(workers) > maxWorkers {
						workers = workers[:maxWorkers]
					}

					// Create a slice of top workers to preserve sorted order
					topWorkers := make([]struct {
						Name        string
						Utilization float64
					}, 0, len(workers))
					for _, worker := range workers {
						topWorkers = append(topWorkers, struct {
							Name        string
							Utilization float64
						}{
							Name:        worker.Name,
							Utilization: worker.Utilization,
						})
					}
					workerStats["TopWorkers"] = topWorkers
				}

				stats["workerStats"] = workerStats
			}
		}
	}

	if expvar.Get("autoconfig") != nil {
		autoConfigStatsJSON := []byte(expvar.Get("autoconfig").String())
		autoConfigStats := make(map[string]interface{})
		_ = json.Unmarshal(autoConfigStatsJSON, &autoConfigStats)
		stats["autoConfigStats"] = autoConfigStats
	}

	checkSchedulerStatsJSON := []byte(expvar.Get("CheckScheduler").String())
	checkSchedulerStats := make(map[string]interface{})
	_ = json.Unmarshal(checkSchedulerStatsJSON, &checkSchedulerStats)
	stats["checkSchedulerStats"] = checkSchedulerStats

	pyLoaderData := expvar.Get("pyLoader")
	if pyLoaderData != nil {
		pyLoaderStatsJSON := []byte(pyLoaderData.String())
		pyLoaderStats := make(map[string]interface{})
		_ = json.Unmarshal(pyLoaderStatsJSON, &pyLoaderStats)
		stats["pyLoaderStats"] = pyLoaderStats
	} else {
		stats["pyLoaderStats"] = nil
	}

	pythonInitData := expvar.Get("pythonInit")
	if pythonInitData != nil {
		pythonInitJSON := []byte(pythonInitData.String())
		pythonInit := make(map[string]interface{})
		_ = json.Unmarshal(pythonInitJSON, &pythonInit)
		stats["pythonInit"] = pythonInit
	} else {
		stats["pythonInit"] = nil
	}

	stats["inventories"] = p.collectCheckMetadata()
}

// collectCheckMetadata builds a map of check-hash → metadata by calling check.GetMetadata
// directly on each check registered with the collector. Falls back to reading the
// inventories expvar when the collector is not available (e.g. in tests or CLI commands).
func (p Provider) collectCheckMetadata() map[string]map[string]string {
	checkMetadata := map[string]map[string]string{}

	if p.coll == nil {
		// Fall back to the inventories expvar published by the inventorychecks component.
		return collectCheckMetadataFromExpvar()
	}

	invChecksEnabled := p.config != nil && p.config.GetBool("inventories_checks_configuration_enabled")

	p.coll.MapOverChecks(func(checks []check.Info) {
		for _, c := range checks {
			metadata := check.GetMetadata(c, invChecksEnabled)
			checkHash := ""
			result := map[string]string{}
			for k, v := range metadata {
				if vStr, ok := v.(string); ok {
					switch k {
					case "config.hash":
						checkHash = vStr
					case "config.provider":
						// excluded from the status output (same as the expvar path)
					default:
						result[k] = vStr
					}
				}
			}
			if checkHash != "" && len(result) != 0 {
				checkMetadata[checkHash] = result
			}
		}
	})

	return checkMetadata
}

// collectCheckMetadataFromExpvar reads check metadata from the inventories expvar,
// which is published by the inventorychecks component. Used as a fallback when the
// collector component is not wired into the status provider.
func collectCheckMetadataFromExpvar() map[string]map[string]string {
	checkMetadata := map[string]map[string]string{}

	inventories := expvar.Get("inventories")
	if inventories == nil {
		return checkMetadata
	}

	var inventoriesStats map[string]interface{}
	_ = json.Unmarshal([]byte(inventories.String()), &inventoriesStats)

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

	return checkMetadata
}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (Provider) Name() string {
	return "Collector"
}

// Section return the section
func (Provider) Section() string {
	return status.CollectorSection
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	p.PopulateStatus(stats)

	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "collector.tmpl", buffer, p.GetStatusInfo())
}

// HTML renders the html output
func (p Provider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "collectorHTML.tmpl", buffer, p.GetStatusInfo())
}

// TextWithData allows to render the human readable version with custom data.
// This is a hack only needed for the agent check subcommand.
func (Provider) TextWithData(buffer io.Writer, data any) error {
	return status.RenderText(templatesFS, "collector.tmpl", buffer, data)
}
