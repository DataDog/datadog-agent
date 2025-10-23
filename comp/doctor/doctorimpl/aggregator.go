// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package doctorimpl

import (
	"encoding/json"
	"expvar"
	"fmt"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	checkstats "github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	logsstatus "github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/status/health"

	"github.com/DataDog/datadog-agent/comp/core/config"
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

	// Extract transaction success/failure/requeue/error counts and create synthetic endpoints
	successByEndpoint := make(map[string]int64)
	droppedByEndpoint := make(map[string]int64)
	requeuedByEndpoint := make(map[string]int64)
	errorsByEndpoint := make(map[string]int64)

	if transactions, ok := forwarderStats["Transactions"].(map[string]interface{}); ok {
		// Extract success counts by endpoint
		if successMap, ok := transactions["SuccessByEndpoint"].(map[string]interface{}); ok {
			for endpoint, count := range successMap {
				if countFloat, ok := count.(float64); ok {
					successByEndpoint[endpoint] = int64(countFloat)
				}
			}
		}

		// Extract dropped counts by endpoint
		if droppedMap, ok := transactions["DroppedByEndpoint"].(map[string]interface{}); ok {
			for endpoint, count := range droppedMap {
				if countFloat, ok := count.(float64); ok {
					droppedByEndpoint[endpoint] = int64(countFloat)
				}
			}
		}

		// Extract requeued counts by endpoint
		if requeuedMap, ok := transactions["RequeuedByEndpoint"].(map[string]interface{}); ok {
			for endpoint, count := range requeuedMap {
				if countFloat, ok := count.(float64); ok {
					requeuedByEndpoint[endpoint] = int64(countFloat)
				}
			}
		}

		// Extract error counts by endpoint
		if errorsMap, ok := transactions["ErrorsByEndpoint"].(map[string]interface{}); ok {
			for endpoint, count := range errorsMap {
				if countFloat, ok := count.(float64); ok {
					errorsByEndpoint[endpoint] = int64(countFloat)
				}
			}
		}
	}

	// Collect all unique endpoint names
	allEndpoints := make(map[string]bool)
	for endpoint := range successByEndpoint {
		allEndpoints[endpoint] = true
	}
	for endpoint := range droppedByEndpoint {
		allEndpoints[endpoint] = true
	}
	for endpoint := range requeuedByEndpoint {
		allEndpoints[endpoint] = true
	}
	for endpoint := range errorsByEndpoint {
		allEndpoints[endpoint] = true
	}

	// Build URLs and aggregate counts by unique URL
	// This deduplicates endpoints that map to the same URL (e.g., "process" and "rtprocess")
	successByURL := make(map[string]int64)
	droppedByURL := make(map[string]int64)
	requeuedByURL := make(map[string]int64)
	errorsByURL := make(map[string]int64)

	for endpointName := range allEndpoints {
		url := constructEndpointURL(endpointName, d.config)

		// Aggregate counts for this URL
		successByURL[url] += successByEndpoint[endpointName]
		droppedByURL[url] += droppedByEndpoint[endpointName]
		requeuedByURL[url] += requeuedByEndpoint[endpointName]
		errorsByURL[url] += errorsByEndpoint[endpointName]
	}

	// Create sorted list of unique URLs for consistent ordering
	uniqueURLs := make([]string, 0)
	seenURLs := make(map[string]bool)

	// Collect from all maps to ensure we get all URLs
	for url := range successByURL {
		if !seenURLs[url] {
			uniqueURLs = append(uniqueURLs, url)
			seenURLs[url] = true
		}
	}
	for url := range droppedByURL {
		if !seenURLs[url] {
			uniqueURLs = append(uniqueURLs, url)
			seenURLs[url] = true
		}
	}
	for url := range requeuedByURL {
		if !seenURLs[url] {
			uniqueURLs = append(uniqueURLs, url)
			seenURLs[url] = true
		}
	}
	for url := range errorsByURL {
		if !seenURLs[url] {
			uniqueURLs = append(uniqueURLs, url)
			seenURLs[url] = true
		}
	}
	slices.Sort(uniqueURLs)

	// Create endpoint status entries in sorted order (one per unique URL)
	for _, url := range uniqueURLs {
		successCount := successByURL[url]
		droppedCount := droppedByURL[url]
		requeuedCount := requeuedByURL[url]
		errorCount := errorsByURL[url]

		endpointStatus := def.EndpointStatus{
			Name:          url, // Use URL as name for consistency
			URL:           url,
			SuccessCount:  successCount,
			FailureCount:  droppedCount,
			RequeuedCount: requeuedCount,
			ErrorCount:    errorCount,
		}

		// Determine status based on success/failure counts
		if successCount > 0 && droppedCount == 0 {
			endpointStatus.Status = "connected"
			endpointStatus.LastSuccess = time.Now()
		} else if droppedCount > 0 && successCount == 0 {
			endpointStatus.Status = "error"
			endpointStatus.LastFailure = time.Now()
		} else if successCount > 0 && droppedCount > 0 {
			endpointStatus.Status = "connected" // Some successes
			endpointStatus.LastSuccess = time.Now()
			endpointStatus.LastFailure = time.Now()
		} else {
			endpointStatus.Status = "unknown"
		}

		status.Endpoints = append(status.Endpoints, endpointStatus)
	}

	return status
}

