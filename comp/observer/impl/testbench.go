// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

type logDataView struct {
	data *recorderdef.LogData
}

// Ensure logDataView implements observer.LogView
var _ observer.LogView = (*logDataView)(nil)

func (v *logDataView) GetContent() []byte {
	return v.data.Content
}

func (v *logDataView) GetStatus() string {
	return v.data.Status
}

func (v *logDataView) GetHostname() string {
	return v.data.Hostname
}

func (v *logDataView) GetTags() []string {
	return v.data.Tags
}

func (v *logDataView) GetTimestampUnixMilli() int64 {
	return v.data.TimestampMs
}

// EpisodePhase represents a time phase within an episode (baseline, disruption, cooldown, warmup).
type EpisodePhase struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// EpisodeScenario describes the scenario context within an episode.
type EpisodeScenario struct {
	AppName         string `json:"app_name"`
	Description     string `json:"description"`
	LongDescription string `json:"long_description"`
}

// EpisodeInfo holds the parsed episode.json metadata for a scenario run.
// The file is optional; if absent, EpisodeInfo will be nil.
type EpisodeInfo struct {
	Episode     string          `json:"episode,omitempty"`
	Cycle       int             `json:"cycle,omitempty"`
	Scenario    EpisodeScenario `json:"scenario,omitempty"`
	Environment string          `json:"environment,omitempty"`
	ExecutionID string          `json:"execution_id,omitempty"`
	Success     bool            `json:"success,omitempty"`
	StartTime   string          `json:"start_time,omitempty"`
	EndTime     string          `json:"end_time,omitempty"`
	Warmup      *EpisodePhase   `json:"warmup,omitempty"`
	Baseline    *EpisodePhase   `json:"baseline,omitempty"`
	Disruption  *EpisodePhase   `json:"disruption,omitempty"`
	Cooldown    *EpisodePhase   `json:"cooldown,omitempty"`
}

// TestBenchConfig configures the test bench.
type TestBenchConfig struct {
	ScenariosDir string
	HTTPAddr     string
	Recorder     recorderdef.Component // Optional: for loading parquet scenarios
	Cfg          config.Component
	Logger       log.Component

	// EnableOverrides controls which components are enabled at startup.
	// Keys are component names (e.g. "cusum", "lead_lag").
	// If a name is present, its value overrides the registry DefaultEnabled.
	// Components not listed use their registry default.
	EnableOverrides map[string]bool
}

// TestBench is the main controller for the observer test bench.
// It manages scenarios, components, and analysis results.
// All orchestration (detection, correlation) is delegated to the engine.
type TestBench struct {
	config TestBenchConfig

	mu             sync.RWMutex
	engine         *engine
	catalog        *componentCatalog
	components     map[string]*componentInstance // from catalog
	loadedScenario string
	ready          bool
	episodeInfo    *EpisodeInfo

	// Logs and log anomalies (testbench-specific, not in engine)
	rawLogs                []observer.LogView
	logAnomalies           []observerdef.Anomaly            // all anomalies from log detectors
	logAnomaliesByDetector map[string][]observerdef.Anomaly // anomalies grouped by detector name

	// Cached compressed correlations (expensive to recompute)
	compCorrCache      []CompressedGroup
	compCorrThreshold  float64
	compCorrGeneration uint64
	corrGeneration     uint64 // bumped after each rerunDetectorsLocked

	// SSE broadcast hub for pushing events to connected browsers.
	sse     *sseHub
	sseStop chan struct{}

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
	Category    string `json:"category"` // "detector", "correlator", "processing"
	Enabled     bool   `json:"enabled"`
}

// StatusResponse is the response for /api/status.
type StatusResponse struct {
	Ready                 bool         `json:"ready"`
	Scenario              string       `json:"scenario,omitempty"`
	SeriesCount           int          `json:"seriesCount"`
	AnomalyCount          int          `json:"anomalyCount"`
	LogAnomalyCount       int          `json:"logAnomalyCount"`
	ComponentCount        int          `json:"componentCount"`
	CorrelatorsProcessing bool         `json:"correlatorsProcessing"`
	ScenarioStart         *int64       `json:"scenarioStart,omitempty"`
	ScenarioEnd           *int64       `json:"scenarioEnd,omitempty"`
	EpisodeInfo           *EpisodeInfo `json:"episodeInfo,omitempty"`
	ServerConfig          ServerConfig `json:"serverConfig"`
}

