// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// TestBenchConfig configures the test bench.
type TestBenchConfig struct {
	ScenariosDir string
	HTTPAddr     string
	Recorder     recorderdef.Component // Optional: for loading parquet scenarios

	// EnableOverrides controls which components are enabled at startup.
	// Keys are component names (e.g. "cusum", "lead_lag", "dedup").
	// If a name is present, its value overrides the registry DefaultEnabled.
	// Components not listed use their registry default.
	EnableOverrides map[string]bool

	// Config params (not component toggles)
	CUSUMIncludeCount bool // CUSUM: include :count metrics (default: false, skips them)
}

// TestBench is the main controller for the observer test bench.
// It manages scenarios, components, and analysis results.
type TestBench struct {
	config TestBenchConfig

	mu             sync.RWMutex
	storage        *timeSeriesStorage
	loadedScenario string
	ready          bool

	// Components (log processors are not registry-managed)
	logProcessors []observerdef.LogProcessor

	// Registry-managed components
	components map[string]*registeredComponent

	// Results (computed eagerly on scenario load)
	anomalies    []observerdef.AnomalyOutput            // all anomalies from TS analyses
	correlations []observerdef.ActiveCorrelation        // from anomaly processors
	byAnalyzer   map[string][]observerdef.AnomalyOutput // anomalies grouped by analyzer

	// API server
	api *TestBenchAPI
}

// ScenarioInfo describes an available scenario.
type ScenarioInfo struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	HasParquet bool   `json:"hasParquet"`
	HasLogs    bool   `json:"hasLogs"`
	HasEvents  bool   `json:"hasEvents"`
}

// ComponentInfo describes a registered component.
type ComponentInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Category    string `json:"category"` // "analyzer", "correlator", "processing"
	Enabled     bool   `json:"enabled"`
}

// StatusResponse is the response for /api/status.
type StatusResponse struct {
	Ready          bool         `json:"ready"`
	Scenario       string       `json:"scenario,omitempty"`
	SeriesCount    int          `json:"seriesCount"`
	AnomalyCount   int          `json:"anomalyCount"`
	ComponentCount int          `json:"componentCount"`
	ServerConfig   ServerConfig `json:"serverConfig"`
}

// ServerConfig exposes server-side configuration to the UI.
type ServerConfig struct {
	Components     map[string]bool `json:"components"`
	CUSUMSkipCount bool            `json:"cusumSkipCount"` // true = filtering out :count metrics
}

// NewTestBench creates a new test bench instance.
func NewTestBench(config TestBenchConfig) (*TestBench, error) {
	// Verify scenarios directory exists
	if _, err := os.Stat(config.ScenariosDir); os.IsNotExist(err) {
		// Create it if it doesn't exist
		if err := os.MkdirAll(config.ScenariosDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create scenarios directory: %w", err)
		}
	}

	if config.EnableOverrides == nil {
		config.EnableOverrides = make(map[string]bool)
	}

	tb := &TestBench{
		config:  config,
		storage: newTimeSeriesStorage(),

		// Log processors are not registry-managed
		logProcessors: []observerdef.LogProcessor{
			&LogTimeSeriesAnalysis{
				MaxEvalBytes: 4096,
				ExcludeFields: map[string]struct{}{
					"timestamp": {}, "ts": {}, "time": {},
					"pid": {}, "ppid": {}, "uid": {}, "gid": {},
				},
			},
			&ConnectionErrorExtractor{},
		},

		components: make(map[string]*registeredComponent),
		anomalies:  []observerdef.AnomalyOutput{},
		byAnalyzer: make(map[string][]observerdef.AnomalyOutput),
	}

	// Instantiate all registry components
	for _, reg := range defaultRegistry {
		enabled := reg.DefaultEnabled
		if override, ok := config.EnableOverrides[reg.Name]; ok {
			enabled = override
		}
		tb.components[reg.Name] = &registeredComponent{
			Registration: reg,
			Instance:     reg.Factory(tb),
			Enabled:      enabled,
		}
	}

	tb.api = NewTestBenchAPI(tb)

	return tb, nil
}

// Start starts the test bench HTTP server.
func (tb *TestBench) Start() error {
	return tb.api.Start(tb.config.HTTPAddr)
}