// constructEndpointURL constructs the full intake URL for a given endpoint name
func constructEndpointURL(endpointName string, cfg config.Component) string {
	// Get the configured site (defaults to datadoghq.com)
	site := cfg.GetString("site")
	if site == "" {
		site = "datadoghq.com"
	}

	// Map endpoint names to their full URLs
	// These follow the standard Datadog intake URL patterns
	switch endpointName {
	// Metrics series endpoints
	case "series_v1":
		return fmt.Sprintf("https://api.%s/api/v1/series", site)
	case "series_v2":
		return fmt.Sprintf("https://api.%s/api/v2/series", site)

	// Metrics distribution/sketches endpoints
	case "sketches_v1":
		return fmt.Sprintf("https://api.%s/api/v1/distribution_points", site)
	case "sketches_v2":
		return fmt.Sprintf("https://api.%s/api/v2/distribution_points", site)

	// Logs intake
	case "intake":
		return fmt.Sprintf("https://http-intake.logs.%s/api/v2/logs", site)

	// Process monitoring endpoints
	case "process":
		return fmt.Sprintf("https://process.%s/api/v1/collector", site)
	case "rtprocess":
		return fmt.Sprintf("https://process.%s/api/v1/collector", site)
	case "container":
		return fmt.Sprintf("https://process.%s/api/v1/container", site)
	case "rtcontainer":
		return fmt.Sprintf("https://process.%s/api/v1/container", site)
	case "connections":
		return fmt.Sprintf("https://process.%s/api/v1/connections", site)

	// Orchestrator
	case "orchestrator":
		return fmt.Sprintf("https://orchestrator.%s/api/v1/orchestrator", site)

	// Check runs and service checks
	case "check_run_v1":
		return fmt.Sprintf("https://api.%s/api/v1/check_run", site)
	case "services_checks_v2":
		return fmt.Sprintf("https://api.%s/api/v2/service_checks", site)

	// Metadata endpoints
	case "metadata_v1":
		return fmt.Sprintf("https://api.%s/api/v1/metadata", site)
	case "host_metadata_v2":
		return fmt.Sprintf("https://api.%s/api/v2/host_metadata", site)

	// Events
	case "events_v2":
		return fmt.Sprintf("https://api.%s/api/v2/events", site)

	// API key validation
	case "validate_v1":
		return fmt.Sprintf("https://api.%s/api/v1/validate", site)

	// Trace endpoints (APM)
	case "v0.4/traces", "v0.5/traces", "v0.6/traces", "v0.7/traces":
		return fmt.Sprintf("https://trace.agent.%s/api/%s", site, endpointName)

	// Default: return a generic API URL with the endpoint name
	default:
		return fmt.Sprintf("https://api.%s/api/%s", site, endpointName)
	}
}

// collectHealthStatus uses the health package to get component health
func (d *doctorImpl) collectHealthStatus() def.HealthStatus {
	h := health.GetReady()

	// Sort healthy and unhealthy components by name
	slices.Sort(h.Healthy)
	slices.Sort(h.Unhealthy)

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

	// Collect DogStatsD metrics stats by service from aggregator
	d.collectDogStatsDServiceStats(serviceMap)

	// Collect logs stats by service from log agent
	d.collectLogsServiceStats(serviceMap)

	// Convert map to sorted slice
	services := make([]def.ServiceStats, 0, len(serviceMap))
	// var hasOther bool
	for _, stats := range serviceMap {
		// if name == "" {
		// 	hasOther = true
		// 	continue
		// }
		services = append(services, *stats)
	}
	// if hasOther {
	// 	services = append(services, *serviceMap[""])
	// }

	// Sort by service name for consistent ordering
	// (Could also sort by total activity: traces + metrics + logs)

	slices.SortFunc(services, func(a, b def.ServiceStats) int {
		// Always put empty name at the end
		if a.Name == "" {
			return 1
		}
		if b.Name == "" {
			return -1
		}

		return strings.Compare(a.Name, b.Name)
	})

	// for i := 0; i < len(services); i++ {
	// 	for j := i + 1; j < len(services); j++ {
	// 		if services[i].Name > services[j].Name {
	// 			services[i], services[j] = services[j], services[i]
	// 		}
	// 	}
	// }

	return services
}