// ServerConfig exposes server-side configuration to the UI.
type ServerConfig struct {
	Components map[string]bool `json:"components"`
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

	catalog := testbenchCatalog()
	detectors, correlators, components := catalog.Instantiate(config.EnableOverrides)

	extractors := []observerdef.LogMetricsExtractor{
		&LogMetricsExtractor{
			MaxEvalBytes: 4096,
			ExcludeFields: map[string]struct{}{
				"timestamp": {}, "ts": {}, "time": {},
				"pid": {}, "ppid": {}, "uid": {}, "gid": {},
			},
		},
		&ConnectionErrorExtractor{},
	}

	eng := newEngine(engineConfig{
		storage:     newTimeSeriesStorage(),
		extractors:  extractors,
		detectors:   detectors,
		correlators: correlators,
		scheduler:   &currentBehaviorPolicy{},
	})

	hub := newSSEHub()
	stop := make(chan struct{})

	tb := &TestBench{
		config:                 config,
		engine:                 eng,
		catalog:                catalog,
		components:             components,
		logAnomalies:           []observerdef.Anomaly{},
		logAnomaliesByDetector: make(map[string][]observerdef.Anomaly),
		sse:                    hub,
		sseStop:                stop,
	}

	// Heartbeat goroutine — lets SSE clients detect stale connections.
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				hub.broadcast(sseEvent{Event: "heartbeat", Data: []byte(`{}`)})
			case <-stop:
				return
			}
		}
	}()

	tb.api = NewTestBenchAPI(tb)

	// Seed the SSE hub with initial status so the first subscriber gets it.
	tb.broadcastStatus()

	return tb, nil
}

// Start starts the test bench HTTP server.
func (tb *TestBench) Start() error {
	return tb.api.Start(tb.config.HTTPAddr)
}

// Stop stops the test bench HTTP server and background goroutines.
func (tb *TestBench) Stop() error {
	close(tb.sseStop)
	return tb.api.Stop()
}

// broadcastStatus sends the current status to all SSE clients.
// Must be called without holding tb.mu (GetStatus acquires its own read lock).
func (tb *TestBench) broadcastStatus() {
	status := tb.GetStatus()
	data, _ := json.Marshal(status)
	tb.sse.broadcast(sseEvent{Event: "status", Data: data})
}

// broadcastProgress sends current replay progress to all SSE clients.
func (tb *TestBench) broadcastProgress() {
	progress := tb.engine.GetReplayProgress()
	data, _ := json.Marshal(progress)
	tb.sse.broadcast(sseEvent{Event: "progress", Data: data})
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

	// Clear existing data
	tb.engine.storage = newTimeSeriesStorage() // TODO: encapsulate behind engine method
	tb.rawLogs = nil
	tb.logAnomalies = []observerdef.Anomaly{}
	tb.logAnomaliesByDetector = make(map[string][]observerdef.Anomaly)
	tb.ready = false
	tb.loadedScenario = name
	tb.engine.replayPhase.Store("loading")

	// Try to read optional episode.json metadata
	tb.episodeInfo = nil
	if data, err := os.ReadFile(filepath.Join(scenarioPath, "episode.json")); err == nil {
		var info EpisodeInfo
		if jsonErr := json.Unmarshal(data, &info); jsonErr == nil {
			tb.episodeInfo = &info
		}
	}

	// Reset ALL components so disabled ones clear stale state
	tb.resetAllState()

	// Release lock briefly to broadcast "loading" status to all SSE clients,
	// then reacquire for the heavy work.
	tb.mu.Unlock()
	tb.broadcastStatus()
	tb.mu.Lock()

	// Broadcast progress to SSE clients while loading (reads atomic counters, no lock needed).
	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				tb.broadcastProgress()
			case <-progressDone:
				return
			}
		}
	}()

	loadFailed := func(err error) error {
		tb.engine.replayPhase.Store("")
		tb.loadedScenario = "" // roll back so status doesn't look like "loading"
		close(progressDone)
		tb.mu.Unlock()
		tb.broadcastStatus() // notify clients of failure
		return err
	}

	// Load data from scenario
	scenarioStart := time.Now()
	fmt.Printf("Loading scenario: %s\n", name)

	// Load parquet files
	parquetDir := filepath.Join(scenarioPath, "parquet")
	parquetStart := time.Now()
	if _, err := os.Stat(parquetDir); err == nil {
		if err := tb.loadParquetDir(parquetDir); err != nil {
			return loadFailed(fmt.Errorf("failed to load parquet data: %w", err))
		}
	} else {
		// Check for parquet files directly in scenario directory
		if files, _ := filepath.Glob(filepath.Join(scenarioPath, "*.parquet")); len(files) > 0 {
			if err := tb.loadParquetDir(scenarioPath); err != nil {
				return loadFailed(fmt.Errorf("failed to load parquet data: %w", err))
			}
		}
	}
	fmt.Printf("  Parquet loading took %s\n", time.Since(parquetStart))

	// Run analyses on all loaded data (detectors sync, correlators async)
	analysisStart := time.Now()
	tb.rerunDetectorsLocked()
	fmt.Printf("  Detector phase took %s\n", time.Since(analysisStart))
	fmt.Printf("  Total scenario load took %s\n", time.Since(scenarioStart))
	fmt.Printf("Scenario loaded: %d series, %d metric anomalies, %d log entries, %d log anomalies\n", tb.seriesCount(), len(tb.engine.RawAnomalies()), len(tb.rawLogs), len(tb.logAnomalies))

	close(progressDone)
	tb.mu.Unlock()

	// Broadcast final status to SSE clients (outside lock).
	tb.broadcastStatus()

	return nil
}