// Stop stops the test bench HTTP server.
func (tb *TestBench) Stop() error {
	return tb.api.Stop()
}

// ListScenarios returns all available scenarios.
func (tb *TestBench) ListScenarios() ([]ScenarioInfo, error) {
	entries, err := os.ReadDir(tb.config.ScenariosDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read scenarios directory: %w", err)
	}

	var scenarios []ScenarioInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		scenarioPath := filepath.Join(tb.config.ScenariosDir, entry.Name())
		info := ScenarioInfo{
			Name: entry.Name(),
			Path: scenarioPath,
		}

		// Check for subdirectories
		if _, err := os.Stat(filepath.Join(scenarioPath, "parquet")); err == nil {
			info.HasParquet = true
		}
		if _, err := os.Stat(filepath.Join(scenarioPath, "logs")); err == nil {
			info.HasLogs = true
		}
		if _, err := os.Stat(filepath.Join(scenarioPath, "events")); err == nil {
			info.HasEvents = true
		}

		// Also check for parquet files directly in the scenario directory
		if !info.HasParquet {
			if files, _ := filepath.Glob(filepath.Join(scenarioPath, "*.parquet")); len(files) > 0 {
				info.HasParquet = true
			}
		}

		scenarios = append(scenarios, info)
	}

	// Sort by name
	sort.Slice(scenarios, func(i, j int) bool {
		return scenarios[i].Name < scenarios[j].Name
	})

	return scenarios, nil
}

// LoadScenario loads a scenario by name, clearing any previously loaded data.
func (tb *TestBench) LoadScenario(name string) error {
	// Special handling for built-in demo scenario
	if name == "demo" {
		return tb.loadDemoScenario()
	}

	scenarioPath := filepath.Join(tb.config.ScenariosDir, name)

	// Verify scenario exists
	if _, err := os.Stat(scenarioPath); os.IsNotExist(err) {
		return fmt.Errorf("scenario not found: %s", name)
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Clear existing data
	tb.storage = newTimeSeriesStorage()
	tb.anomalies = []observerdef.AnomalyOutput{}
	tb.correlations = []observerdef.ActiveCorrelation{}
	tb.byAnalyzer = make(map[string][]observerdef.AnomalyOutput)
	tb.ready = false
	tb.loadedScenario = name

	// Reset ALL correlators (not just enabled) so disabled ones clear stale state
	tb.resetAllProcessors()

	// Load data from scenario
	scenarioStart := time.Now()
	fmt.Printf("Loading scenario: %s\n", name)

	// Load parquet files
	parquetDir := filepath.Join(scenarioPath, "parquet")
	parquetStart := time.Now()
	if _, err := os.Stat(parquetDir); err == nil {
		if err := tb.loadParquetDir(parquetDir); err != nil {
			return fmt.Errorf("failed to load parquet data: %w", err)
		}
	} else {
		// Check for parquet files directly in scenario directory
		if files, _ := filepath.Glob(filepath.Join(scenarioPath, "*.parquet")); len(files) > 0 {
			if err := tb.loadParquetDir(scenarioPath); err != nil {
				return fmt.Errorf("failed to load parquet data: %w", err)
			}
		}
	}
	fmt.Printf("  Parquet loading took %s\n", time.Since(parquetStart))

	// Load log files
	logsDir := filepath.Join(scenarioPath, "logs")
	logsStart := time.Now()
	if _, err := os.Stat(logsDir); err == nil {
		if err := tb.loadLogsDir(logsDir); err != nil {
			return fmt.Errorf("failed to load logs: %w", err)
		}
	}
	fmt.Printf("  Log loading took %s\n", time.Since(logsStart))

	// Load event files
	eventsDir := filepath.Join(scenarioPath, "events")
	eventsStart := time.Now()
	if _, err := os.Stat(eventsDir); err == nil {
		if err := tb.loadEventsDir(eventsDir); err != nil {
			return fmt.Errorf("failed to load events: %w", err)
		}
	}
	fmt.Printf("  Event loading took %s\n", time.Since(eventsStart))

	// Run analyses on all loaded data
	analysisStart := time.Now()
	tb.runAnalyses()
	fmt.Printf("  Analyses took %s\n", time.Since(analysisStart))
	fmt.Printf("  Total scenario load took %s\n", time.Since(scenarioStart))

	tb.ready = true
	fmt.Printf("Scenario loaded: %d series, %d anomalies\n", tb.seriesCount(), len(tb.anomalies))

	return nil
}

// loadParquetDir loads all parquet files from a directory using the recorder component.
// Uses batch loading for efficiency - reads all metrics at once instead of streaming.
func (tb *TestBench) loadParquetDir(dir string) error {
	if tb.config.Recorder == nil {
		return fmt.Errorf("recorder component not configured - cannot load parquet files")
	}

	// Use batch loading - get all metrics at once
	metrics, err := tb.config.Recorder.ReadAllMetrics(dir)
	if err != nil {
		return fmt.Errorf("reading parquet metrics: %w", err)
	}

	fmt.Printf("  Loading %d samples from parquet files\n", len(metrics))

	// Batch add all metrics to storage
	for _, m := range metrics {
		// Strip aggregation suffix from metric name (e.g., ":avg", ":count")
		metricName := m.Name
		if idx := strings.LastIndex(metricName, ":"); idx != -1 {
			suffix := metricName[idx+1:]
			if suffix == "avg" || suffix == "count" || suffix == "sum" || suffix == "min" || suffix == "max" {
				metricName = metricName[:idx]
			}
		}

		tb.storage.Add(
			"parquet", // namespace
			metricName,
			m.Value,
			m.Timestamp,
			m.Tags,
		)
	}

	return nil
}

// loadLogsDir loads all log files from a directory.
func (tb *TestBench) loadLogsDir(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		return err
	}

	totalLogs := 0
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil || info.IsDir() {
			continue
		}

		logs, err := LoadLogFile(file)
		if err != nil {
			fmt.Printf("  Warning: failed to load log file %s: %v\n", filepath.Base(file), err)
			continue
		}

		for _, log := range logs {
			// Get timestamp from testLogView
			var timestamp int64
			if tlv, ok := log.(*testLogView); ok {
				timestamp = tlv.timestamp
			}

			// Process through log processors
			for _, processor := range tb.logProcessors {
				result := processor.Process(log)
				for _, m := range result.Metrics {
					tb.storage.Add("logs", m.Name, m.Value, timestamp, m.Tags)
				}
			}
		}
		totalLogs += len(logs)
	}

	if totalLogs > 0 {
		fmt.Printf("  Loaded %d log entries\n", totalLogs)
	}
	return nil
}

