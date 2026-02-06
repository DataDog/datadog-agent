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
	EnableRRCF        bool // Enable RRCF multivariate anomaly detector (default: false)
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
	components   map[string]*registeredComponent
	rrcfAnalysis *RRCFAnalysis // RRCF is managed separately (MultiSeriesAnalysis, not in registry)

	// Results (computed eagerly on scenario load)
	anomalies    []observerdef.AnomalyOutput                          // all anomalies from TS analyses
	correlations []observerdef.ActiveCorrelation                      // from anomaly processors
	byAnalyzer   map[string][]observerdef.AnomalyOutput               // anomalies grouped by analyzer
	bySeriesID   map[observerdef.SeriesID][]observerdef.AnomalyOutput // anomalies grouped by source series id

	// Async correlator processing
	correlatorsProcessing bool  // true while background correlator goroutine is running
	correlatorGen         int64 // generation counter, incremented each rerun

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
	Ready                 bool         `json:"ready"`
	Scenario              string       `json:"scenario,omitempty"`
	SeriesCount           int          `json:"seriesCount"`
	AnomalyCount          int          `json:"anomalyCount"`
	ComponentCount        int          `json:"componentCount"`
	CorrelatorsProcessing bool         `json:"correlatorsProcessing"`
	ScenarioStart         *int64       `json:"scenarioStart,omitempty"`
	ScenarioEnd           *int64       `json:"scenarioEnd,omitempty"`
	ServerConfig          ServerConfig `json:"serverConfig"`
}