// loadParquetDir loads all parquet files from a directory using the recorder component.
// Uses batch loading for efficiency - reads all metrics at once instead of streaming.
func (tb *TestBench) loadParquetDir(dir string) error {
	if tb.config.Recorder == nil {
		return fmt.Errorf("recorder component not configured - cannot load parquet files")
	}

	storage := tb.engine.Storage()

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

		storage.Add(
			"parquet", // namespace
			metricName,
			m.Value,
			m.Timestamp,
			m.Tags,
		)
	}

	// Load trace stats and derive trace.* metrics via processStatsView
	traceStats, err := tb.config.Recorder.ReadAllTraceStats(dir)
	if err != nil {
		return fmt.Errorf("reading parquet trace stats: %w", err)
	}

	if len(traceStats) > 0 {
		fmt.Printf("  Loading %d trace stat rows from parquet files\n", len(traceStats))

		sh := &storageHandle{namespace: "parquet", storage: storage}

		// Group by (AgentHostname, AgentEnv) so each view carries the correct agent context
		type agentKey struct{ hostname, env string }
		groups := make(map[agentKey][]recorderdef.TraceStatsData)
		for _, s := range traceStats {
			key := agentKey{s.AgentHostname, s.AgentEnv}
			groups[key] = append(groups[key], s)
		}
		for key, rows := range groups {
			view := &parquetTraceStatsView{agentHostname: key.hostname, agentEnv: key.env, rows: rows}
			processStatsView(sh, view)
		}
	}

	// Load logs from parquet files
	parquetLogs, err := tb.config.Recorder.ReadAllLogs(dir)
	if err != nil {
		return fmt.Errorf("failed to read parquet logs: %w", err)
	}

	for _, log := range parquetLogs {
		tb.rawLogs = append(tb.rawLogs, &logDataView{data: &log})
	}

	return nil
}

// storageHandle implements observerdef.Handle backed by timeSeriesStorage.
// Only ObserveMetric is meaningful; other methods are no-ops.
type storageHandle struct {
	namespace string
	storage   *timeSeriesStorage
}

func (h *storageHandle) ObserveMetric(sample observerdef.MetricView) {
	h.storage.Add(h.namespace, sample.GetName(), sample.GetValue(), sample.GetTimestampUnix(), sample.GetRawTags())
}
func (h *storageHandle) ObserveLog(_ observerdef.LogView)               {}
func (h *storageHandle) ObserveTrace(_ observerdef.TraceView)           {}
func (h *storageHandle) ObserveTraceStats(_ observerdef.TraceStatsView) {}
func (h *storageHandle) ObserveProfile(_ observerdef.ProfileView)       {}

// parquetTraceStatsView adapts a slice of TraceStatsData (sharing the same AgentHostname/AgentEnv)
// to the TraceStatsView interface for use with processStatsView.
type parquetTraceStatsView struct {
	agentHostname string
	agentEnv      string
	rows          []recorderdef.TraceStatsData
}

func (v *parquetTraceStatsView) GetAgentHostname() string { return v.agentHostname }
func (v *parquetTraceStatsView) GetAgentEnv() string      { return v.agentEnv }
func (v *parquetTraceStatsView) GetRows() observerdef.TraceStatsRowIterator {
	return &parquetTraceStatsIterator{rows: v.rows, idx: -1}
}

type parquetTraceStatsIterator struct {
	rows []recorderdef.TraceStatsData
	idx  int
}

func (it *parquetTraceStatsIterator) Next() bool {
	it.idx++
	return it.idx < len(it.rows)
}

func (it *parquetTraceStatsIterator) Row() observerdef.TraceStatRow {
	return &parquetTraceStatRow{data: it.rows[it.idx]}
}

type parquetTraceStatRow struct {
	data recorderdef.TraceStatsData
}

func (r *parquetTraceStatRow) GetClientHostname() string    { return r.data.ClientHostname }
func (r *parquetTraceStatRow) GetClientEnv() string         { return r.data.ClientEnv }
func (r *parquetTraceStatRow) GetClientVersion() string     { return r.data.ClientVersion }
func (r *parquetTraceStatRow) GetClientContainerID() string { return r.data.ClientContainerID }
func (r *parquetTraceStatRow) GetBucketStartUnixNano() uint64 {
	return r.data.BucketStart
}
func (r *parquetTraceStatRow) GetBucketDurationNano() uint64 { return r.data.BucketDuration }
func (r *parquetTraceStatRow) GetService() string            { return r.data.Service }
func (r *parquetTraceStatRow) GetName() string               { return r.data.Name }
func (r *parquetTraceStatRow) GetResource() string           { return r.data.Resource }
func (r *parquetTraceStatRow) GetType() string               { return r.data.Type }
func (r *parquetTraceStatRow) GetHTTPStatusCode() uint32     { return r.data.HTTPStatusCode }
func (r *parquetTraceStatRow) GetSpanKind() string           { return r.data.SpanKind }
func (r *parquetTraceStatRow) GetIsTraceRoot() int32         { return r.data.IsTraceRoot }
func (r *parquetTraceStatRow) GetSynthetics() bool           { return r.data.Synthetics }
func (r *parquetTraceStatRow) GetHits() uint64               { return r.data.Hits }
func (r *parquetTraceStatRow) GetErrors() uint64             { return r.data.Errors }
func (r *parquetTraceStatRow) GetTopLevelHits() uint64       { return r.data.TopLevelHits }
func (r *parquetTraceStatRow) GetDurationNano() uint64       { return r.data.Duration }
func (r *parquetTraceStatRow) GetOkSummary() []byte          { return r.data.OkSummary }
func (r *parquetTraceStatRow) GetErrorSummary() []byte       { return r.data.ErrorSummary }
func (r *parquetTraceStatRow) GetPeerTags() []string         { return r.data.PeerTags }


