// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package doctorimpl

import (
	"encoding/json"
	"expvar"
	"time"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	checkstats "github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/status/health"

	"github.com/DataDog/datadog-agent/comp/doctor/def"
)

// collectIngestionStatus aggregates ingestion telemetry from expvars
func (d *doctorImpl) collectIngestionStatus() def.IngestionStatus {
	return def.IngestionStatus{
		Checks:    d.collectChecksStatus(),
		DogStatsD: d.collectDogStatsDStatus(),
		Logs:      d.collectLogsStatus(),
		Metrics:   d.collectMetricsStatus(),
	}
}

// collectChecksStatus aggregates check runner telemetry
func (d *doctorImpl) collectChecksStatus() def.ChecksStatus {
	checksStatus := def.ChecksStatus{
		CheckList: []def.CheckInfo{},
	}

	// Get check stats from runner expvars
	checkStatsMap := expvars.GetCheckStats()

	for checkName, instances := range checkStatsMap {
		for checkID, stats := range instances {
			checksStatus.Total++

			// Determine status
			status := "ok"
			var lastError string

			if stats.LastError != "" {
				checksStatus.Errors++
				status = "error"
				lastError = stats.LastError
			} else if len(stats.LastWarnings) > 0 {
				checksStatus.Warnings++
				status = "warning"
			}

			// Check if currently running
			runningTime := expvars.GetRunningStats(checkID)
			if !runningTime.IsZero() {
				checksStatus.Running++
			}

			checksStatus.CheckList = append(checksStatus.CheckList, def.CheckInfo{
				Name:         checkName,
				Status:       status,
				LastRun:      time.Time{}, // TODO: derive from UpdateTimestamp
				LastError:    lastError,
				MetricsCount: int(stats.TotalMetricSamples),
			})
		}
	}

	// Get error and warning counts from expvars
	checksStatus.Errors = int(expvars.GetErrorsCount())
	checksStatus.Warnings = int(expvars.GetWarningsCount())

	return checksStatus
}

// collectDogStatsDStatus aggregates DogStatsD telemetry
func (d *doctorImpl) collectDogStatsDStatus() def.DogStatsDStatus {
	status := def.DogStatsDStatus{}

	// Get DogStatsD stats from expvars
	dogstatsdVar := expvar.Get("dogstatsd")
	if dogstatsdVar == nil {
		return status
	}

	dogstatsdJSON := []byte(dogstatsdVar.String())
	var dogstatsdStats map[string]interface{}
	if err := json.Unmarshal(dogstatsdJSON, &dogstatsdStats); err != nil {
		d.log.Debugf("Failed to unmarshal dogstatsd stats: %v", err)
		return status
	}

	// Extract metrics
	if val, ok := dogstatsdStats["MetricsReceived"].(float64); ok {
		status.MetricsReceived = int64(val)
	}
	if val, ok := dogstatsdStats["PacketsReceived"].(float64); ok {
		status.PacketsReceived = int64(val)
	}
	if val, ok := dogstatsdStats["PacketsDropped"].(float64); ok {
		status.PacketsDropped = int64(val)
	}
	if val, ok := dogstatsdStats["ParseErrors"].(float64); ok {
		status.ParseErrors = int64(val)
	}

	return status
}

// collectLogsStatus aggregates logs telemetry
func (d *doctorImpl) collectLogsStatus() def.LogsStatus {
	status := def.LogsStatus{}

	// Get logs stats from expvars
	logsVar := expvar.Get("logs")
	if logsVar == nil {
		return status
	}

	logsJSON := []byte(logsVar.String())
	var logsStats map[string]interface{}
	if err := json.Unmarshal(logsJSON, &logsStats); err != nil {
		d.log.Debugf("Failed to unmarshal logs stats: %v", err)
		return status
	}

	// Extract metrics
	if val, ok := logsStats["LogSources"].(float64); ok {
		status.Sources = int(val)
	}
	if val, ok := logsStats["BytesProcessed"].(float64); ok {
		status.BytesProcessed = int64(val)
	}
	if val, ok := logsStats["LinesProcessed"].(float64); ok {
		status.LinesProcessed = int64(val)
	}
	if val, ok := logsStats["Errors"].(float64); ok {
		status.Errors = int(val)
	}

	return status
}