// loadEventsDir loads all event files from a directory.
func (tb *TestBench) loadEventsDir(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}

	enabledProcs := tb.enabledCorrelators()

	totalEvents := 0
	for _, file := range files {
		events, err := LoadEventFile(file)
		if err != nil {
			fmt.Printf("  Warning: failed to load event file %s: %v\n", filepath.Base(file), err)
			continue
		}

		// Send events to enabled anomaly processors that support them
		for _, event := range events {
			for _, proc := range enabledProcs {
				if receiver, ok := proc.(observerdef.EventSignalReceiver); ok {
					receiver.AddEventSignal(event)
				}
			}
		}
		totalEvents += len(events)
	}

	if totalEvents > 0 {
		fmt.Printf("  Loaded %d events\n", totalEvents)
	}
	return nil
}

// resetAllProcessors resets all correlators and the deduplicator.
func (tb *TestBench) resetAllProcessors() {
	for _, proc := range tb.allCorrelators() {
		if resetter, ok := proc.(interface{ Reset() }); ok {
			resetter.Reset()
		} else {
			proc.Flush()
		}
	}
	if dedup := tb.getDeduplicator(); dedup != nil {
		dedup.Reset()
	}
}

// runAnalyses runs all time series analyses on all stored series.
func (tb *TestBench) runAnalyses() {
	analyzers := tb.enabledAnalyzers()
	correlators := tb.enabledCorrelators()
	dedup := tb.getDeduplicator()

	// Collect all series from all namespaces
	collectStart := time.Now()
	var allSeries []observerdef.Series
	for _, agg := range []Aggregate{AggregateAverage, AggregateCount} {
		for _, ns := range tb.storage.Namespaces() {
			series := tb.storage.AllSeries(ns, agg)
			for _, s := range series {
				sCopy := s
				sCopy.Name = s.Name + ":" + aggSuffix(agg)
				allSeries = append(allSeries, sCopy)
			}
		}
	}
	fmt.Printf("    Collect series: %s\n", time.Since(collectStart))

	fmt.Printf("  Running analyses on %d series\n", len(allSeries))

	// Run analyses
	analyzerTime := make(map[string]time.Duration)
	processorTime := make(map[string]time.Duration)
	dedupedCount := 0
	for _, series := range allSeries {
		for _, analysis := range analyzers {
			t0 := time.Now()
			result := analysis.Analyze(series)
			analyzerTime[analysis.Name()] += time.Since(t0)
			for _, anomaly := range result.Anomalies {
				anomaly.AnalyzerName = analysis.Name()
				tb.anomalies = append(tb.anomalies, anomaly)

				// Group by analyzer
				tb.byAnalyzer[anomaly.AnalyzerName] = append(tb.byAnalyzer[anomaly.AnalyzerName], anomaly)

				// Apply deduplication if enabled
				if dedup != nil {
					if !dedup.ShouldProcess(anomaly.Source, anomaly.Timestamp) {
						dedupedCount++
						continue // Skip duplicate
					}
				}

				// Send to enabled anomaly processors
				for _, proc := range correlators {
					t1 := time.Now()
					proc.Process(anomaly)
					processorTime[proc.Name()] += time.Since(t1)
				}
			}
		}
	}

	for name, d := range analyzerTime {
		fmt.Printf("    Analyzer %s: %s\n", name, d)
	}
	for name, d := range processorTime {
		fmt.Printf("    Processor %s: %s\n", name, d)
	}

	if dedup != nil && dedupedCount > 0 {
		fmt.Printf("    Deduplication: filtered %d duplicate anomalies\n", dedupedCount)
	}

	// Flush processors to get correlations
	flushStart := time.Now()
	for _, proc := range correlators {
		proc.Flush()
	}
	fmt.Printf("    Processor flush: %s\n", time.Since(flushStart))

	// Collect correlations from processors that support it
	collectCorrelationsStart := time.Now()
	for _, proc := range correlators {
		if cs, ok := proc.(interface {
			ActiveCorrelations() []observerdef.ActiveCorrelation
		}); ok {
			tb.correlations = append(tb.correlations, cs.ActiveCorrelations()...)
		}
	}
	fmt.Printf("    Collect correlations: %s\n", time.Since(collectCorrelationsStart))
}