// resetAllState resets all registered components that support Reset().
func (tb *TestBench) resetAllState() {
	for _, ci := range tb.components {
		if resetter, ok := ci.instance.(interface{ Reset() }); ok {
			resetter.Reset()
		}
	}
}

// GetStatus returns the current status.
func (tb *TestBench) GetStatus() StatusResponse {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	compMap := make(map[string]bool)
	for name, ci := range tb.components {
		compMap[name] = ci.enabled
	}

	scenarioStart, scenarioEnd, hasBounds := tb.engine.Storage().TimeBounds()

	// Extend bounds to include log timestamps (parquet logs can fall outside the metrics range)
	for _, l := range tb.rawLogs {
		ts := l.GetTimestampUnixMilli()
		if ts == 0 {
			continue
		}
		if !hasBounds {
			scenarioStart = ts / 1000
			scenarioEnd = ts / 1000
			hasBounds = true
		} else {
			if ts/1000 < scenarioStart {
				scenarioStart = ts / 1000
			}
			if (ts+999)/1000 > scenarioEnd {
				scenarioEnd = (ts + 999) / 1000
			}
		}
	}

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
		AnomalyCount:          len(tb.engine.RawAnomalies()),
		LogAnomalyCount:       len(tb.logAnomalies),
		ComponentCount:        len(tb.engine.extractors) + len(tb.components),
		CorrelatorsProcessing: false,
		ScenarioStart:         scenarioStartPtr,
		ScenarioEnd:           scenarioEndPtr,
		EpisodeInfo:           tb.episodeInfo,
		ServerConfig: ServerConfig{
			Components: compMap,
		},
	}
}

// rerunDetectorsLocked re-runs all detectors and correlators on current data.
// Caller must hold lock. All orchestration is delegated to the engine.
func (tb *TestBench) rerunDetectorsLocked() {
	// Configure engine with enabled components and reset state BEFORE
	// ingesting logs, so that log observers on the correct detector set
	// receive log data.
	tb.engine.SetDetectors(catalogEnabledDetectors(tb.components, tb.catalog))
	tb.engine.SetCorrelators(catalogEnabledCorrelators(tb.components, tb.catalog))
	tb.engine.resetFull()

	// Reset ALL components (not just enabled) so disabled ones clear stale state
	tb.resetAllState()

	// Feed raw logs through the engine's IngestLog path so that extractors,
	// log observers, and timestamp tracking all use the same code path as
	// live ingestion. We ignore the returned advance requests because
	// ReplayStoredData (below) will handle scheduling after all data is loaded.
	for _, log := range tb.rawLogs {
		obs := &logObs{
			content:     log.GetContent(),
			status:      log.GetStatus(),
			tags:        log.GetTags(),
			hostname:    log.GetHostname(),
			timestampMs: log.GetTimestampUnixMilli(),
		}
		tb.engine.IngestLog("parquet", obs)
	}

	// Replay all stored data through the scheduler policy.
	// The engine's captureRawAnomaly deduplicates anomalies internally,
	// so stateView.Anomalies() returns a clean deduplicated set.
	result := tb.engine.ReplayStoredData()

	// Handle telemetry (write telemetry metrics to storage for UI)
	dataTime := tb.engine.Storage().MaxTimestamp()
	for _, t := range result.telemetry {
		detName := t.DetectorName
		if detName == "" {
			detName = "unknown"
		}
		tb.handleTelemetry([]observerdef.ObserverTelemetry{t}, detName, dataTime)
	}

	// Invalidate compressed correlations cache
	tb.corrGeneration++

	// Mark scenario ready now that all analysis is complete
	tb.ready = true
}

