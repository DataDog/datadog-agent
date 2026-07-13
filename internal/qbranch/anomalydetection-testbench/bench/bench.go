// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bench provides the observer test bench controller. It manages scenarios,
// components, and analysis results by using the public DebugView API of the observer
// component rather than accessing private engine types.
package bench

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	reporterimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/impl"
	testbenchimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/impl-testbench"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// logDataView wraps a recorderdef.LogData and implements observerdef.LogView.
type logDataView struct {
	data *recorderdef.LogData
}

// Ensure logDataView implements observerdef.LogView.
var _ observerdef.LogView = (*logDataView)(nil)

func (v *logDataView) GetContent() string           { return string(v.data.Content) }
func (v *logDataView) GetStatus() string            { return v.data.Status }
func (v *logDataView) GetHostname() string          { return v.data.Hostname }
func (v *logDataView) Tags() []string               { return v.data.Tags }
func (v *logDataView) GetTimestampUnixMilli() int64 { return v.data.TimestampMs }

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

// Config configures the Bench.
type Config struct {
	ScenariosDir string
	HTTPAddr     string
	Cfg          config.Component
	Logger       log.Component

	// ComponentSettings provides per-component configuration and enabled state.
	ComponentSettings observerimpl.ComponentSettings

	// SkipDroppedMetrics filters out metrics marked as dropped during parquet load.
	SkipDroppedMetrics bool

	// LogsOnly skips metric samples and trace stats; only log rows are loaded.
	LogsOnly bool

	// ParquetFormat selects the parquet layout. Empty string = auto-detect.
	ParquetFormat ParquetFormat

	// StreamParquet ingests globally ordered parquet data without retaining raw rows.
	// It is intended for one-shot headless runs, which do not need interactive reruns.
	StreamParquet bool
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
	Name        string         `json:"name"`
	DisplayName string         `json:"displayName"`
	Category    string         `json:"category"` // "detector", "correlator", "processing"
	Enabled     bool           `json:"enabled"`
	Config      map[string]any `json:"config,omitempty"`
}

// testbenchView extends DebugView with debug-only hooks that the live agent never
// calls. Methods prefixed with Debug are implemented by observerImpl but intentionally
// excluded from DebugView to keep the production interface free of testbench concerns.
type testbenchView interface {
	observerimpl.DebugView
	DebugSubscribeBaselineCompleted(func(endSec int64, mutedGroups []string))
}

// BaselineInfo is the baseline analysis window state exposed to the testbench UI.
type BaselineInfo struct {
	Enabled          bool     `json:"enabled"`
	DurationSec      int64    `json:"durationSec"`
	MuteNoisyMetrics bool     `json:"muteNoisyMetrics"`
	Active           bool     `json:"active"`
	WindowEndSec     int64    `json:"windowEndSec,omitempty"`
	MutedSeries      []string `json:"mutedSeries,omitempty"`
}

// StatusResponse is the response for /api/status.
type StatusResponse struct {
	Ready                 bool          `json:"ready"`
	Scenario              string        `json:"scenario,omitempty"`
	SeriesCount           int           `json:"seriesCount"`
	AnomalyCount          int           `json:"anomalyCount"`
	LogAnomalyCount       int           `json:"logAnomalyCount"`
	ComponentCount        int           `json:"componentCount"`
	CorrelatorsProcessing bool          `json:"correlatorsProcessing"`
	ScenarioStart         *int64        `json:"scenarioStart,omitempty"`
	ScenarioEnd           *int64        `json:"scenarioEnd,omitempty"`
	EpisodeInfo           *EpisodeInfo  `json:"episodeInfo,omitempty"`
	ServerConfig          ServerConfig  `json:"serverConfig"`
	Baseline              *BaselineInfo `json:"baseline,omitempty"`
}

// ServerConfig exposes server-side configuration to the UI.
type ServerConfig struct {
	Components map[string]bool `json:"components"`
	LogsOnly   bool            `json:"logsOnly"`
}

// Bench is the main controller for the observer test bench.
// It uses the DebugView API to access observer state without depending on
// private engine types.
type Bench struct {
	config Config

	obs       observerdef.Component
	debug     observerimpl.DebugView
	sseAccess testbenchimpl.SSEAccess // nil in headless mode
	settings  observerimpl.ComponentSettings

	mu             sync.RWMutex
	loadedScenario string
	ready          bool
	episodeInfo    *EpisodeInfo

	rawLogs                []observerdef.LogView
	rawMetrics             []*parquetMetricView
	logAnomalies           []observerdef.Anomaly
	logAnomaliesByDetector map[string][]observerdef.Anomaly

	reportedEvents []ReportedEvent

	// Cached compressed correlations.
	compCorrCache      []observerimpl.CompressedGroup
	compCorrThreshold  float64
	compCorrGeneration uint64
	corrGeneration     uint64

	liveAdvanceTimes []int64

	sseStop chan struct{}

	// API server
	api *BenchAPI

	replayStats *ReplayStats

	streamedLogStartMs int64
	streamedLogEndMs   int64
	hasStreamedLogSpan bool

	// baselineMu protects baseline fields. Separate from tb.mu because the
	// callback fires from the engine run goroutine while tb.mu may already be
	// held by LoadScenario driving ingestion/replay.
	baselineMu           sync.Mutex
	baselineFrozen       bool
	baselineWindowEndSec int64
	baselineMutedSeries  []string
}