// GetStatus returns the current status.
func (tb *TestBench) GetStatus() StatusResponse {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	compMap := make(map[string]bool)
	for name, comp := range tb.components {
		compMap[name] = comp.Enabled
	}

	return StatusResponse{
		Ready:          tb.ready,
		Scenario:       tb.loadedScenario,
		SeriesCount:    tb.seriesCount(),
		AnomalyCount:   len(tb.anomalies),
		ComponentCount: len(tb.logProcessors) + len(tb.components),
		ServerConfig: ServerConfig{
			Components:     compMap,
			CUSUMSkipCount: !tb.config.CUSUMIncludeCount,
		},
	}
}

// ConfigUpdateRequest is the request body for POST /api/config.
type ConfigUpdateRequest struct {
	CUSUMSkipCount *bool `json:"cusumSkipCount,omitempty"`
	DedupEnabled   *bool `json:"dedupEnabled,omitempty"`
}

// UpdateConfigAndReanalyze updates configuration and re-runs analyses.
func (tb *TestBench) UpdateConfigAndReanalyze(req ConfigUpdateRequest) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	configChanged := false

	// Update CUSUM skip count
	if req.CUSUMSkipCount != nil {
		newSkip := *req.CUSUMSkipCount
		oldSkip := !tb.config.CUSUMIncludeCount
		if newSkip != oldSkip {
			tb.config.CUSUMIncludeCount = !newSkip
			if comp, ok := tb.components["cusum"]; ok {
				if cusum, ok := comp.Instance.(*CUSUMDetector); ok {
					cusum.SkipCountMetrics = newSkip
				}
			}
			configChanged = true
		}
	}

	// Update dedup
	if req.DedupEnabled != nil {
		if comp, ok := tb.components["dedup"]; ok {
			if *req.DedupEnabled != comp.Enabled {
				comp.Enabled = *req.DedupEnabled
				if comp.Enabled {
					// Reset the deduplicator when re-enabling
					if d, ok := comp.Instance.(*AnomalyDeduplicator); ok {
						d.Reset()
					}
				}
				configChanged = true
			}
		}
	}

	// Re-run analyses if config changed and scenario is loaded
	if configChanged && tb.ready && tb.storage != nil {
		tb.rerunAnalysesLocked()
	}

	return nil
}