// This will handle custom telemetry created by the anomaly detectors.
// It includes metrics and logs.
func (tb *TestBench) handleTelemetry(telemetry []observerdef.ObserverTelemetry, detectorName string, baseTimestampMs int64) {
	for _, telemetryEvent := range telemetry {
		// Generate missing fields if needed
		if telemetryEvent.Metric != nil {
			metric := &metricObs{
				name:      telemetryEvent.Metric.GetName(),
				value:     telemetryEvent.Metric.GetValue(),
				tags:      telemetryEvent.Metric.GetRawTags(),
				timestamp: telemetryEvent.Metric.GetTimestampUnix(),
			}
			if metric.timestamp == 0 {
				metric.timestamp = baseTimestampMs / 1000
			}
			if telemetryEvent.DetectorName == "" {
				telemetryEvent.DetectorName = detectorName
			}
			// Save this for UI
			tb.engine.Storage().Add("telemetry", "telemetry."+telemetryEvent.DetectorName+"."+metric.name, metric.value, metric.timestamp, metric.tags)
		}

		if telemetryEvent.Log != nil {
			timestamp := telemetryEvent.Log.GetTimestampUnixMilli()
			if timestamp == 0 {
				timestamp = baseTimestampMs
			}
			logTags := telemetryEvent.Log.GetTags()
			tagsCopy := make([]string, len(logTags), len(logTags)+2)
			copy(tagsCopy, logTags)
			tagsCopy = append(tagsCopy, "detector:"+detectorName)
			tagsCopy = append(tagsCopy, "telemetry:true")
			log := recorderdef.LogData{
				Content:     telemetryEvent.Log.GetContent(),
				Status:      telemetryEvent.Log.GetStatus(),
				Tags:        tagsCopy,
				TimestampMs: timestamp,
				Hostname:    telemetryEvent.Log.GetHostname(),
			}
			if log.Status == "" {
				log.Status = "info"
			}
			if telemetryEvent.DetectorName == "" {
				telemetryEvent.DetectorName = detectorName
			}
			// Save this for UI
			tb.rawLogs = append(tb.rawLogs, &logDataView{data: &log})
		}
	}
}

// GetComponents returns all registered components.
func (tb *TestBench) GetComponents() []ComponentInfo {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	var components []ComponentInfo

	// Return components in catalog order
	for _, entry := range tb.catalog.Entries() {
		ci := tb.components[entry.name]
		if ci == nil {
			continue
		}
		category := "detector"
		if entry.kind == componentCorrelator {
			category = "correlator"
		}
		components = append(components, ComponentInfo{
			Name:        entry.name,
			DisplayName: entry.displayName,
			Category:    category,
			Enabled:     ci.enabled,
		})
	}

	return components
}

// GetStorage returns the storage (for API handlers).
func (tb *TestBench) GetStorage() *timeSeriesStorage {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return tb.engine.Storage()
}

// GetMetricsAnomalies returns all metric anomalies detected by TS detectors.
func (tb *TestBench) GetMetricsAnomalies() []observerdef.Anomaly {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	return tb.resolveAnomalySeriesIDs(tb.engine.StateView().Anomalies())
}

// GetMetricsAnomaliesByDetector returns metric anomalies grouped by detector name.
func (tb *TestBench) GetMetricsAnomaliesByDetector() map[string][]observerdef.Anomaly {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	byDetector := tb.engine.StateView().AnomaliesByDetector()
	for k, v := range byDetector {
		byDetector[k] = tb.resolveAnomalySeriesIDs(v)
	}
	return byDetector
}

// GetMetricsAnomaliesForSeries returns metric anomalies associated with a specific series id.
func (tb *TestBench) GetMetricsAnomaliesForSeries(seriesID observerdef.SeriesID) []observerdef.Anomaly {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	// Resolve SourceSeriesIDs first, then filter by the requested series ID.
	// We resolve all anomalies because the engine may store them with empty
	// SourceSeriesID (e.g. RRCF anomalies) that resolve to the requested ID.
	resolved := tb.resolveAnomalySeriesIDs(tb.engine.StateView().Anomalies())
	var result []observerdef.Anomaly
	for _, a := range resolved {
		if a.SourceSeriesID == seriesID {
			result = append(result, a)
		}
	}
	return result
}

// resolveAnomalySeriesIDs applies testbench-specific SourceSeriesID resolution
// to anomalies that have an empty SourceSeriesID. Detectors like RRCF that
// operate on the full storage don't set SourceSeriesID; this maps them to the
// corresponding telemetry series using the naming convention from handleTelemetry.
func (tb *TestBench) resolveAnomalySeriesIDs(anomalies []observerdef.Anomaly) []observerdef.Anomaly {
	for i := range anomalies {
		a := &anomalies[i]
		if a.SourceSeriesID == "" && a.Source != "" {
			telemetryName := "telemetry." + a.DetectorName + "." + string(a.Source)
			a.SourceSeriesID = observerdef.SeriesID(seriesKey("telemetry", telemetryName+":avg", nil))
		}
	}
	return anomalies
}

// GetLogAnomalies returns all anomalies emitted directly by log detectors.
func (tb *TestBench) GetLogAnomalies() []observerdef.Anomaly {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	result := make([]observerdef.Anomaly, len(tb.logAnomalies))
	copy(result, tb.logAnomalies)
	return result
}

// GetLogAnomaliesByDetector returns log anomalies grouped by detector name.
func (tb *TestBench) GetLogAnomaliesByDetector() map[string][]observerdef.Anomaly {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	result := make(map[string][]observerdef.Anomaly)
	for k, v := range tb.logAnomaliesByDetector {
		copied := make([]observerdef.Anomaly, len(v))
		copy(copied, v)
		result[k] = copied
	}
	return result
}