// New creates a new Bench instance with the given observer, debug view, SSE access, and config.
// sseAccess may be nil for headless mode.
func New(obs observerdef.Component, debug observerimpl.DebugView, sseAccess testbenchimpl.SSEAccess, cfg Config) (*Bench, error) {
	if _, err := os.Stat(cfg.ScenariosDir); os.IsNotExist(err) {
		if err := os.MkdirAll(cfg.ScenariosDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create scenarios directory: %w", err)
		}
	}

	stop := make(chan struct{})

	tb := &Bench{
		config:                 cfg,
		obs:                    obs,
		debug:                  debug,
		sseAccess:              sseAccess,
		settings:               cfg.ComponentSettings,
		logAnomalies:           []observerdef.Anomaly{},
		logAnomaliesByDetector: make(map[string][]observerdef.Anomaly),
		sseStop:                stop,
	}

	if sseAccess != nil {
		// Heartbeat goroutine for SSE clients.
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					sseAccess.Broadcast(testbenchimpl.SSEEvent{Event: "heartbeat", Data: []byte(`{}`)})
				case <-stop:
					return
				}
			}
		}()
	}

	if cfg.ComponentSettings.Baseline.Enabled {
		if tbv, ok := debug.(testbenchView); ok {
			tbv.DebugSubscribeBaselineCompleted(func(endSec int64, mutedGroups []string) {
				tb.baselineMu.Lock()
				tb.baselineFrozen = true
				tb.baselineWindowEndSec = endSec
				tb.baselineMutedSeries = mutedGroups
				tb.baselineMu.Unlock()
			})
		}
	}

	tb.api = NewBenchAPI(tb)

	// Seed SSE with initial status.
	tb.broadcastStatus()

	return tb, nil
}

// Start starts the test bench HTTP server.
func (tb *Bench) Start() error {
	return tb.api.Start(tb.config.HTTPAddr)
}

// Stop stops the test bench HTTP server and background goroutines.
func (tb *Bench) Stop() error {
	close(tb.sseStop)
	return tb.api.Stop()
}

// broadcastStatus sends the current status to all SSE clients.
func (tb *Bench) broadcastStatus() {
	if tb.sseAccess == nil {
		return
	}
	status := tb.GetStatus()
	data, _ := json.Marshal(status)
	tb.sseAccess.Broadcast(testbenchimpl.SSEEvent{Event: "status", Data: data})
}

// broadcastProgress sends current replay progress to all SSE clients.
func (tb *Bench) broadcastProgress() {
	if tb.sseAccess == nil {
		return
	}
	progress := tb.debug.GetReplayProgress()
	data, _ := json.Marshal(progress)
	tb.sseAccess.Broadcast(testbenchimpl.SSEEvent{Event: "progress", Data: data})
}

// ListScenarios returns all available scenarios.
func (tb *Bench) ListScenarios() ([]ScenarioInfo, error) {
	entries, err := os.ReadDir(tb.config.ScenariosDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read scenarios directory: %w", err)
	}

	scenarios := []ScenarioInfo{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		scenarioPath := filepath.Join(tb.config.ScenariosDir, entry.Name())
		info := ScenarioInfo{
			Name: entry.Name(),
			Path: scenarioPath,
		}

		if _, err := os.Stat(filepath.Join(scenarioPath, "parquet")); err == nil {
			info.HasParquet = true
		}
		if _, err := os.Stat(filepath.Join(scenarioPath, "logs")); err == nil {
			info.HasLogs = true
		}
		if _, err := os.Stat(filepath.Join(scenarioPath, "events")); err == nil {
			info.HasEvents = true
		}

		if !info.HasParquet {
			if files, _ := filepath.Glob(filepath.Join(scenarioPath, "*.parquet")); len(files) > 0 {
				info.HasParquet = true
			}
		}

		scenarios = append(scenarios, info)
	}

	sort.Slice(scenarios, func(i, j int) bool {
		return scenarios[i].Name < scenarios[j].Name
	})

	return scenarios, nil
}

// LoadScenario loads a scenario by name, clearing any previously loaded data.
func (tb *Bench) LoadScenario(name string) error {
	if name == "demo" {
		return tb.loadDemoScenario()
	}

	scenarioPath := filepath.Join(tb.config.ScenariosDir, name)

	if _, err := os.Stat(scenarioPath); os.IsNotExist(err) {
		return fmt.Errorf("scenario not found: %s", name)
	}

	tb.mu.Lock()

	tb.rawLogs = nil
	tb.rawMetrics = nil
	tb.logAnomalies = []observerdef.Anomaly{}
	tb.logAnomaliesByDetector = make(map[string][]observerdef.Anomaly)
	tb.liveAdvanceTimes = nil
	tb.streamedLogStartMs = 0
	tb.streamedLogEndMs = 0
	tb.hasStreamedLogSpan = false
	tb.ready = false
	tb.loadedScenario = name
	tb.debug.SetReplayPhase("loading")

	tb.episodeInfo = nil
	if data, err := os.ReadFile(filepath.Join(scenarioPath, "episode.json")); err == nil {
		var info EpisodeInfo
		if jsonErr := json.Unmarshal(data, &info); jsonErr == nil {
			tb.episodeInfo = &info
		}
	}

	tb.resetAllState()

	tb.mu.Unlock()
	tb.broadcastStatus()
	tb.mu.Lock()

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
		tb.debug.SetReplayPhase("")
		tb.loadedScenario = ""
		close(progressDone)
		tb.mu.Unlock()
		tb.broadcastStatus()
		return err
	}

	scenarioStart := time.Now()
	fmt.Printf("Loading scenario: %s\n", name)

	parquetDir := filepath.Join(scenarioPath, "parquet")
	parquetStart := time.Now()
	if _, err := os.Stat(parquetDir); err == nil {
		if err := tb.loadParquetDir(parquetDir); err != nil {
			return loadFailed(fmt.Errorf("failed to load parquet data: %w", err))
		}
	} else {
		if files, _ := filepath.Glob(filepath.Join(scenarioPath, "*.parquet")); len(files) > 0 {
			if err := tb.loadParquetDir(scenarioPath); err != nil {
				return loadFailed(fmt.Errorf("failed to load parquet data: %w", err))
			}
		}
	}
	fmt.Printf("  Parquet loading took %s\n", time.Since(parquetStart))

	analysisStart := time.Now()
	if tb.config.StreamParquet {
		// Parquet rows were already ingested while reading. Finish the same replay
		// used by the retained-data path without resetting away streamed storage.
		tb.finishReplayLocked()
	} else {
		tb.rerunDetectorsLocked()
	}
	fmt.Printf("  Detector phase took %s\n", time.Since(analysisStart))
	fmt.Printf("  Total scenario load took %s\n", time.Since(scenarioStart))

	sv := tb.debug.StateView()
	rs := tb.replayStats
	fmt.Printf("Scenario loaded: %d metric samples (%d unique series), %d metric anomalies, %d log entries, %d log anomalies\n",
		rs.InputMetricsCount, rs.InputMetricsCardinality, sv.TotalAnomalyCount(), rs.InputLogsCount, len(tb.logAnomalies))

	close(progressDone)
	tb.mu.Unlock()

	tb.broadcastStatus()
	return nil
}