// rerunAnalysesLocked re-runs all analyses on current data. Caller must hold lock.
func (tb *TestBench) rerunAnalysesLocked() {
	// Clear existing results
	tb.anomalies = tb.anomalies[:0]
	tb.correlations = tb.correlations[:0]
	for k := range tb.byAnalyzer {
		delete(tb.byAnalyzer, k)
	}

	// Reset ALL correlators (not just enabled) so disabled ones clear stale state
	tb.resetAllProcessors()

	analyzers := tb.enabledAnalyzers()
	correlators := tb.enabledCorrelators()
	dedup := tb.getDeduplicator()

	// Re-run time series analyses
	for _, ns := range tb.storage.Namespaces() {
		for _, agg := range []Aggregate{AggregateAverage, AggregateCount} {
			allSeries := tb.storage.AllSeries(ns, agg)
			for _, series := range allSeries {
				seriesCopy := series
				seriesCopy.Name = series.Name + ":" + aggSuffix(agg)

				for _, analyzer := range analyzers {
					result := analyzer.Analyze(seriesCopy)
					for _, anomaly := range result.Anomalies {
						anomaly.AnalyzerName = analyzer.Name()
						anomaly.Source = seriesCopy.Name

						// Apply deduplication if enabled
						if dedup != nil {
							if !dedup.ShouldProcess(anomaly.Source, anomaly.Timestamp) {
								continue
							}
						}

						tb.anomalies = append(tb.anomalies, anomaly)
						tb.byAnalyzer[anomaly.AnalyzerName] = append(tb.byAnalyzer[anomaly.AnalyzerName], anomaly)

						// Send to enabled anomaly processors
						for _, proc := range correlators {
							proc.Process(anomaly)
						}
					}
				}
			}
		}
	}

	// Flush processors to get correlations
	for _, proc := range correlators {
		proc.Flush()
	}

	// Collect correlations
	for _, proc := range correlators {
		if cs, ok := proc.(interface {
			ActiveCorrelations() []observerdef.ActiveCorrelation
		}); ok {
			tb.correlations = append(tb.correlations, cs.ActiveCorrelations()...)
		}
	}
}

// GetComponents returns all registered components.
func (tb *TestBench) GetComponents() []ComponentInfo {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	var components []ComponentInfo

	// Return registry components in registry order
	for _, reg := range defaultRegistry {
		comp := tb.components[reg.Name]
		if comp == nil {
			continue
		}
		components = append(components, ComponentInfo{
			Name:        reg.Name,
			DisplayName: reg.DisplayName,
			Category:    reg.Category,
			Enabled:     comp.Enabled,
		})
	}

	return components
}

// GetStorage returns the storage (for API handlers).
func (tb *TestBench) GetStorage() *timeSeriesStorage {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return tb.storage
}

// GetAnomalies returns all detected anomalies.
func (tb *TestBench) GetAnomalies() []observerdef.AnomalyOutput {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	result := make([]observerdef.AnomalyOutput, len(tb.anomalies))
	copy(result, tb.anomalies)
	return result
}

// GetAnomaliesByAnalyzer returns anomalies grouped by analyzer name.
func (tb *TestBench) GetAnomaliesByAnalyzer() map[string][]observerdef.AnomalyOutput {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	result := make(map[string][]observerdef.AnomalyOutput)
	for k, v := range tb.byAnalyzer {
		copied := make([]observerdef.AnomalyOutput, len(v))
		copy(copied, v)
		result[k] = copied
	}
	return result
}

// GetCorrelations returns all detected correlations.
func (tb *TestBench) GetCorrelations() []observerdef.ActiveCorrelation {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	result := make([]observerdef.ActiveCorrelation, len(tb.correlations))
	copy(result, tb.correlations)
	return result
}

// seriesCount returns the number of unique series (must be called with lock held).
func (tb *TestBench) seriesCount() int {
	if tb.storage == nil {
		return 0
	}
	return len(tb.storage.series)
}