// GetDetectorComponentMap returns a mapping from detector implementation name
// (e.g. "cusum_detector") to component registry name (e.g. "cusum").
func (tb *TestBench) GetDetectorComponentMap() map[string]string {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	result := make(map[string]string)
	for componentName, ci := range tb.components {
		if ci.entry.kind != componentDetector {
			continue
		}
		if detector, ok := ci.instance.(observerdef.Detector); ok {
			result[detector.Name()] = componentName
		} else if detector, ok := ci.instance.(observerdef.SeriesDetector); ok {
			result[detector.Name()] = componentName
		}
	}
	return result
}

// GetCorrelations returns all correlations detected across the full run.
func (tb *TestBench) GetCorrelations() []observerdef.ActiveCorrelation {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	return tb.engine.StateView().CorrelationHistory()
}

// GetCompressedCorrelations returns compressed group descriptions for all correlations.
func (tb *TestBench) GetCompressedCorrelations(threshold float64) []CompressedGroup {
	tb.mu.RLock()

	// Check cache: same threshold and generation means the result is still valid.
	if tb.compCorrCache != nil && tb.compCorrThreshold == threshold && tb.compCorrGeneration == tb.corrGeneration {
		cached := tb.compCorrCache
		tb.mu.RUnlock()
		return cached
	}
	tb.mu.RUnlock()

	// Cache miss — need write lock to compute and store.
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Double-check after acquiring write lock.
	if tb.compCorrCache != nil && tb.compCorrThreshold == threshold && tb.compCorrGeneration == tb.corrGeneration {
		return tb.compCorrCache
	}

	correlations := tb.engine.StateView().CorrelationHistory()
	storage := tb.engine.Storage()
	if storage == nil || len(correlations) == 0 {
		tb.compCorrCache = []CompressedGroup{}
		tb.compCorrThreshold = threshold
		tb.compCorrGeneration = tb.corrGeneration
		return tb.compCorrCache
	}

	// Build universe from storage
	universe := storage.ListAllSeriesCompact()

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
	for i, corr := range correlations {
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

	// Store in cache
	tb.compCorrCache = groups
	tb.compCorrThreshold = threshold
	tb.compCorrGeneration = tb.corrGeneration

	return groups
}

// seriesCount returns the number of unique series (must be called with lock held).
func (tb *TestBench) seriesCount() int {
	storage := tb.engine.Storage()
	if storage == nil {
		return 0
	}
	return len(storage.series)
}

// GetLeadLagEdges returns lead-lag edges if the correlator is enabled.
func (tb *TestBench) GetLeadLagEdges() ([]LeadLagEdge, bool) {
	data, enabled := tb.GetComponentData("lead_lag")
	if edges, ok := data.([]LeadLagEdge); ok {
		return edges, enabled
	}
	return nil, enabled
}

// GetSurpriseEdges returns surprise edges if the correlator is enabled.
func (tb *TestBench) GetSurpriseEdges() ([]SurpriseEdge, bool) {
	data, enabled := tb.GetComponentData("surprise")
	if edges, ok := data.([]SurpriseEdge); ok {
		return edges, enabled
	}
	return nil, enabled
}

// GetCorrelatorStats returns stats from all correlators (enabled or not).
func (tb *TestBench) GetCorrelatorStats() map[string]interface{} {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	stats := make(map[string]interface{})
	for name, ci := range tb.components {
		if ci.entry.kind == componentCorrelator {
			if statter, ok := ci.instance.(interface {
				GetStats() map[string]interface{}
			}); ok {
				stats[name] = statter.GetStats()
			}
		}
	}
	return stats
}

// IsCorrelatorsProcessing returns false — correlators now run synchronously via the engine.
func (tb *TestBench) IsCorrelatorsProcessing() bool {
	return false
}

// RunHeadless runs a scenario synchronously without the HTTP server and writes output.
// If verbose is true, the output file includes full correlation detail (title, members, anomalies).
// If verbose is false, correlations include only the anomalous time span.
func (tb *TestBench) RunHeadless(scenario, outputPath string, verbose bool) error {
	// LoadScenario runs detectors and correlators synchronously via the engine.
	if err := tb.LoadScenario(scenario); err != nil {
		return fmt.Errorf("loading scenario %q: %w", scenario, err)
	}

	// Write structured JSON output.
	if outputPath != "" {
		if err := tb.WriteObserverOutput(outputPath, verbose); err != nil {
			return fmt.Errorf("writing observer output: %w", err)
		}
		fmt.Printf("Observer output written to %s\n", outputPath)
	}

	return nil
}

// RunSendAnomalyEvents loads a scenario, waits for correlators to finish, then
// delegates to notify.go to send one Datadog event per correlation.
// Whether events are actually sent is controlled by observer.event_reporter.sending_enabled in cfg.
func (tb *TestBench) RunSendAnomalyEvents(scenario string) error {
	if err := tb.LoadScenario(scenario); err != nil {
		return fmt.Errorf("loading scenario %q: %w", scenario, err)
	}

	tb.mu.RLock()
	defer tb.mu.RUnlock()
	sender, err := newEventSender(tb.config.Cfg, tb.config.Logger)
	if err != nil {
		return err
	}
	correlations := tb.engine.StateView().ActiveCorrelations()
	sender.sendCorrelationEvents(correlations)
	return nil
}

// ToggleComponent toggles a component's enabled state and re-runs analyses if needed.
func (tb *TestBench) ToggleComponent(name string) error {
	tb.mu.Lock()

	ci, ok := tb.components[name]
	if !ok {
		tb.mu.Unlock()
		return fmt.Errorf("unknown component: %s", name)
	}

	ci.enabled = !ci.enabled

	// Re-run analyses if a scenario is loaded
	if tb.ready && tb.engine.Storage() != nil {
		tb.rerunDetectorsLocked()
	}

	tb.mu.Unlock()

	// Notify SSE clients of the state change (outside lock).
	tb.broadcastStatus()

	return nil
}

// loadDemoScenario generates synthetic demo data directly into storage.
func (tb *TestBench) loadDemoScenario() error {
	tb.mu.Lock()

	// Clear existing data
	tb.engine.storage = newTimeSeriesStorage() // TODO: encapsulate behind engine method
	tb.rawLogs = nil
	tb.logAnomalies = []observerdef.Anomaly{}
	tb.logAnomaliesByDetector = make(map[string][]observerdef.Anomaly)
	tb.ready = false
	tb.loadedScenario = "demo"

	// Reset all state
	tb.resetAllState()

	fmt.Println("Generating demo scenario data...")

	storage := tb.engine.Storage()

	// Generate data for each second of the 70-second scenario
	baseTimestamp := int64(1000000)
	const totalSeconds = 70

	for t := 0; t < totalSeconds; t++ {
		elapsed := float64(t)
		timestamp := baseTimestamp + int64(t)

		// Heap usage (host:web-1)
		heapValue := getDemoHeapValue(elapsed)
		storage.Add("demo", "runtime.heap.used_mb", heapValue, timestamp, []string{"host:web-1"})

		// GC pause time (host:web-1)
		gcValue := getDemoGCPauseValue(elapsed)
		storage.Add("demo", "runtime.gc.pause_ms", gcValue, timestamp, []string{"host:web-1"})

		// CPU usage (host:web-1)
		cpuValue := getDemoCPUValue(elapsed)
		storage.Add("demo", "system.cpu.user_percent", cpuValue, timestamp, []string{"host:web-1"})

		// Request latency — two service variants
		latencyValue := getDemoLatencyValue(elapsed)
		storage.Add("demo", "app.request.latency_p99_ms", latencyValue*1.2, timestamp, []string{"service:api"})
		storage.Add("demo", "app.request.latency_p99_ms", latencyValue*0.8, timestamp, []string{"service:worker"})

		// Error rate — two service variants
		errorValue := getDemoErrorRateValue(elapsed)
		storage.Add("demo", "app.request.error_rate", errorValue*1.5, timestamp, []string{"service:api"})
		storage.Add("demo", "app.request.error_rate", errorValue*0.7, timestamp, []string{"service:worker"})

		// Throughput — two service variants
		throughputValue := getDemoThroughputValue(elapsed)
		storage.Add("demo", "app.request.throughput_rps", throughputValue*1.4, timestamp, []string{"service:api"})
		storage.Add("demo", "app.request.throughput_rps", throughputValue*0.6, timestamp, []string{"service:worker"})

		// Correlator-targeted metrics (trigger kernel_bottleneck / network_degradation patterns)
		// network.retransmits → analyzed as "network.retransmits:avg" — host-level
		retransmits := getDemoNetworkRetransmitsValue(elapsed)
		storage.Add("demo", "network.retransmits", retransmits, timestamp, []string{"host:web-1"})

		// ebpf.lock_contention_ns → analyzed as "ebpf.lock_contention_ns:avg" — host-level
		lockContention := getDemoLockContentionValue(elapsed)
		storage.Add("demo", "ebpf.lock_contention_ns", lockContention, timestamp, []string{"host:web-1"})

		// connection.errors → analyzed as "connection.errors:count" — two service variants
		connErrors := getDemoConnectionErrorsValue(elapsed)
		storage.Add("demo", "connection.errors", connErrors, timestamp, []string{"service:api"})
		storage.Add("demo", "connection.errors", connErrors*0.6, timestamp, []string{"service:worker"})
	}

	fmt.Printf("  Generated %d seconds of demo data\n", totalSeconds)

	// Generate demo log entries following phase-based intervals
	logMsgIdx := 0
	for t := 0; t < totalSeconds; t++ {
		elapsed := float64(t)
		timestamp := baseTimestamp + int64(t)

		// Determine log interval in seconds based on phase
		var logInterval int
		switch {
		case elapsed < 25.0:
			logInterval = 5
		case elapsed < 30.0:
			logInterval = 2
		case elapsed < 45.0:
			logInterval = 1
		case elapsed < 50.0:
			logInterval = 2
		default:
			logInterval = 5
		}

		if logInterval > 0 && t%logInterval == 0 {
			content := errorLogMessages[logMsgIdx%len(errorLogMessages)]
			logMsgIdx++

			// Store raw log entry
			servicetag := ""
			if t%2 == 0 {
				servicetag = "service:service_a"
			} else {
				servicetag = "service:service_b"
			}
			tb.rawLogs = append(tb.rawLogs, &logDataView{data: &recorderdef.LogData{
				TimestampMs: timestamp * 1000,
				Status:      "error",
				Content:     []byte(content),
				Tags:        []string{servicetag},
				Hostname:    "host:web-1",
			}})
		}
	}
	fmt.Printf("  Generated %d demo log entries\n", len(tb.rawLogs))

	// Add hardcoded log anomalies from two detectors (incident peak: t=30-45s)
	score := func(v float64) *float64 { return &v }
	type demoAnomaly struct {
		ts           int64
		detectorName string
		title        string
		description  string
		score        *float64
		service      string
	}
	demoAnomalies := []demoAnomaly{
		// connection_error_extractor: fires during the incident peak
		{baseTimestamp + 30, "connection_error_extractor", "Connection pool exhausted", "connection pool exhausted: max connections reached", score(0.91), "service_a"},
		{baseTimestamp + 35, "connection_error_extractor", "Circuit breaker opened", "circuit breaker open: too many recent failures", score(0.95), "service_a"},
		{baseTimestamp + 40, "connection_error_extractor", "Retry storm detected", "retry limit exceeded after 3 attempts", score(0.88), "service_a"},
		{baseTimestamp + 45, "connection_error_extractor", "Memory pressure rejecting requests", "memory pressure: request rejected", score(0.82), "service_b"},
		// log_metrics_extractor: fires at ramp-up and peak
		{baseTimestamp + 26, "log_metrics_extractor", "Log rate ramp-up detected", "Log emission rate increased 2.5x above baseline (5s → 2s interval)", score(0.74), "service_a"},
		{baseTimestamp + 32, "log_metrics_extractor", "Log rate spike at incident peak", "Log emission rate spiked 10x above baseline (5s → 500ms interval)", score(0.97), "service_a"},
		{baseTimestamp + 38, "log_metrics_extractor", "Sustained high log rate", "Log rate remains elevated: 1 log/s vs baseline 1 log/5s", score(0.85), "service_b"},
	}
	for _, a := range demoAnomalies {
		anomaly := observerdef.Anomaly{
			Type:         observerdef.AnomalyTypeLog,
			Source:       "logs",
			DetectorName: a.detectorName,
			Title:        a.title,
			Description:  a.description,
			Timestamp:    a.ts,
			Score:        a.score,
			Tags:         []string{"service:" + a.service},
		}
		tb.logAnomalies = append(tb.logAnomalies, anomaly)
		tb.logAnomaliesByDetector[a.detectorName] = append(tb.logAnomaliesByDetector[a.detectorName], anomaly)
	}

	// Run analyses on all loaded data (detectors sync, correlators async)
	tb.rerunDetectorsLocked()
	fmt.Printf("Demo scenario loaded: %d series, %d metric anomalies, %d log entries, %d log anomalies\n", tb.seriesCount(), len(tb.engine.RawAnomalies()), len(tb.rawLogs), len(tb.logAnomalies))

	tb.mu.Unlock()
	tb.broadcastStatus()
	return nil
}

// GetRawLogs returns all stored raw log entries.
func (tb *TestBench) GetRawLogs() []observerdef.LogView {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	return tb.rawLogs
}

// errorLogMessages contains realistic error messages for the demo scenario.
var errorLogMessages = []string{
	"request timeout: upstream service did not respond within 30s",
	"connection pool exhausted: max connections reached",
	"circuit breaker open: too many recent failures",
	"retry limit exceeded after 3 attempts",
	"memory pressure: request rejected",
	"GC overhead limit exceeded",
}

// Helper functions for demo data generation

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

// getDemoNetworkRetransmitsValue returns network retransmits (spikes with latency during incident).
func getDemoNetworkRetransmitsValue(elapsed float64) float64 {
	const baseline, peak = 5.0, 90.0
	return getDemoPhaseValue(elapsed, baseline, peak, 1.0) // co-occurs with latency
}

// getDemoLockContentionValue returns eBPF lock contention in ns (rises with heap pressure).
func getDemoLockContentionValue(elapsed float64) float64 {
	const baseline, peak = 800.0, 14000.0
	return getDemoPhaseValue(elapsed, baseline, peak, 0.5) // slightly lags heap
}

// getDemoConnectionErrorsValue returns connection error count (co-occurs with error rate).
func getDemoConnectionErrorsValue(elapsed float64) float64 {
	const baseline, peak = 1.0, 30.0
	return getDemoPhaseValue(elapsed, baseline, peak, 2.0) // co-occurs with error rate
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