// loadParquetDir loads all parquet files from a directory.
func (tb *Bench) loadParquetDir(dir string) error {
	format := tb.config.ParquetFormat
	if format == FormatAuto {
		format = detectParquetFormat(dir)
	}
	fmt.Printf("  Parquet format: %s\n", format)

	if tb.config.LogsOnly {
		fmt.Printf("  Logs-only mode: skipping parquet metrics and trace stats\n")
	} else if tb.config.StreamParquet {
		if err := tb.streamParquetMetrics(dir, format); err != nil {
			return err
		}
	} else {
		var metrics []recorderdef.MetricData
		var err error
		if format == FormatV2 {
			metrics, err = readAllMetricsV2(dir)
		} else {
			metrics, err = readAllMetrics(dir)
		}
		if err != nil {
			return fmt.Errorf("reading parquet metrics: %w", err)
		}

		fmt.Printf("  Loading %d samples from parquet files\n", len(metrics))

		var droppedCount int
		for _, m := range metrics {
			if strings.HasPrefix(m.Name, "datadog.") {
				continue
			}
			if tb.config.SkipDroppedMetrics && m.Dropped {
				droppedCount++
				continue
			}
			tb.rawMetrics = append(tb.rawMetrics, &parquetMetricView{
				name:      m.Name,
				value:     m.Value,
				tags:      m.Tags,
				timestamp: m.Timestamp,
			})
		}
		if droppedCount > 0 {
			fmt.Printf("  Skipped %d dropped observations from parquet\n", droppedCount)
		}

		tb.feedRawMetrics()
	}

	if tb.config.StreamParquet {
		fmt.Printf("  Streaming globally ordered log rows from parquet files\n")
		count, err := streamOrderedLogs(dir, format, func(entry recorderdef.LogData) error {
			view := logDataView{data: &entry}
			tb.debug.IngestTestbenchLog("parquet", &view)
			tb.debug.AddTelemetry(telemetryTbInputLogsCount, 1, entry.TimestampMs/1000, nil)

			if !tb.hasStreamedLogSpan {
				tb.streamedLogStartMs = entry.TimestampMs
				tb.hasStreamedLogSpan = true
			}
			tb.streamedLogEndMs = entry.TimestampMs
			return nil
		})
		if err != nil {
			return fmt.Errorf("streaming parquet logs: %w", err)
		}
		fmt.Printf("  Streamed %d log rows from parquet files\n", count)
		return nil
	}

	var (
		parquetLogs []recorderdef.LogData
		logsErr     error
	)
	if format == FormatV2 {
		parquetLogs, logsErr = readAllLogsV2(dir)
	} else {
		parquetLogs, logsErr = readAllLogs(dir)
	}
	if logsErr != nil {
		return fmt.Errorf("failed to read parquet logs: %w", logsErr)
	}

	for _, logEntry := range parquetLogs {
		logCopy := logEntry
		tb.rawLogs = append(tb.rawLogs, &logDataView{data: &logCopy})
	}

	return nil
}

func (tb *Bench) streamParquetMetrics(dir string, format ParquetFormat) error {
	fmt.Printf("  Streaming globally ordered metric rows from parquet files\n")

	var (
		haveTimestamp      bool
		currentTimestamp   int64
		currentCount       int64
		currentCardinality map[string]struct{}
		ingestedCount      int
	)
	flushTelemetry := func() {
		if !haveTimestamp {
			return
		}
		tb.debug.AddTelemetry(telemetryTbInputMetricsCount, float64(currentCount), currentTimestamp, nil)
		tb.debug.AddTelemetry(telemetryTbInputMetricsCardinality, float64(len(currentCardinality)), currentTimestamp, nil)
	}

	_, err := streamOrderedMetrics(dir, format, func(metric recorderdef.MetricData) error {
		if strings.HasPrefix(metric.Name, "datadog.") {
			return nil
		}
		if tb.config.SkipDroppedMetrics && metric.Dropped {
			return nil
		}

		if !haveTimestamp || metric.Timestamp != currentTimestamp {
			flushTelemetry()
			currentTimestamp = metric.Timestamp
			currentCount = 0
			currentCardinality = make(map[string]struct{})
			haveTimestamp = true
		}

		view := parquetMetricView{
			name:      metric.Name,
			value:     metric.Value,
			tags:      metric.Tags,
			timestamp: metric.Timestamp,
		}
		tb.debug.IngestMetricSync("parquet", &view)
		currentCount++
		currentCardinality[metric.Name+"|"+strings.Join(metric.Tags, ",")] = struct{}{}
		ingestedCount++
		return nil
	})
	if err != nil {
		return fmt.Errorf("streaming parquet metrics: %w", err)
	}
	flushTelemetry()
	fmt.Printf("  Streamed %d metric rows from parquet files\n", ingestedCount)
	return nil
}