// collectTraceServiceStats collects trace rates per service from trace-agent expvars
// Uses delta tracking to calculate instantaneous rates instead of cumulative counts
func (d *doctorImpl) collectTraceServiceStats(serviceMap map[string]*def.ServiceStats) {
	// Get trace-agent debug port from config
	traceAgentPort := d.config.GetInt("apm_config.debug.port")
	if traceAgentPort == 0 {
		traceAgentPort = 5012 // Default trace-agent debug port
	}

	// Fetch expvars from trace-agent via HTTP
	url := fmt.Sprintf("https://localhost:%d/debug/vars", traceAgentPort)

	resp, err := d.httpclient.Get(url)
	if err != nil {
		// Trace-agent may not be running or not accessible
		d.log.Debugf("Failed to fetch trace-agent expvars from %s: %v", url, err)
		return
	}

	// Parse the full expvar response
	var expvarData map[string]interface{}
	if err := json.Unmarshal(resp, &expvarData); err != nil {
		d.log.Debugf("Failed to unmarshal trace-agent expvars: %v", err)
		return
	}

	// Extract the receiver stats
	receiverInterface, ok := expvarData["receiver"]
	if !ok {
		d.log.Debugf("No 'receiver' key in trace-agent expvars")
		return
	}

	receiverJSON, err := json.Marshal(receiverInterface)
	if err != nil {
		d.log.Debugf("Failed to marshal receiver stats: %v", err)
		return
	}

	var receiverStats []map[string]interface{}
	if err := json.Unmarshal(receiverJSON, &receiverStats); err != nil {
		d.log.Debugf("Failed to unmarshal trace receiver stats: %v", err)
		return
	}

	// Lock for thread-safe access to delta tracking state
	d.tracesDeltaMu.Lock()
	defer d.tracesDeltaMu.Unlock()

	// Calculate time delta since last collection
	now := time.Now()
	timeDelta := now.Sub(d.lastTracesCollectionTime).Seconds()

	// Collect current traces received per service
	// Note: Same service can appear multiple times with different Lang/TracerVersion
	// so we need to aggregate across all TagStats entries
	currentTracesReceived := make(map[string]int64)

	for _, tagStats := range receiverStats {
		// Service name is at the top level, not nested under "Tags"
		serviceName, hasService := tagStats["Service"].(string)
		if !hasService || serviceName == "" {
			continue
		}

		// TracesReceived is also at the top level
		tracesReceived, ok := tagStats["TracesReceived"].(float64)
		if !ok {
			continue
		}

		currentTracesReceived[serviceName] += int64(tracesReceived)
	}

	// First collection or time delta too small - use average rate
	if d.lastTracesCollectionTime.IsZero() || timeDelta < 0.1 {
		uptimeSeconds := time.Since(d.startTime).Seconds()
		if uptimeSeconds == 0 {
			uptimeSeconds = 1
		}

		for serviceName, currentTraces := range currentTracesReceived {
			if currentTraces > 0 {
				if _, exists := serviceMap[serviceName]; !exists {
					serviceMap[serviceName] = &def.ServiceStats{Name: serviceName}
				}
				tracesRate := float64(currentTraces) / uptimeSeconds
				serviceMap[serviceName].TracesRate += tracesRate

				// Store for next iteration
				d.previousTracesReceived[serviceName] = currentTraces
			}
		}

		d.lastTracesCollectionTime = now
		return
	}

	// Calculate instantaneous rates using deltas
	for serviceName, currentTraces := range currentTracesReceived {
		previousTraces, hasPrevious := d.previousTracesReceived[serviceName]

		var tracesRate float64
		if hasPrevious && currentTraces >= previousTraces {
			// Calculate instantaneous rate from delta
			deltaTraces := currentTraces - previousTraces
			tracesRate = float64(deltaTraces) / timeDelta
		} else {
			// No previous data or counter reset - fallback to average
			uptimeSeconds := time.Since(d.startTime).Seconds()
			if uptimeSeconds > 0 {
				tracesRate = float64(currentTraces) / uptimeSeconds
			}
		}

		// Get or create service entry
		if _, exists := serviceMap[serviceName]; !exists {
			serviceMap[serviceName] = &def.ServiceStats{Name: serviceName}
		}
		serviceMap[serviceName].TracesRate += tracesRate

		// Update tracking state
		d.previousTracesReceived[serviceName] = currentTraces
	}

	// Update last collection time
	d.lastTracesCollectionTime = now
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

// collectDogStatsDServiceStats collects DogStatsD metric rates per service from aggregator
// Uses delta tracking to calculate instantaneous rates instead of average-since-start
func (d *doctorImpl) collectDogStatsDServiceStats(serviceMap map[string]*def.ServiceStats) {
	// Get aggregator expvars
	aggregatorVar := expvar.Get("aggregator")
	if aggregatorVar == nil {
		return
	}

	aggregatorJSON := []byte(aggregatorVar.String())
	var aggregatorStats map[string]interface{}
	if err := json.Unmarshal(aggregatorJSON, &aggregatorStats); err != nil {
		d.log.Debugf("Failed to unmarshal aggregator stats: %v", err)
		return
	}

	// Get DogStatsD service stats map
	serviceStatsInterface, ok := aggregatorStats["DogstatsdServiceStats"]
	if !ok {
		return
	}

	serviceStatsMap, ok := serviceStatsInterface.(map[string]interface{})
	if !ok {
		return
	}

	// Lock for thread-safe access to delta tracking state
	d.dogstatsdDeltaMu.Lock()
	defer d.dogstatsdDeltaMu.Unlock()

	// Calculate time delta since last collection
	now := time.Now()
	timeDelta := now.Sub(d.lastDogstatsdCollectionTime).Seconds()

	// If this is the first collection or time delta is too small, fall back to average rate
	if d.lastDogstatsdCollectionTime.IsZero() || timeDelta < 0.1 {
		// First collection - use average rate since agent start as fallback
		uptimeSeconds := time.Since(d.startTime).Seconds()
		if uptimeSeconds == 0 {
			uptimeSeconds = 1
		}

		for serviceName, countInterface := range serviceStatsMap {
			var currentCount int64
			switch v := countInterface.(type) {
			case float64:
				currentCount = int64(v)
			case int64:
				currentCount = v
			default:
				continue
			}

			if currentCount > 0 {
				if _, exists := serviceMap[serviceName]; !exists {
					serviceMap[serviceName] = &def.ServiceStats{Name: serviceName}
				}
				metricsRate := float64(currentCount) / uptimeSeconds
				serviceMap[serviceName].MetricsRate += metricsRate

				// Store current value for next iteration
				d.previousDogstatsdSamples[serviceName] = currentCount
			}
		}

		d.lastDogstatsdCollectionTime = now
		return
	}

	// We have previous data - calculate instantaneous rates using deltas
	for serviceName, countInterface := range serviceStatsMap {
		var currentCount int64
		switch v := countInterface.(type) {
		case float64:
			currentCount = int64(v)
		case int64:
			currentCount = v
		default:
			continue
		}

		previousCount, hasPrevious := d.previousDogstatsdSamples[serviceName]

		var metricsRate float64
		if hasPrevious && currentCount >= previousCount {
			// Calculate instantaneous rate from delta
			deltaCount := currentCount - previousCount
			metricsRate = float64(deltaCount) / timeDelta
		} else {
			// No previous data or counter reset - use average since start
			uptimeSeconds := time.Since(d.startTime).Seconds()
			if uptimeSeconds > 0 {
				metricsRate = float64(currentCount) / uptimeSeconds
			}
		}

		// Get or create service entry
		if _, exists := serviceMap[serviceName]; !exists {
			serviceMap[serviceName] = &def.ServiceStats{Name: serviceName}
		}
		serviceMap[serviceName].MetricsRate += metricsRate

		// Update tracking state
		d.previousDogstatsdSamples[serviceName] = currentCount
	}

	// Update last collection time
	d.lastDogstatsdCollectionTime = now
}

// Helper to safely get check ID from various check stat types
func getCheckID(stats interface{}) checkid.ID {
	if s, ok := stats.(*checkstats.Stats); ok {
		return s.CheckID
	}
	return checkid.ID("")
}