// ServerConfig exposes server-side configuration to the UI.
type ServerConfig struct {
	Components     map[string]bool `json:"components"`
	RRCFEnabled    bool            `json:"rrcfEnabled"`
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
		bySeriesID: make(map[observerdef.SeriesID][]observerdef.AnomalyOutput),
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

	// RRCF is a MultiSeriesAnalysis and runs outside the registry
	if config.EnableRRCF {
		rrcfConfig := DefaultRRCFConfig()
		rrcfConfig.Metrics = TestBenchRRCFMetrics()
		tb.rrcfAnalysis = NewRRCFAnalysis(rrcfConfig)
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
	tb.bySeriesID = make(map[observerdef.SeriesID][]observerdef.AnomalyOutput)
	tb.ready = false
	tb.loadedScenario = name

	// Reset ALL correlators (not just enabled) so disabled ones clear stale state
	tb.resetAllProcessors()
	// Reset RRCF state if enabled
	if tb.rrcfAnalysis != nil {
		tb.rrcfAnalysis.Reset()
	}

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

	// Run analyses on all loaded data (analyzers sync, correlators async)
	analysisStart := time.Now()
	tb.rerunAnalysesLocked()
	fmt.Printf("  Analyzer phase took %s\n", time.Since(analysisStart))
	fmt.Printf("  Total scenario load took %s (correlators running in background)\n", time.Since(scenarioStart))
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

// GetStatus returns the current status.
func (tb *TestBench) GetStatus() StatusResponse {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	compMap := make(map[string]bool)
	for name, comp := range tb.components {
		compMap[name] = comp.Enabled
	}

	scenarioStart, scenarioEnd, hasBounds := tb.storage.TimeBounds()
	var scenarioStartPtr *int64
	var scenarioEndPtr *int64
	if hasBounds {
		scenarioStartPtr = &scenarioStart
		scenarioEndPtr = &scenarioEnd
	}

	return StatusResponse{
		Ready:                 tb.ready,
		Scenario:              tb.loadedScenario,
		SeriesCount:           tb.seriesCount(),
		AnomalyCount:          len(tb.anomalies),
		ComponentCount:        len(tb.logProcessors) + len(tb.components),
		CorrelatorsProcessing: tb.correlatorsProcessing,
		ScenarioStart:         scenarioStartPtr,
		ScenarioEnd:           scenarioEndPtr,
		ServerConfig: ServerConfig{
			Components:     compMap,
			RRCFEnabled:    tb.config.EnableRRCF,
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
// Analyzers run synchronously (fast), then correlators run asynchronously in a
// background goroutine so the UI is not blocked by slow correlators like GraphSketch.
func (tb *TestBench) rerunAnalysesLocked() {
	// Clear existing results
	tb.anomalies = tb.anomalies[:0]
	tb.correlations = tb.correlations[:0]
	for k := range tb.byAnalyzer {
		delete(tb.byAnalyzer, k)
	}
	for k := range tb.bySeriesID {
		delete(tb.bySeriesID, k)
	}

	// Reset ALL correlators (not just enabled) so disabled ones clear stale state
	tb.resetAllProcessors()

	analyzers := tb.enabledAnalyzers()
	dedup := tb.getDeduplicator()

	// Phase 1 (sync): Run time series analyzers to collect anomalies
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
						anomaly.Source = observerdef.MetricName(seriesCopy.Name)
						anomaly.SourceSeriesID = observerdef.SeriesID(seriesKey(seriesCopy.Namespace, seriesCopy.Name, seriesCopy.Tags))
						if anomaly.AnalyzerName == "" || anomaly.Source == "" || anomaly.Timestamp == 0 {
							fmt.Printf("  Warning: dropping invalid anomaly (analyzer=%q source=%q ts=%d)\n",
								anomaly.AnalyzerName, anomaly.Source, anomaly.Timestamp)
							continue
						}

						// Apply deduplication if enabled
						if dedup != nil {
							if !dedup.ShouldProcess(string(anomaly.SourceSeriesID), anomaly.Timestamp) {
								continue
							}
						}

						tb.anomalies = append(tb.anomalies, anomaly)
						tb.byAnalyzer[anomaly.AnalyzerName] = append(tb.byAnalyzer[anomaly.AnalyzerName], anomaly)
						if anomaly.SourceSeriesID != "" {
							tb.bySeriesID[anomaly.SourceSeriesID] = append(tb.bySeriesID[anomaly.SourceSeriesID], anomaly)
						}
					}
				}
			}
		}
	}

	// Phase 1b (sync): Run RRCF if enabled (MultiSeriesAnalysis, pull-based)
	if tb.rrcfAnalysis != nil {
		tb.rrcfAnalysis.Reset()
		dataTime := tb.storage.MaxTimestamp()
		rrcfAnomalies := tb.rrcfAnalysis.Analyze(tb.storage, dataTime)
		for _, anomaly := range rrcfAnomalies {
			if dedup != nil {
				ts := anomaly.Timestamp
				if ts == 0 {
					ts = anomaly.TimeRange.End
				}
				if !dedup.ShouldProcess(string(anomaly.SourceSeriesID), ts) {
					continue
				}
			}
			tb.anomalies = append(tb.anomalies, anomaly)
			tb.byAnalyzer[anomaly.AnalyzerName] = append(tb.byAnalyzer[anomaly.AnalyzerName], anomaly)
			if anomaly.SourceSeriesID != "" {
				tb.bySeriesID[anomaly.SourceSeriesID] = append(tb.bySeriesID[anomaly.SourceSeriesID], anomaly)
			}
		}
	}

	// Mark scenario ready now that analyzers are done — anomalies are available
	tb.ready = true

	// Snapshot anomalies sorted by time for the background goroutine.
	// Anomalies are collected per-series, not in time order. Sorting ensures
	// correlators see events chronologically, matching the live observer's
	// behavior and preventing premature eviction of early clusters.
	anomalySnapshot := make([]observerdef.AnomalyOutput, len(tb.anomalies))
	copy(anomalySnapshot, tb.anomalies)
	sort.Slice(anomalySnapshot, func(i, j int) bool {
		return anomalySnapshot[i].Timestamp < anomalySnapshot[j].Timestamp
	})
	correlators := tb.enabledCorrelators()

	// Phase 2 (async): Run correlators in a background goroutine
	tb.correlatorGen++
	gen := tb.correlatorGen
	tb.correlatorsProcessing = true

	go func() {
		// Simulate the live observer's periodic flush+snapshot pattern.
		// We process anomalies in time order, periodically flushing and
		// capturing ActiveCorrelations so we see clusters that would have
		// been observed before eviction — not just those alive at the end.
		allCorrelations := make(map[string]observerdef.ActiveCorrelation) // keyed by Pattern

		snapshotCorrelations := func() {
			for _, proc := range correlators {
				proc.Flush()
			}
			for _, proc := range correlators {
				cs, ok := proc.(interface {
					ActiveCorrelations() []observerdef.ActiveCorrelation
				})
				if !ok {
					continue
				}
				for _, corr := range cs.ActiveCorrelations() {
					// Keep the latest version of each correlation (most anomalies)
					if existing, ok := allCorrelations[corr.Pattern]; ok {
						if len(corr.Anomalies) >= len(existing.Anomalies) {
							allCorrelations[corr.Pattern] = corr
						}
					} else {
						allCorrelations[corr.Pattern] = corr
					}
				}
			}
		}

		var lastFlushTime int64
		const flushInterval int64 = 10 // seconds — matches live observer's ~per-observation cadence

		for _, anomaly := range anomalySnapshot {
			for _, proc := range correlators {
				proc.Process(anomaly)
			}
			// Periodically flush and snapshot, simulating the live observer
			if anomaly.Timestamp-lastFlushTime >= flushInterval {
				snapshotCorrelations()
				lastFlushTime = anomaly.Timestamp
			}
		}
		// Final flush to capture anything remaining
		snapshotCorrelations()

		// Collect results under lock
		tb.mu.Lock()
		defer tb.mu.Unlock()

		if tb.correlatorGen != gen {
			return // Stale — a newer rerun is in progress or completed
		}

		tb.correlations = tb.correlations[:0]
		for _, corr := range allCorrelations {
			tb.correlations = append(tb.correlations, corr)
		}

		tb.correlatorsProcessing = false
	}()
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

// GetAnomaliesForSeries returns anomalies associated with a specific series id.
func (tb *TestBench) GetAnomaliesForSeries(seriesID observerdef.SeriesID) []observerdef.AnomalyOutput {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	anomalies := tb.bySeriesID[seriesID]
	result := make([]observerdef.AnomalyOutput, len(anomalies))
	copy(result, anomalies)
	return result
}

// GetAnalyzerComponentMap returns a mapping from analyzer implementation name
// (e.g. "cusum_detector") to component registry name (e.g. "cusum").
func (tb *TestBench) GetAnalyzerComponentMap() map[string]string {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	result := make(map[string]string)
	for componentName, comp := range tb.components {
		if comp.Registration.Category != "analyzer" {
			continue
		}
		analyzer, ok := comp.Instance.(observerdef.TimeSeriesAnalysis)
		if !ok {
			continue
		}
		result[analyzer.Name()] = componentName
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

// GetCompressedCorrelations returns compressed group descriptions for all active correlations.
func (tb *TestBench) GetCompressedCorrelations(threshold float64) []CompressedGroup {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	if tb.storage == nil || len(tb.correlations) == 0 {
		return []CompressedGroup{}
	}

	// Build universe from storage
	universe := tb.storage.ListAllSeriesCompact()

	// Expand the universe to include aggregated variants (since anomalies use "name:agg" keys)
	var expandedUniverse []seriesCompact
	for _, u := range universe {
		for _, agg := range []string{"avg", "count"} {
			expandedUniverse = append(expandedUniverse, seriesCompact{
				Namespace: u.Namespace,
				Name:      u.Name + ":" + agg,
				Tags:      u.Tags,
			})
		}
	}

	var groups []CompressedGroup
	for i, corr := range tb.correlations {
		// Resolve member series from anomaly SourceSeriesIDs
		memberSet := make(map[string]struct{})
		var members []seriesCompact
		for _, a := range corr.Anomalies {
			if a.SourceSeriesID == "" {
				continue
			}
			sid := string(a.SourceSeriesID)
			if _, seen := memberSet[sid]; seen {
				continue
			}
			memberSet[sid] = struct{}{}

			ns, name, tags, ok := parseSeriesKey(sid)
			if !ok {
				continue
			}
			members = append(members, seriesCompact{
				Namespace: ns,
				Name:      name,
				Tags:      tags,
			})
		}

		groupID := fmt.Sprintf("corr-%d", i)
		cg := CompressGroup(corr.Pattern, groupID, corr.Title, members, expandedUniverse, threshold)
		cg.FirstSeen = corr.FirstSeen
		cg.LastUpdated = corr.LastUpdated
		groups = append(groups, cg)
	}

	return groups
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

// GetRRCFScoreStats returns RRCF score distribution stats if RRCF is enabled.
func (tb *TestBench) GetRRCFScoreStats() RRCFScoreStats {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	if tb.rrcfAnalysis == nil {
		return RRCFScoreStats{Enabled: false}
	}
	return tb.rrcfAnalysis.GetScoreStats()
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

// IsCorrelatorsProcessing returns true if correlators are running in the background.
func (tb *TestBench) IsCorrelatorsProcessing() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.correlatorsProcessing
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
	tb.bySeriesID = make(map[observerdef.SeriesID][]observerdef.AnomalyOutput)
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

	// Run analyses on all loaded data (analyzers sync, correlators async)
	tb.rerunAnalysesLocked()
	fmt.Printf("Demo scenario loaded: %d series, %d anomalies (correlators running in background)\n", tb.seriesCount(), len(tb.anomalies))

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