// GetLeadLagEdges returns lead-lag edges if the correlator is enabled.
func (tb *TestBench) GetLeadLagEdges() ([]LeadLagEdge, bool) {
	data, enabled := tb.GetCorrelatorData("lead_lag")
	if edges, ok := data.([]LeadLagEdge); ok {
		return edges, enabled
	}
	return nil, enabled
}

// GetSurpriseEdges returns surprise edges if the correlator is enabled.
func (tb *TestBench) GetSurpriseEdges() ([]SurpriseEdge, bool) {
	data, enabled := tb.GetCorrelatorData("surprise")
	if edges, ok := data.([]SurpriseEdge); ok {
		return edges, enabled
	}
	return nil, enabled
}

// GetGraphSketchEdges returns graph sketch edges if the correlator is enabled.
func (tb *TestBench) GetGraphSketchEdges() ([]EdgeInfo, bool) {
	data, enabled := tb.GetCorrelatorData("graph_sketch")
	if edges, ok := data.([]EdgeInfo); ok {
		return edges, enabled
	}
	return nil, enabled
}

// GetCorrelatorStats returns stats from all correlators (enabled or not).
func (tb *TestBench) GetCorrelatorStats() map[string]interface{} {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	stats := make(map[string]interface{})
	for name, comp := range tb.components {
		if comp.Registration.Category == "correlator" {
			if statter, ok := comp.Instance.(interface {
				GetStats() map[string]interface{}
			}); ok {
				stats[name] = statter.GetStats()
			}
		}
	}
	return stats
}

// ToggleComponent toggles a component's enabled state and re-runs analyses if needed.
func (tb *TestBench) ToggleComponent(name string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	comp, ok := tb.components[name]
	if !ok {
		return fmt.Errorf("unknown component: %s", name)
	}

	comp.Enabled = !comp.Enabled

	// Re-run analyses if a scenario is loaded
	if tb.ready && tb.storage != nil {
		tb.rerunAnalysesLocked()
	}

	return nil
}

// loadDemoScenario generates synthetic demo data directly into storage.
func (tb *TestBench) loadDemoScenario() error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Clear existing data
	tb.storage = newTimeSeriesStorage()
	tb.anomalies = []observerdef.AnomalyOutput{}
	tb.correlations = []observerdef.ActiveCorrelation{}
	tb.byAnalyzer = make(map[string][]observerdef.AnomalyOutput)
	tb.ready = false
	tb.loadedScenario = "demo"

	// Reset all processors
	tb.resetAllProcessors()

	fmt.Println("Generating demo scenario data...")

	// Generate data for each second of the 70-second scenario
	baseTimestamp := int64(1000000)
	const totalSeconds = 70

	for t := 0; t < totalSeconds; t++ {
		elapsed := float64(t)
		timestamp := baseTimestamp + int64(t)

		// Heap usage
		heapValue := getDemoHeapValue(elapsed)
		tb.storage.Add("demo", "runtime.heap.used_mb", heapValue, timestamp, nil)

		// GC pause time
		gcValue := getDemoGCPauseValue(elapsed)
		tb.storage.Add("demo", "runtime.gc.pause_ms", gcValue, timestamp, nil)

		// Request latency
		latencyValue := getDemoLatencyValue(elapsed)
		tb.storage.Add("demo", "app.request.latency_p99_ms", latencyValue, timestamp, nil)

		// Error rate
		errorValue := getDemoErrorRateValue(elapsed)
		tb.storage.Add("demo", "app.request.error_rate", errorValue, timestamp, nil)

		// CPU usage
		cpuValue := getDemoCPUValue(elapsed)
		tb.storage.Add("demo", "system.cpu.user_percent", cpuValue, timestamp, nil)

		// Throughput (drops during incident)
		throughputValue := getDemoThroughputValue(elapsed)
		tb.storage.Add("demo", "app.request.throughput_rps", throughputValue, timestamp, nil)
	}

	fmt.Printf("  Generated %d seconds of demo data\n", totalSeconds)

	// Run analyses on all loaded data
	tb.runAnalyses()

	tb.ready = true
	fmt.Printf("Demo scenario loaded: %d series, %d anomalies\n", tb.seriesCount(), len(tb.anomalies))

	return nil
}