// feedRawMetrics feeds tb.rawMetrics synchronously into the engine and re-adds
// per-timestamp telemetry counters. Uses IngestMetricSync to bypass the
// dispatch channel, so no data is lost regardless of volume. Called from both
// loadParquetDir (initial load) and rerunDetectorsLocked (after engine reset on
// component toggle).
func (tb *Bench) feedRawMetrics() {
	for _, m := range tb.rawMetrics {
		tb.debug.IngestMetricSync("parquet", m)
	}

	// Re-add per-timestamp telemetry. These counters live in TelemetryNamespace
	// which is also cleared by debug.Reset(), so they must be restored on every
	// call to feedRawMetrics (not just the initial load).
	type byTimestampEntry struct {
		Timestamp int64
		Count     int64
	}
	byTimestampCounter := make(map[int64]int64)
	byTimestampCardinality := make(map[int64]map[string]struct{})
	for _, m := range tb.rawMetrics {
		byTimestampCounter[m.timestamp]++
		if _, ok := byTimestampCardinality[m.timestamp]; !ok {
			byTimestampCardinality[m.timestamp] = make(map[string]struct{})
		}
		byTimestampCardinality[m.timestamp][m.name+"|"+strings.Join(m.tags, ",")] = struct{}{}
	}
	countOrdered := make([]byTimestampEntry, 0, len(byTimestampCounter))
	for ts, count := range byTimestampCounter {
		countOrdered = append(countOrdered, byTimestampEntry{ts, count})
	}
	sort.Slice(countOrdered, func(i, j int) bool { return countOrdered[i].Timestamp < countOrdered[j].Timestamp })
	for _, e := range countOrdered {
		tb.debug.AddTelemetry(telemetryTbInputMetricsCount, float64(e.Count), e.Timestamp, nil)
	}
	cardOrdered := make([]byTimestampEntry, 0, len(byTimestampCardinality))
	for ts, set := range byTimestampCardinality {
		cardOrdered = append(cardOrdered, byTimestampEntry{ts, int64(len(set))})
	}
	sort.Slice(cardOrdered, func(i, j int) bool { return cardOrdered[i].Timestamp < cardOrdered[j].Timestamp })
	for _, e := range cardOrdered {
		tb.debug.AddTelemetry(telemetryTbInputMetricsCardinality, float64(e.Count), e.Timestamp, nil)
	}
}

// parquetMetricView wraps a metric record to satisfy observerdef.MetricView.
type parquetMetricView struct {
	name      string
	value     float64
	tags      []string
	timestamp int64
}

func (m *parquetMetricView) GetName() string         { return m.name }
func (m *parquetMetricView) GetValue() float64       { return m.value }
func (m *parquetMetricView) GetRawTags() []string    { return m.tags }
func (m *parquetMetricView) GetTimestampUnix() int64 { return m.timestamp }
func (m *parquetMetricView) GetSampleRate() float64  { return 1.0 }

// unboundedStorageCfg returns a StorageConfig for testbench replay:
// no point-retention window (pre-loaded data stays in memory) and full
// correlation history accumulation enabled (disabled in live mode to avoid
// per-Advance overhead that production reporters never read).
func unboundedStorageCfg() observerimpl.StorageConfig {
	cfg := observerimpl.DefaultStorageConfig()
	cfg.PointRetentionSecs = 0
	cfg.MaxCorrelations = -1           // unlimited — testbench must show all patterns
	cfg.TrackCorrelationHistory = true // accumulate history for replay UI / output
	return cfg
}

// resetAllState resets engine state via DebugView.Reset.
func (tb *Bench) resetAllState() {
	tb.baselineMu.Lock()
	tb.baselineFrozen = false
	tb.baselineWindowEndSec = 0
	tb.baselineMutedSeries = nil
	tb.baselineMu.Unlock()
	tb.debug.Reset(tb.settings, unboundedStorageCfg())
}

// GetStatus returns the current status.
func (tb *Bench) GetStatus() StatusResponse {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	sv := tb.debug.StateView()

	compMap := make(map[string]bool)
	for _, e := range tb.debug.CatalogEntries() {
		compMap[e.Name] = tb.isComponentEnabled(e.Name)
	}

	scenarioStart, scenarioEnd, hasBounds := sv.ScenarioBounds()

	// Extend bounds to include log timestamps
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
	if tb.hasStreamedLogSpan {
		startSec := tb.streamedLogStartMs / 1000
		endSec := (tb.streamedLogEndMs + 999) / 1000
		if !hasBounds {
			scenarioStart = startSec
			scenarioEnd = endSec
			hasBounds = true
		} else {
			if startSec < scenarioStart {
				scenarioStart = startSec
			}
			if endSec > scenarioEnd {
				scenarioEnd = endSec
			}
		}
	}

	var scenarioStartPtr *int64
	var scenarioEndPtr *int64
	if hasBounds {
		scenarioStartPtr = &scenarioStart
		scenarioEndPtr = &scenarioEnd
	}

	componentCount := tb.debug.ExtractorCount() + len(tb.debug.CatalogEntries())

	var baselineInfo *BaselineInfo
	if tb.settings.Baseline.Enabled {
		tb.baselineMu.Lock()
		frozen := tb.baselineFrozen
		windowEndSec := tb.baselineWindowEndSec
		mutedSeries := tb.baselineMutedSeries
		tb.baselineMu.Unlock()
		baselineInfo = &BaselineInfo{
			Enabled:          true,
			DurationSec:      tb.settings.Baseline.DurationSec,
			MuteNoisyMetrics: tb.settings.Baseline.MuteNoisyMetrics,
			Active:           !frozen,
			WindowEndSec:     windowEndSec,
			MutedSeries:      mutedSeries,
		}
	}

	return StatusResponse{
		Ready:                 tb.ready,
		Scenario:              tb.loadedScenario,
		SeriesCount:           sv.TotalSeriesCount(observerdef.TelemetryNamespace),
		AnomalyCount:          sv.TotalAnomalyCount(),
		LogAnomalyCount:       len(tb.logAnomalies),
		ComponentCount:        componentCount,
		CorrelatorsProcessing: false,
		ScenarioStart:         scenarioStartPtr,
		ScenarioEnd:           scenarioEndPtr,
		EpisodeInfo:           tb.episodeInfo,
		ServerConfig: ServerConfig{
			Components: compMap,
			LogsOnly:   tb.config.LogsOnly,
		},
		Baseline: baselineInfo,
	}
}

// isComponentEnabled returns whether the named component is currently enabled.
// Caller must hold tb.mu (at least read lock).
func (tb *Bench) isComponentEnabled(name string) bool {
	if v, ok := tb.settings.Enabled[name]; ok {
		return v
	}
	// Fall back to catalog default.
	for _, e := range tb.debug.CatalogEntries() {
		if e.Name == name {
			return e.DefaultEnabled
		}
	}
	return false
}

