// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package doctorimpl

import (
	"encoding/json"
	"expvar"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	checkstats "github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	logsstatus "github.com/DataDog/datadog-agent/pkg/logs/status"
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
	// Check if logs are enabled
	logsEnabled := d.config.GetBool("logs_enabled") || d.config.GetBool("log_enabled")

	status := def.LogsStatus{
		Enabled:      logsEnabled,
		Integrations: []def.LogSource{},
	}

	if !logsEnabled {
		return status
	}

	// Get detailed logs status from the logs agent
	// Use verbose=true to get tailer information
	logsAgentStatus := logsstatus.Get(true)

	if !logsAgentStatus.IsRunning {
		return status
	}

	// Collect integration sources
	for _, integration := range logsAgentStatus.Integrations {
		for _, source := range integration.Sources {
			logSource := def.LogSource{
				Name:   integration.Name,
				Type:   source.Type,
				Status: source.Status,
				Inputs: source.Inputs,
				Info:   make(map[string]string),
			}

			// Convert info map from []string to string
			for key, values := range source.Info {
				if len(values) > 0 {
					logSource.Info[key] = values[0]
				}
			}

			status.Integrations = append(status.Integrations, logSource)
			status.Sources++
		}
	}

	// Get expvar stats for additional metrics
	logsVar := expvar.Get("logs")
	if logsVar != nil {
		logsJSON := []byte(logsVar.String())
		var logsStats map[string]interface{}
		if err := json.Unmarshal(logsJSON, &logsStats); err == nil {
			if val, ok := logsStats["BytesProcessed"].(float64); ok {
				status.BytesProcessed = int64(val)
			}
			if val, ok := logsStats["LinesProcessed"].(float64); ok {
				status.LinesProcessed = int64(val)
			}
		}
	}

	// Count errors from status
	status.Errors = len(logsAgentStatus.Errors)

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

// collectServicesStatus aggregates service-level telemetry from traces, metrics, and logs
func (d *doctorImpl) collectServicesStatus() []def.ServiceStats {
	// Map to aggregate service stats
	serviceMap := make(map[string]*def.ServiceStats)

	// Collect trace stats by service from trace-agent expvars
	d.collectTraceServiceStats(serviceMap)

	// Collect metrics stats by service from checks
	d.collectMetricsServiceStats(serviceMap)

	// Collect logs stats by service from log agent
	d.collectLogsServiceStats(serviceMap)

	// Convert map to sorted slice
	services := make([]def.ServiceStats, 0, len(serviceMap))
	for _, stats := range serviceMap {
		services = append(services, *stats)
	}

	// Sort by service name for consistent ordering
	// (Could also sort by total activity: traces + metrics + logs)
	for i := 0; i < len(services); i++ {
		for j := i + 1; j < len(services); j++ {
			if services[i].Name > services[j].Name {
				services[i], services[j] = services[j], services[i]
			}
		}
	}

	return services
}

// collectTraceServiceStats collects trace counts per service from trace-agent expvars
func (d *doctorImpl) collectTraceServiceStats(serviceMap map[string]*def.ServiceStats) {
	// Get trace receiver stats from expvars
	receiverVar := expvar.Get("receiver")
	if receiverVar == nil {
		return
	}

	receiverJSON := []byte(receiverVar.String())
	var receiverStats []map[string]interface{}
	if err := json.Unmarshal(receiverJSON, &receiverStats); err != nil {
		d.log.Debugf("Failed to unmarshal trace receiver stats: %v", err)
		return
	}

	// Extract traces per service
	for _, tagStats := range receiverStats {
		// Look for service tag in Tags
		if tags, ok := tagStats["Tags"].(map[string]interface{}); ok {
			if serviceName, ok := tags["Service"].(string); ok && serviceName != "" {
				// Get traces received count
				if stats, ok := tagStats["Stats"].(map[string]interface{}); ok {
					if tracesReceived, ok := stats["TracesReceived"].(float64); ok {
						// Get or create service entry
						if _, exists := serviceMap[serviceName]; !exists {
							serviceMap[serviceName] = &def.ServiceStats{Name: serviceName}
						}
						// For traces, we only have cumulative counts, not rates
						// TODO: Track delta over time to calculate true rate
						serviceMap[serviceName].TracesRate += tracesReceived
					}
				}
			}
		}
	}
}

// collectMetricsServiceStats collects metric rates per service from check stats
// This parses the instance config YAML to extract the service tag
func (d *doctorImpl) collectMetricsServiceStats(serviceMap map[string]*def.ServiceStats) {
	// Get check stats from runner expvars
	checkStatsMap := expvars.GetCheckStats()

	// Get collector to access check instances
	coll, collectorAvailable := d.collector.Get()

	for _, instances := range checkStatsMap {
		for checkID, stats := range instances {
			// Skip if no metrics were collected in the last run
			if stats.MetricSamples == 0 {
				continue
			}

			var serviceName string
			var checkLoader string

			// Try to get the check instance to parse its config and determine loader
			if collectorAvailable {
				// Get all checks and find the one with matching ID
				checks := coll.GetChecks()
				for _, check := range checks {
					if check.ID() == checkID {
						// Get the loader name (e.g., "core" for corechecks, "python" for python checks)
						checkLoader = check.Loader()

						// Parse the instance config to extract service tag
						serviceName = extractServiceFromInstanceConfig(check.InstanceConfig())
						break
					}
				}
			}

			// If no service tag found, determine default based on check type
			if serviceName == "" {
				// Corechecks (loader == "core") are grouped under "datadog-agent"
				if checkLoader == "core" {
					serviceName = "datadog-agent"
				}
				// For other checks without service tag, use empty string
			}

			// Calculate metrics per second based on check interval
			metricsPerSecond := float64(0)
			if stats.Interval > 0 {
				metricsPerSecond = float64(stats.MetricSamples) / stats.Interval.Seconds()
			}

			// Get or create service entry
			if _, exists := serviceMap[serviceName]; !exists {
				serviceMap[serviceName] = &def.ServiceStats{Name: serviceName}
			}
			// Accumulate rate from all check instances for this service
			serviceMap[serviceName].MetricsRate += metricsPerSecond
		}
	}
}

// collectLogsServiceStats collects log byte rates per service from logs agent
// This uses the "service" tag configured in each log source configuration
// Sources without a service tag are grouped under the "" key
// Uses delta tracking to calculate instantaneous rates instead of average-since-start
func (d *doctorImpl) collectLogsServiceStats(serviceMap map[string]*def.ServiceStats) {
	// Check if logs are enabled
	logsEnabled := d.config.GetBool("logs_enabled") || d.config.GetBool("log_enabled")
	if !logsEnabled {
		return
	}

	// Get detailed logs status from the logs agent
	logsAgentStatus := logsstatus.Get(true)
	if !logsAgentStatus.IsRunning {
		return
	}

	// Lock for thread-safe access to delta tracking state
	d.logsDeltaMu.Lock()
	defer d.logsDeltaMu.Unlock()

	// Calculate time delta since last collection
	now := time.Now()
	timeDelta := now.Sub(d.lastLogsCollectionTime).Seconds()

	// If this is the first collection or time delta is too small, fall back to average rate
	if d.lastLogsCollectionTime.IsZero() || timeDelta < 0.1 {
		// First collection - use average rate since agent start as fallback
		uptimeSeconds := time.Since(d.startTime).Seconds()
		if uptimeSeconds == 0 {
			uptimeSeconds = 1
		}

		for _, integration := range logsAgentStatus.Integrations {
			for _, source := range integration.Sources {
				serviceName := ""
				if svc, ok := source.Configuration["Service"].(string); ok && svc != "" {
					serviceName = svc
				}

				if bytesInfo, ok := source.Info["Bytes Read"]; ok && len(bytesInfo) > 0 {
					var bytesRead int64
					if _, err := fmt.Sscanf(bytesInfo[0], "%d", &bytesRead); err == nil && bytesRead > 0 {
						if _, exists := serviceMap[serviceName]; !exists {
							serviceMap[serviceName] = &def.ServiceStats{Name: serviceName}
						}
						bytesPerSecond := float64(bytesRead) / uptimeSeconds
						serviceMap[serviceName].LogsRate += bytesPerSecond

						// Store current value for next iteration
						d.previousLogsBytesRead[serviceName] = bytesRead
					}
				}
			}
		}

		d.lastLogsCollectionTime = now
		return
	}

	// We have previous data - calculate instantaneous rates using deltas
	currentBytesRead := make(map[string]int64)

	// Collect current bytes read per service
	for _, integration := range logsAgentStatus.Integrations {
		for _, source := range integration.Sources {
			serviceName := ""
			if svc, ok := source.Configuration["Service"].(string); ok && svc != "" {
				serviceName = svc
			}

			if bytesInfo, ok := source.Info["Bytes Read"]; ok && len(bytesInfo) > 0 {
				var bytesRead int64
				if _, err := fmt.Sscanf(bytesInfo[0], "%d", &bytesRead); err == nil && bytesRead > 0 {
					currentBytesRead[serviceName] += bytesRead
				}
			}
		}
	}

	// Calculate rates using deltas
	for serviceName, currentBytes := range currentBytesRead {
		previousBytes, hasPrevious := d.previousLogsBytesRead[serviceName]

		var bytesPerSecond float64
		if hasPrevious && currentBytes >= previousBytes {
			// Calculate instantaneous rate from delta
			deltaBytes := currentBytes - previousBytes
			bytesPerSecond = float64(deltaBytes) / timeDelta
		} else {
			// No previous data or counter reset - use average since start
			uptimeSeconds := time.Since(d.startTime).Seconds()
			if uptimeSeconds == 0 {
				uptimeSeconds = 1
			}
			bytesPerSecond = float64(currentBytes) / uptimeSeconds
		}

		// Get or create service entry
		if _, exists := serviceMap[serviceName]; !exists {
			serviceMap[serviceName] = &def.ServiceStats{Name: serviceName}
		}
		serviceMap[serviceName].LogsRate += bytesPerSecond

		// Update tracking state
		d.previousLogsBytesRead[serviceName] = currentBytes
	}

	// Update last collection time
	d.lastLogsCollectionTime = now
}

// extractServiceFromInstanceConfig parses the instance config YAML to extract the service tag
// Returns the service name from the "service:" tag, or empty string if not found
func extractServiceFromInstanceConfig(instanceConfig string) string {
	if instanceConfig == "" {
		return ""
	}

	// Parse YAML config
	var config map[string]interface{}
	if err := yaml.Unmarshal([]byte(instanceConfig), &config); err != nil {
		// If parsing fails, return empty string
		return ""
	}

	// Extract tags array
	tagsInterface, ok := config["tags"]
	if !ok {
		return ""
	}

	// tags can be a slice of interfaces
	tags, ok := tagsInterface.([]interface{})
	if !ok {
		return ""
	}

	// Look for "service:" tag
	for _, tagInterface := range tags {
		tag, ok := tagInterface.(string)
		if !ok {
			continue
		}

		// Check if tag starts with "service:"
		if strings.HasPrefix(tag, "service:") {
			// Extract the service name after "service:"
			serviceName := strings.TrimPrefix(tag, "service:")
			return strings.TrimSpace(serviceName)
		}
	}

	return ""
}

// Helper to safely get check ID from various check stat types
func getCheckID(stats interface{}) checkid.ID {
	if s, ok := stats.(*checkstats.Stats); ok {
		return s.CheckID
	}
	return checkid.ID("")
}