// Helper functions for demo data generation (simplified versions of DataGenerator methods)

func getDemoHeapValue(elapsed float64) float64 {
	const baseline, peak = 512.0, 900.0
	return getDemoPhaseValue(elapsed, baseline, peak, -3.0) // heap leads by 3s
}

func getDemoGCPauseValue(elapsed float64) float64 {
	const baseline, peak, spikeLevel = 15.0, 150.0, 80.0
	// Non-correlated spike at 10-12s
	if elapsed >= 10.0 && elapsed < 12.0 {
		mid := 11.0
		if elapsed < mid {
			progress := (elapsed - 10.0) / 1.0
			return baseline + (spikeLevel-baseline)*progress
		}
		progress := (elapsed - mid) / 1.0
		return spikeLevel - (spikeLevel-baseline)*progress
	}
	return getDemoPhaseValue(elapsed, baseline, peak, 0.0)
}

func getDemoLatencyValue(elapsed float64) float64 {
	const baseline, peak, spikeLevel = 45.0, 500.0, 200.0
	// Non-correlated spike at 17-19s
	if elapsed >= 17.0 && elapsed < 19.0 {
		mid := 18.0
		if elapsed < mid {
			progress := (elapsed - 17.0) / 1.0
			return baseline + (spikeLevel-baseline)*progress
		}
		progress := (elapsed - mid) / 1.0
		return spikeLevel - (spikeLevel-baseline)*progress
	}
	return getDemoPhaseValue(elapsed, baseline, peak, 1.0) // latency lags by 1s
}

func getDemoErrorRateValue(elapsed float64) float64 {
	const baseline, peak = 0.1, 8.0
	return getDemoPhaseValue(elapsed, baseline, peak, 2.0) // errors lag by 2s
}

func getDemoCPUValue(elapsed float64) float64 {
	const baseline, peak = 35.0, 75.0
	return getDemoPhaseValue(elapsed, baseline, peak, 1.5) // CPU lags by 1.5s
}

func getDemoThroughputValue(elapsed float64) float64 {
	// Throughput DROPS during incident (inverse of other metrics)
	const baseline, trough = 1000.0, 200.0
	return getDemoPhaseValueInverse(elapsed, baseline, trough, 1.0) // drops with latency
}

func getDemoPhaseValue(elapsed, baseline, peak, delay float64) float64 {
	adjustedBaselineEnd := 25.0 + delay
	adjustedRampEnd := 30.0 + delay
	adjustedPeakEnd := 45.0 + delay
	adjustedRecoveryEnd := 50.0 + delay

	switch {
	case elapsed < adjustedBaselineEnd:
		return baseline
	case elapsed < adjustedRampEnd:
		progress := (elapsed - adjustedBaselineEnd) / (adjustedRampEnd - adjustedBaselineEnd)
		return baseline + (peak-baseline)*progress
	case elapsed < adjustedPeakEnd:
		return peak
	case elapsed < adjustedRecoveryEnd:
		progress := (elapsed - adjustedPeakEnd) / (adjustedRecoveryEnd - adjustedPeakEnd)
		return peak - (peak-baseline)*progress
	default:
		return baseline
	}
}

// getDemoPhaseValueInverse is like getDemoPhaseValue but goes DOWN during incident
func getDemoPhaseValueInverse(elapsed, baseline, trough, delay float64) float64 {
	adjustedBaselineEnd := 25.0 + delay
	adjustedRampEnd := 30.0 + delay
	adjustedTroughEnd := 45.0 + delay
	adjustedRecoveryEnd := 50.0 + delay

	switch {
	case elapsed < adjustedBaselineEnd:
		return baseline
	case elapsed < adjustedRampEnd:
		// Drop from baseline to trough
		progress := (elapsed - adjustedBaselineEnd) / (adjustedRampEnd - adjustedBaselineEnd)
		return baseline - (baseline-trough)*progress
	case elapsed < adjustedTroughEnd:
		return trough
	case elapsed < adjustedRecoveryEnd:
		// Recover from trough to baseline
		progress := (elapsed - adjustedTroughEnd) / (adjustedRecoveryEnd - adjustedTroughEnd)
		return trough + (baseline-trough)*progress
	default:
		return baseline
	}
}