// rerunDetectorsLocked re-runs all detectors and correlators on current data.
// Caller must hold lock.
func (tb *Bench) rerunDetectorsLocked() {
	// Reset engine with current settings (clears all storage).
	tb.debug.Reset(tb.settings, unboundedStorageCfg())

	// Re-feed parquet metrics synchronously into the fresh storage.
	tb.feedRawMetrics()

	// Pre-load logs without driving advances so extractor state and log metrics
	// are written to storage. Detectors and correlators are deferred to the
	// subsequent ReplayStoredData call, matching the original single-pass approach.
	for _, logEntry := range tb.rawLogs {
		tb.debug.IngestTestbenchLog("parquet", logEntry)
		ts := logEntry.GetTimestampUnixMilli() / 1000
		tb.debug.AddTelemetry(telemetryTbInputLogsCount, 1, ts, nil)
	}

	tb.finishReplayLocked()
}

// finishReplayLocked runs analysis over data already loaded into observer storage
// and refreshes all derived testbench output. Caller must hold tb.mu.
func (tb *Bench) finishReplayLocked() {
	// Run the full batch replay: reset analysis state (not storage), advance
	// through every stored timestamp so detectors see the full accumulated
	// dataset at each step. This matches what the old testbench achieved via
	// engine.ReplayStoredData() after pre-loading all data into storage.
	tb.debug.ReplayStoredData()

	sv := tb.debug.StateView()

	// Populate log anomalies.
	tb.logAnomalies = []observerdef.Anomaly{}
	tb.logAnomaliesByDetector = make(map[string][]observerdef.Anomaly)
	for _, a := range sv.Anomalies() {
		if a.Type == observerdef.AnomalyTypeLog || reporterimpl.IsLogDerivedAnomaly(a) {
			tb.logAnomalies = append(tb.logAnomalies, a)
			tb.logAnomaliesByDetector[a.DetectorName] = append(tb.logAnomaliesByDetector[a.DetectorName], a)
		}
	}

	tb.corrGeneration++

	// Build reported events from correlation history.
	tb.reportedEvents = buildReportedEvents(sv.CorrelationHistory(), tb.debug.StorageReader())

	// Compute replay stats.
	detectorStats := computeDetectorProcessingStatsFromStateView(sv)
	enrichDetectorStatsKind(detectorStats, tb.debug.CatalogEntries())
	tb.replayStats = &ReplayStats{
		DetectorStats:           detectorStats,
		InputMetricsCount:       sv.TotalSampleCount(observerdef.TelemetryNamespace),
		InputMetricsCardinality: sv.TotalSeriesCount(observerdef.TelemetryNamespace),
		InputLogsCount:          sumStoredTelemetryCounter(sv, telemetryTbInputLogsCount),
		InputAnomaliesCount:     len(sv.Anomalies()),
	}

	tb.ready = true
}

// GetComponents returns all registered components.
func (tb *Bench) GetComponents() []ComponentInfo {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	entries := tb.debug.CatalogEntries()
	components := make([]ComponentInfo, 0, len(entries))
	for _, e := range entries {
		enabled := tb.isComponentEnabled(e.Name)
		category := e.Kind
		if category == "extractor" {
			category = "processing"
		}
		components = append(components, ComponentInfo{
			Name:        e.Name,
			DisplayName: e.DisplayName,
			Category:    category,
			Enabled:     enabled,
		})
	}
	return components
}

// extractorNamespaces returns storage namespace names used by pipeline extractors.
func (tb *Bench) extractorNamespaces() map[string]struct{} {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	out := make(map[string]struct{})
	for _, e := range tb.debug.CatalogEntries() {
		if e.Kind == "extractor" {
			out[e.Name] = struct{}{}
		}
	}
	return out
}

// getStateView returns the current StateView (for API handlers).
func (tb *Bench) getStateView() observerimpl.StateView {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return tb.debug.StateView()
}

// filterMetricAnomalies returns only non-log anomalies from a slice.
func filterMetricAnomalies(anomalies []observerdef.Anomaly) []observerdef.Anomaly {
	result := anomalies[:0:0]
	for _, a := range anomalies {
		if a.Type != observerdef.AnomalyTypeLog {
			result = append(result, a)
		}
	}
	return result
}

// GetMetricsAnomalies returns all metric anomalies detected by TS detectors.
func (tb *Bench) GetMetricsAnomalies() []observerdef.Anomaly {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return filterMetricAnomalies(tb.debug.StateView().Anomalies())
}

// GetMetricsAnomaliesByDetector returns metric anomalies grouped by detector name.
func (tb *Bench) GetMetricsAnomaliesByDetector() map[string][]observerdef.Anomaly {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	byDetector := tb.debug.StateView().AnomaliesByDetector()
	for k, v := range byDetector {
		filtered := filterMetricAnomalies(v)
		if len(filtered) == 0 {
			delete(byDetector, k)
			continue
		}
		byDetector[k] = filtered
	}
	return byDetector
}

// GetMetricsAnomaliesForSource returns metric anomalies for a specific SeriesDescriptor.
func (tb *Bench) GetMetricsAnomaliesForSource(sd observerdef.SeriesDescriptor) []observerdef.Anomaly {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	targetKey := sd.Key()
	all := filterMetricAnomalies(tb.debug.StateView().Anomalies())
	var result []observerdef.Anomaly
	for _, a := range all {
		if a.Source.Key() == targetKey {
			result = append(result, a)
			continue
		}
		if a.Source.Namespace != sd.Namespace && a.Source.Name != "" {
			telemetryName := "telemetry." + a.DetectorName + "." + a.Source.String()
			telemetrySD := observerdef.SeriesDescriptor{
				Namespace: "telemetry",
				Name:      telemetryName,
				Aggregate: observerdef.AggregateAverage,
			}
			if telemetrySD.Key() == targetKey {
				result = append(result, a)
			}
		}
	}
	return result
}

// GetLogAnomalies returns all anomalies emitted directly by log detectors.
func (tb *Bench) GetLogAnomalies() []observerdef.Anomaly {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	result := make([]observerdef.Anomaly, len(tb.logAnomalies))
	copy(result, tb.logAnomalies)
	return result
}

