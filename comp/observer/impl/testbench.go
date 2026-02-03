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

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// TestBenchConfig configures the test bench.
type TestBenchConfig struct {
	ScenariosDir string
	HTTPAddr     string
	Recorder     recorderdef.Component // Optional: for loading parquet scenarios
}

// TestBench is the main controller for the observer test bench.
// It manages scenarios, components, and analysis results.
type TestBench struct {
	config TestBenchConfig

	mu             sync.RWMutex
	storage        *timeSeriesStorage
	loadedScenario string
	ready          bool

	// Components
	logProcessors     []observerdef.LogProcessor
	tsAnalyses        []observerdef.TimeSeriesAnalysis
	anomalyProcessors []observerdef.AnomalyProcessor

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
	Type        string `json:"type"` // "log_processor", "ts_analysis", "anomaly_processor"
	Description string `json:"description,omitempty"`
}

// StatusResponse is the response for /api/status.
type StatusResponse struct {
	Ready          bool   `json:"ready"`
	Scenario       string `json:"scenario,omitempty"`
	SeriesCount    int    `json:"seriesCount"`
	AnomalyCount   int    `json:"anomalyCount"`
	ComponentCount int    `json:"componentCount"`
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

	// Create correlator for anomaly processing
	correlator := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		MinClusterSize:   2,
		WindowSeconds:    120,
	})

	tb := &TestBench{
		config:  config,
		storage: newTimeSeriesStorage(),

		// Register default components
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
		tsAnalyses: []observerdef.TimeSeriesAnalysis{
			NewCUSUMDetector(),
			NewRobustZScoreDetector(),
		},
		anomalyProcessors: []observerdef.AnomalyProcessor{
			correlator,
		},

		anomalies:  []observerdef.AnomalyOutput{},
		byAnalyzer: make(map[string][]observerdef.AnomalyOutput),
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

	// Reset anomaly processors
	for _, proc := range tb.anomalyProcessors {
		proc.Flush() // Clear any accumulated state
	}

	// Load data from scenario
	fmt.Printf("Loading scenario: %s\n", name)

	// Load parquet files
	parquetDir := filepath.Join(scenarioPath, "parquet")
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

	// Load log files
	logsDir := filepath.Join(scenarioPath, "logs")
	if _, err := os.Stat(logsDir); err == nil {
		if err := tb.loadLogsDir(logsDir); err != nil {
			return fmt.Errorf("failed to load logs: %w", err)
		}
	}

	// Load event files
	eventsDir := filepath.Join(scenarioPath, "events")
	if _, err := os.Stat(eventsDir); err == nil {
		if err := tb.loadEventsDir(eventsDir); err != nil {
			return fmt.Errorf("failed to load events: %w", err)
		}
	}

	// Run analyses on all loaded data
	tb.runAnalyses()

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

	totalEvents := 0
	for _, file := range files {
		events, err := LoadEventFile(file)
		if err != nil {
			fmt.Printf("  Warning: failed to load event file %s: %v\n", filepath.Base(file), err)
			continue
		}

		// Events are loaded but routing to processors is handled elsewhere
		totalEvents += len(events)
	}

	if totalEvents > 0 {
		fmt.Printf("  Loaded %d events\n", totalEvents)
	}
	return nil
}

// runAnalyses runs all time series analyses on all stored series.
func (tb *TestBench) runAnalyses() {
	// Get all unique series keys
	type seriesKey struct {
		namespace string
		name      string
		tags      []string
	}

	// Collect all series
	var allSeries []observerdef.Series
	for _, agg := range []Aggregate{AggregateAverage, AggregateCount} {
		// Get series from all namespaces
		for _, ns := range []string{"parquet", "logs", "demo"} {
			series := tb.storage.AllSeries(ns, agg)
			for _, s := range series {
				// Append aggregation suffix to name
				sCopy := s
				sCopy.Name = s.Name + ":" + aggSuffix(agg)
				allSeries = append(allSeries, sCopy)
			}
		}
	}

	fmt.Printf("  Running analyses on %d series\n", len(allSeries))

	// Run analyses
	for _, series := range allSeries {
		for _, analysis := range tb.tsAnalyses {
			result := analysis.Analyze(series)
			for _, anomaly := range result.Anomalies {
				anomaly.AnalyzerName = analysis.Name()
				tb.anomalies = append(tb.anomalies, anomaly)

				// Group by analyzer
				tb.byAnalyzer[anomaly.AnalyzerName] = append(tb.byAnalyzer[anomaly.AnalyzerName], anomaly)

				// Send to anomaly processors
				for _, proc := range tb.anomalyProcessors {
					proc.Process(anomaly)
				}
			}
		}
	}

	// Flush processors to get correlations
	for _, proc := range tb.anomalyProcessors {
		proc.Flush()
	}

	// Collect correlations from processors that support it
	for _, proc := range tb.anomalyProcessors {
		if cs, ok := proc.(interface {
			ActiveCorrelations() []observerdef.ActiveCorrelation
		}); ok {
			tb.correlations = append(tb.correlations, cs.ActiveCorrelations()...)
		}
	}
}

// GetStatus returns the current status.
func (tb *TestBench) GetStatus() StatusResponse {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	return StatusResponse{
		Ready:          tb.ready,
		Scenario:       tb.loadedScenario,
		SeriesCount:    tb.seriesCount(),
		AnomalyCount:   len(tb.anomalies),
		ComponentCount: len(tb.logProcessors) + len(tb.tsAnalyses) + len(tb.anomalyProcessors),
	}
}

// GetComponents returns all registered components.
func (tb *TestBench) GetComponents() []ComponentInfo {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	var components []ComponentInfo

	for _, p := range tb.logProcessors {
		components = append(components, ComponentInfo{
			Name: p.Name(),
			Type: "log_processor",
		})
	}

	for _, a := range tb.tsAnalyses {
		components = append(components, ComponentInfo{
			Name: a.Name(),
			Type: "ts_analysis",
		})
	}

	for _, p := range tb.anomalyProcessors {
		components = append(components, ComponentInfo{
			Name: p.Name(),
			Type: "anomaly_processor",
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

	// Reset anomaly processors
	for _, proc := range tb.anomalyProcessors {
		proc.Flush()
	}

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