// collectMetricsStatus aggregates metrics aggregator telemetry
func (d *doctorImpl) collectMetricsStatus() def.MetricsStatus {
	status := def.MetricsStatus{}

	// Get aggregator stats from expvars
	aggregatorVar := expvar.Get("aggregator")
	if aggregatorVar == nil {
		return status
	}

	aggregatorJSON := []byte(aggregatorVar.String())
	var aggregatorStats map[string]interface{}
	if err := json.Unmarshal(aggregatorJSON, &aggregatorStats); err != nil {
		d.log.Debugf("Failed to unmarshal aggregator stats: %v", err)
		return status
	}

	// Extract metrics
	if val, ok := aggregatorStats["MetricsInQueue"].(float64); ok {
		status.InQueue = int(val)
	}
	if val, ok := aggregatorStats["MetricsFlushed"].(float64); ok {
		status.Flushed = int64(val)
	}

	return status
}

// collectIntakeStatus aggregates forwarder and backend connectivity telemetry
func (d *doctorImpl) collectIntakeStatus() def.IntakeStatus {
	status := def.IntakeStatus{
		Endpoints: []def.EndpointStatus{},
	}

	// Get forwarder stats from expvars
	forwarderVar := expvar.Get("forwarder")
	if forwarderVar == nil {
		return status
	}

	forwarderJSON := []byte(forwarderVar.String())
	var forwarderStats map[string]interface{}
	if err := json.Unmarshal(forwarderJSON, &forwarderStats); err != nil {
		d.log.Debugf("Failed to unmarshal forwarder stats: %v", err)
		return status
	}

	// Extract connection status
	if val, ok := forwarderStats["Connected"].(bool); ok {
		status.Connected = val
	}

	// Extract API key info
	if apiKeyValid, ok := forwarderStats["APIKeyValid"].(bool); ok {
		status.APIKeyInfo.Valid = apiKeyValid
	}

	// Extract last flush time
	if lastFlush, ok := forwarderStats["LastFlush"].(string); ok {
		if t, err := time.Parse(time.RFC3339, lastFlush); err == nil {
			status.LastFlush = t
		}
	}

	// Extract retry queue size
	if retryQueue, ok := forwarderStats["RetryQueueSize"].(float64); ok {
		status.RetryQueue = int(retryQueue)
	}

	// Extract endpoint statuses
	if endpoints, ok := forwarderStats["Endpoints"].(map[string]interface{}); ok {
		for name, endpointData := range endpoints {
			if endpoint, ok := endpointData.(map[string]interface{}); ok {
				endpointStatus := def.EndpointStatus{
					Name: name,
				}

				if url, ok := endpoint["URL"].(string); ok {
					endpointStatus.URL = url
				}
				if endpointState, ok := endpoint["Status"].(string); ok {
					endpointStatus.Status = endpointState
				}
				if lastError, ok := endpoint["LastError"].(string); ok {
					endpointStatus.LastError = lastError
				}

				status.Endpoints = append(status.Endpoints, endpointStatus)
			}
		}
	}

	return status
}

// collectHealthStatus uses the health package to get component health
func (d *doctorImpl) collectHealthStatus() def.HealthStatus {
	h := health.GetReady()

	return def.HealthStatus{
		Healthy:   h.Healthy,
		Unhealthy: h.Unhealthy,
	}
}

// Helper to safely get check ID from various check stat types
func getCheckID(stats interface{}) checkid.ID {
	if s, ok := stats.(*checkstats.Stats); ok {
		return s.CheckID
	}
	return checkid.ID("")
}