// GetLogAnomaliesByDetector returns log anomalies grouped by detector name.
func (tb *Bench) GetLogAnomaliesByDetector() map[string][]observerdef.Anomaly {
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
// to component registry name.
func (tb *Bench) GetDetectorComponentMap() map[string]string {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	result := make(map[string]string)
	sv := tb.debug.StateView()

	// Use detector list from StateView (these are the runtime names).
	for _, d := range sv.ListDetectors() {
		result[d.Name] = d.Name
	}
	return result
}

// GetReplayStats returns all statistics computed from the last replay run.
func (tb *Bench) GetReplayStats() *ReplayStats {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return tb.replayStats
}

// GetCorrelations returns all correlations detected across the full run.
func (tb *Bench) GetCorrelations() []observerdef.ActiveCorrelation {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return tb.debug.StateView().CorrelationHistory()
}

// GetCompressedCorrelations returns compressed group descriptions for all correlations.
func (tb *Bench) GetCompressedCorrelations(threshold float64) []observerimpl.CompressedGroup {
	tb.mu.RLock()

	if tb.compCorrCache != nil && tb.compCorrThreshold == threshold && tb.compCorrGeneration == tb.corrGeneration {
		cached := tb.compCorrCache
		tb.mu.RUnlock()
		return cached
	}
	tb.mu.RUnlock()

	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tb.compCorrCache != nil && tb.compCorrThreshold == threshold && tb.compCorrGeneration == tb.corrGeneration {
		return tb.compCorrCache
	}

	sv := tb.debug.StateView()
	correlations := sv.CorrelationHistory()

	if len(correlations) == 0 {
		tb.compCorrCache = []observerimpl.CompressedGroup{}
		tb.compCorrThreshold = threshold
		tb.compCorrGeneration = tb.corrGeneration
		return tb.compCorrCache
	}

	var groups []observerimpl.CompressedGroup
	for i, corr := range correlations {
		memberSources := make([]string, 0, len(corr.Anomalies))
		seen := make(map[string]bool)
		for _, a := range corr.Anomalies {
			var src string
			if a.SourceRef != nil {
				src = a.SourceRef.CompactID()
			} else {
				src = a.Source.Key()
			}
			if !seen[src] {
				seen[src] = true
				memberSources = append(memberSources, src)
			}
		}

		groupID := fmt.Sprintf("corr-%d", i)
		cg := observerimpl.CompressedGroup{
			CorrelatorName: corr.Pattern,
			GroupID:        groupID,
			Title:          corr.Title,
			MemberSources:  memberSources,
			SeriesCount:    len(memberSources),
			Precision:      1.0,
			FirstSeen:      corr.FirstSeen,
			LastUpdated:    corr.LastUpdated,
		}
		groups = append(groups, cg)
	}

	tb.compCorrCache = groups
	tb.compCorrThreshold = threshold
	tb.compCorrGeneration = tb.corrGeneration

	return groups
}

// GetCorrelatorStats returns stats from all correlators.
func (tb *Bench) GetCorrelatorStats() map[string]interface{} {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	// In bench mode we don't have direct access to correlator instances.
	return make(map[string]interface{})
}

// IsCorrelatorsProcessing returns false — correlators run synchronously.
func (tb *Bench) IsCorrelatorsProcessing() bool {
	return false
}

// RunHeadless runs a scenario synchronously without the HTTP server and writes output.
func (tb *Bench) RunHeadless(scenario, outputPath string, verbose bool) error {
	if err := tb.LoadScenario(scenario); err != nil {
		return fmt.Errorf("loading scenario %q: %w", scenario, err)
	}

	tb.printHeadlessRunStats()

	if outputPath != "" {
		if err := tb.WriteObserverOutput(outputPath, verbose); err != nil {
			return fmt.Errorf("writing observer output: %w", err)
		}
		fmt.Printf("Observer output written to %s\n", outputPath)
	}

	return nil
}

// printHeadlessRunStats prints per-detector processing-time statistics.
func (tb *Bench) printHeadlessRunStats() {
	rs := tb.GetReplayStats()
	if rs == nil || len(rs.DetectorStats) == 0 {
		return
	}

	names := make([]string, 0, len(rs.DetectorStats))
	for name := range rs.DetectorStats {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Println("\nDetector processing times:")
	for _, name := range names {
		s := rs.DetectorStats[name]
		fmt.Printf("  %-40s  avg=%8.2fµs  median=%8.2fµs  p99=%8.2fµs  total=%s  (%d calls)\n",
			name, s.AvgNs/1e3, s.MedianNs/1e3, s.P99Ns/1e3, formatTotalNs(s.TotalNs), s.Count)
	}

	type perItemRow struct {
		name      string
		kind      string
		nsPerItem float64
		items     int
	}
	perItem := make([]perItemRow, 0, len(rs.DetectorStats))
	for _, name := range names {
		s := rs.DetectorStats[name]
		nItems := replayStatsItemCountForKind(rs, s.Kind)
		if nItems <= 0 {
			continue
		}
		nsPer := s.TotalNs / float64(nItems)
		if nsPer <= 0 {
			continue
		}
		perItem = append(perItem, perItemRow{name: name, kind: s.Kind, nsPerItem: nsPer, items: nItems})
	}
	sort.Slice(perItem, func(i, j int) bool { return perItem[i].nsPerItem > perItem[j].nsPerItem })

	if len(perItem) > 0 {
		fmt.Println("\nProcessing time per item (total ÷ items processed):")
		for _, row := range perItem {
			fmt.Printf("  %-40s  %8s  %-10s (%d items)\n",
				row.name, formatNsAsUsShort(row.nsPerItem), perItemSuffixLabel(row.kind), row.items)
		}
	}
}

func formatTotalNs(ns float64) string {
	us := ns / 1e3
	if us < 1_000 {
		return fmt.Sprintf("%8.1fµs", us)
	}
	ms := us / 1_000
	if ms < 1_000 {
		return fmt.Sprintf("%8.1fms", ms)
	}
	return fmt.Sprintf("%8.3fs ", ms/1_000)
}

func formatNsAsUsShort(ns float64) string {
	us := ns / 1e3
	if us < 10 {
		return fmt.Sprintf("%.2fµs", us)
	}
	if us < 100 {
		return fmt.Sprintf("%.1fµs", us)
	}
	return fmt.Sprintf("%.0fµs", us)
}

func replayStatsItemCountForKind(rs *ReplayStats, kind string) int {
	switch kind {
	case "extractor":
		return rs.InputLogsCount
	case "correlator":
		return rs.InputAnomaliesCount
	default:
		return int(rs.InputMetricsCount)
	}
}

func perItemSuffixLabel(kind string) string {
	switch kind {
	case "extractor":
		return "ns/log"
	case "correlator":
		return "ns/anomaly"
	default:
		return "ns/point"
	}
}

// RunSendAnomalyEvents loads a scenario, then sends one Datadog event per correlation.
func (tb *Bench) RunSendAnomalyEvents(scenario string) error {
	if err := tb.LoadScenario(scenario); err != nil {
		return fmt.Errorf("loading scenario %q: %w", scenario, err)
	}

	tb.mu.RLock()
	events := tb.reportedEvents
	loadedScenario := tb.loadedScenario
	tb.mu.RUnlock()

	extraTags := []string{"scenario:" + loadedScenario}
	if u := os.Getenv("USER"); u != "" {
		extraTags = append(extraTags, "user:"+u)
	} else if h, err := os.Hostname(); err == nil {
		extraTags = append(extraTags, "user:"+h)
	}

	sv := tb.debug.StateView()
	var errs []error
	for _, e := range events {
		if err := sendReportedEventViaAPI(tb.config.Cfg, tb.config.Logger, sv, e, extraTags); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("send-anomaly-event: %d/%d events failed: %w", len(errs), len(events), errors.Join(errs...))
	}
	fmt.Printf("Sent %d anomaly events for scenario %q\n", len(events), loadedScenario)
	return nil
}

// ToggleComponent toggles a component's enabled state and re-runs analyses.
func (tb *Bench) ToggleComponent(name string) error {
	tb.mu.Lock()

	found := false
	for _, e := range tb.debug.CatalogEntries() {
		if e.Name == name {
			found = true
			break
		}
	}
	if !found {
		tb.mu.Unlock()
		return fmt.Errorf("unknown component: %s", name)
	}

	current := tb.isComponentEnabled(name)
	if tb.settings.Enabled == nil {
		tb.settings.Enabled = make(map[string]bool)
	}
	tb.settings.Enabled[name] = !current

	if tb.ready {
		tb.rerunDetectorsLocked()
	}

	tb.mu.Unlock()

	tb.broadcastStatus()
	return nil
}

// loadDemoScenario generates synthetic demo data.
func (tb *Bench) loadDemoScenario() error {
	tb.mu.Lock()

	tb.rawLogs = nil
	tb.rawMetrics = nil
	tb.logAnomalies = []observerdef.Anomaly{}
	tb.logAnomaliesByDetector = make(map[string][]observerdef.Anomaly)
	tb.ready = false
	tb.loadedScenario = "demo"

	tb.resetAllState()

	fmt.Println("Generating demo scenario data...")

	baseTimestamp := int64(1000000)
	const totalSeconds = 70

	if !tb.config.LogsOnly {
		for t := 0; t < totalSeconds; t++ {
			elapsed := float64(t)
			timestamp := baseTimestamp + int64(t)

			observations := []struct {
				name  string
				value float64
				tags  []string
			}{
				{"runtime.heap.used_mb", getDemoHeapValue(elapsed), []string{"host:web-1"}},
				{"runtime.gc.pause_ms", getDemoGCPauseValue(elapsed), []string{"host:web-1"}},
				{"system.cpu.user_percent", getDemoCPUValue(elapsed), []string{"host:web-1"}},
				{"app.request.latency_p99_ms", getDemoLatencyValue(elapsed) * 1.2, []string{"service:api"}},
				{"app.request.latency_p99_ms", getDemoLatencyValue(elapsed) * 0.8, []string{"service:worker"}},
				{"app.request.error_rate", getDemoErrorRateValue(elapsed) * 1.5, []string{"service:api"}},
				{"app.request.error_rate", getDemoErrorRateValue(elapsed) * 0.7, []string{"service:worker"}},
				{"app.request.throughput_rps", getDemoThroughputValue(elapsed) * 1.4, []string{"service:api"}},
				{"app.request.throughput_rps", getDemoThroughputValue(elapsed) * 0.6, []string{"service:worker"}},
				{"network.retransmits", getDemoNetworkRetransmitsValue(elapsed), []string{"host:web-1"}},
				{"ebpf.lock_contention_ns", getDemoLockContentionValue(elapsed), []string{"host:web-1"}},
				{"connection.errors", getDemoConnectionErrorsValue(elapsed), []string{"service:api"}},
				{"connection.errors", getDemoConnectionErrorsValue(elapsed) * 0.6, []string{"service:worker"}},
			}
			for _, obs := range observations {
				tb.rawMetrics = append(tb.rawMetrics, &parquetMetricView{
					name:      obs.name,
					value:     obs.value,
					tags:      obs.tags,
					timestamp: timestamp,
				})
			}
		}
		fmt.Printf("  Generated %d seconds of demo data\n", totalSeconds)
	} else {
		fmt.Printf("  Logs-only mode: skipping demo metric samples\n")
	}

	// Generate demo log entries.
	logMsgIdx := 0
	for t := 0; t < totalSeconds; t++ {
		elapsed := float64(t)
		timestamp := baseTimestamp + int64(t)

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

			serviceTag := "service:service_a"
			if t%2 != 0 {
				serviceTag = "service:service_b"
			}
			tb.rawLogs = append(tb.rawLogs, &logDataView{data: &recorderdef.LogData{
				TimestampMs: timestamp * 1000,
				Status:      "error",
				Content:     []byte(content),
				Tags:        []string{serviceTag},
				Hostname:    "host:web-1",
			}})
		}
	}
	fmt.Printf("  Generated %d demo log entries\n", len(tb.rawLogs))

	tb.rerunDetectorsLocked()
	sv := tb.debug.StateView()
	rs := tb.replayStats
	fmt.Printf("Demo scenario loaded: %d metric samples (%d unique series), %d metric anomalies, %d log entries, %d log anomalies\n",
		rs.InputMetricsCount, rs.InputMetricsCardinality, sv.TotalAnomalyCount(), len(tb.rawLogs), len(tb.logAnomalies))

	tb.mu.Unlock()
	tb.broadcastStatus()
	return nil
}

// LogPatternInfo describes a detected log pattern cluster with its associated metric series.
type LogPatternInfo struct {
	Hash          string   `json:"hash"`
	PatternString string   `json:"patternString"`
	ExampleLog    string   `json:"exampleLog"`
	Count         int      `json:"count"`
	SeriesIDs     []string `json:"seriesIDs"`
}

// GetLogPatterns returns the log patterns detected by the LogPatternExtractor.
// In bench mode we use StateView to find log pattern series.
func (tb *Bench) GetLogPatterns() []LogPatternInfo {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	sv := tb.debug.StateView()
	series := sv.ListSeries(observerdef.SeriesFilter{Namespace: "log_pattern_extractor"})

	type patternKey struct {
		hash string
	}
	patterns := make(map[string]*LogPatternInfo)

	for _, m := range series {
		if !strings.Contains(m.Name, ".count") {
			continue
		}
		// Extract hash from metric name "log.log_pattern_extractor.<hash>.count"
		parts := strings.Split(m.Name, ".")
		if len(parts) < 3 {
			continue
		}
		hash := parts[len(parts)-2]
		info, ok := patterns[hash]
		if !ok {
			info = &LogPatternInfo{
				Hash:      hash,
				SeriesIDs: []string{},
			}
			patterns[hash] = info
		}
		info.SeriesIDs = append(info.SeriesIDs, strconv.Itoa(int(m.Ref))+":count")
	}

	result := make([]LogPatternInfo, 0, len(patterns))
	for _, info := range patterns {
		result = append(result, *info)
	}

	return result
}

// GetRawLogs returns all stored raw log entries.
func (tb *Bench) GetRawLogs() []observerdef.LogView {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return tb.rawLogs
}

// GetReportedEvents returns the events that would have been sent to the Datadog backend.
func (tb *Bench) GetReportedEvents() []ReportedEvent {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return tb.reportedEvents
}

// SendReportedEvent finds the ReportedEvent matching pattern+firstSeen and
// posts it to the Datadog backend.
func (tb *Bench) SendReportedEvent(pattern string, firstSeen int64) error {
	tb.mu.RLock()
	var found *ReportedEvent
	for i := range tb.reportedEvents {
		e := &tb.reportedEvents[i]
		if e.Pattern == pattern && e.FirstSeen == firstSeen {
			found = e
			break
		}
	}
	scenario := tb.loadedScenario
	tb.mu.RUnlock()

	if found == nil {
		return fmt.Errorf("report not found: pattern=%q firstSeen=%d", pattern, firstSeen)
	}

	extraTags := []string{"scenario:" + scenario}
	if u := os.Getenv("USER"); u != "" {
		extraTags = append(extraTags, "user:"+u)
	} else if h, err := os.Hostname(); err == nil {
		extraTags = append(extraTags, "user:"+h)
	}

	return sendReportedEventViaAPI(tb.config.Cfg, tb.config.Logger, tb.debug.StateView(), *found, extraTags)
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

// Helper functions for demo data generation.

func getDemoHeapValue(elapsed float64) float64 {
	const baseline, peak = 512.0, 900.0
	return getDemoPhaseValue(elapsed, baseline, peak, -3.0)
}

func getDemoGCPauseValue(elapsed float64) float64 {
	const baseline, peak, spikeLevel = 15.0, 150.0, 80.0
	if elapsed >= 10.0 && elapsed < 12.0 {
		mid := 11.0
		if elapsed < mid {
			return baseline + (spikeLevel-baseline)*((elapsed-10.0)/1.0)
		}
		return spikeLevel - (spikeLevel-baseline)*((elapsed-mid)/1.0)
	}
	return getDemoPhaseValue(elapsed, baseline, peak, 0.0)
}

func getDemoLatencyValue(elapsed float64) float64 {
	const baseline, peak, spikeLevel = 45.0, 500.0, 200.0
	if elapsed >= 17.0 && elapsed < 19.0 {
		mid := 18.0
		if elapsed < mid {
			return baseline + (spikeLevel-baseline)*((elapsed-17.0)/1.0)
		}
		return spikeLevel - (spikeLevel-baseline)*((elapsed-mid)/1.0)
	}
	return getDemoPhaseValue(elapsed, baseline, peak, 1.0)
}

func getDemoErrorRateValue(elapsed float64) float64 {
	return getDemoPhaseValue(elapsed, 0.1, 8.0, 2.0)
}

func getDemoCPUValue(elapsed float64) float64 {
	return getDemoPhaseValue(elapsed, 35.0, 75.0, 1.5)
}

func getDemoThroughputValue(elapsed float64) float64 {
	return getDemoPhaseValueInverse(elapsed, 1000.0, 200.0, 1.0)
}

func getDemoNetworkRetransmitsValue(elapsed float64) float64 {
	return getDemoPhaseValue(elapsed, 5.0, 90.0, 1.0)
}

func getDemoLockContentionValue(elapsed float64) float64 {
	return getDemoPhaseValue(elapsed, 800.0, 14000.0, 0.5)
}

func getDemoConnectionErrorsValue(elapsed float64) float64 {
	return getDemoPhaseValue(elapsed, 1.0, 30.0, 2.0)
}

func getDemoPhaseValue(elapsed, baseline, peak, delay float64) float64 {
	adj := func(base float64) float64 { return base + delay }
	switch {
	case elapsed < adj(25.0):
		return baseline
	case elapsed < adj(30.0):
		progress := (elapsed - adj(25.0)) / 5.0
		return baseline + (peak-baseline)*progress
	case elapsed < adj(45.0):
		return peak
	case elapsed < adj(50.0):
		progress := (elapsed - adj(45.0)) / 5.0
		return peak - (peak-baseline)*progress
	default:
		return baseline
	}
}

func getDemoPhaseValueInverse(elapsed, baseline, trough, delay float64) float64 {
	adj := func(base float64) float64 { return base + delay }
	switch {
	case elapsed < adj(25.0):
		return baseline
	case elapsed < adj(30.0):
		progress := (elapsed - adj(25.0)) / 5.0
		return baseline - (baseline-trough)*progress
	case elapsed < adj(45.0):
		return trough
	case elapsed < adj(50.0):
		progress := (elapsed - adj(45.0)) / 5.0
		return trough + (baseline-trough)*progress
	default:
		return baseline
	}
}
