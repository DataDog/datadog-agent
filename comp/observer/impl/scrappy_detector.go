// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// Compile-time interface check.
var _ observerdef.Detector = (*ScrappyDetector)(nil)

// ScrappyDetectorConfig holds configuration for the scrappy model detector.
type ScrappyDetectorConfig struct {
	// VocabPath is the path to the frozen vocab.json. Required for
	// tokenization. This file is built once from the synthtel reference
	// catalogue and shared by all episodes and deployments.
	VocabPath string `json:"vocab_path"`

	// ModelPath is the path to the model file (.onnx, .pt, or .bin).
	// When empty, the detector runs in tokenize-only mode (logs token counts
	// as telemetry but does not score). Useful for validating the tokenization
	// pipeline before a trained model is available.
	ModelPath string `json:"model_path"`

	// Threshold is the P([alert]) threshold above which an anomaly is emitted.
	Threshold float64 `json:"threshold"`

	// ContextWindow is the maximum number of tokens kept in the sliding
	// history window. Older tokens are dropped from the front.
	ContextWindow int `json:"context_window"`

	// ScoresOutput is the path to write per-tick scores CSV for evaluation.
	// When empty, no scores file is written. Each line contains:
	// timestamp,p_alert,p_normal,prediction,tick_tokens,series_count,inference_ms,salience,truncated
	ScoresOutput string `json:"scores_output"`

	// TickWindow controls the detection cadence and look-back window in seconds.
	// The detector scores once per TickWindow seconds, reading the past
	// TickWindow seconds of the metric surface for each scored tick. Default 1.
	// Set to 30 to score every 30s over a 30-second window — a full context
	// surface that also yields representative full-context inference latency.
	TickWindow int64 `json:"tick_window"`
}

// DefaultScrappyDetectorConfig returns sensible defaults.
func DefaultScrappyDetectorConfig() ScrappyDetectorConfig {
	return ScrappyDetectorConfig{
		Threshold:     0.5,
		ContextWindow: 4096,
		TickWindow:    1,
	}
}

// readScrappyDetectorConfig reads config from the agent config system.
func readScrappyDetectorConfig(reader ConfigReader, prefix string) any {
	cfg := DefaultScrappyDetectorConfig()
	if key := prefix + "vocab_path"; reader.IsKnown(key) {
		cfg.VocabPath = reader.GetString(key)
	}
	if key := prefix + "model_path"; reader.IsKnown(key) {
		cfg.ModelPath = reader.GetString(key)
	}
	if key := prefix + "threshold"; reader.IsKnown(key) {
		cfg.Threshold = reader.GetFloat64(key)
	}
	if key := prefix + "context_window"; reader.IsKnown(key) {
		cfg.ContextWindow = reader.GetInt(key)
	}
	if key := prefix + "scores_output"; reader.IsKnown(key) {
		cfg.ScoresOutput = reader.GetString(key)
	}
	if key := prefix + "tick_window"; reader.IsKnown(key) {
		cfg.TickWindow = int64(reader.GetInt(key))
	}
	return cfg
}

// scrappyInferenceBackend abstracts model inference so the detector can work
// with different runtimes (ONNX, embedded PyTorch, etc.).
type scrappyInferenceBackend interface {
	// Score takes a token ID sequence and the position of [TICK_END] within it,
	// returning P([alert]) in [0, 1].
	Score(tokenIDs []int, tickEndPos int) (float64, error)

	// Close releases any resources held by the backend.
	Close() error
}

// ScrappyDetector implements observerdef.Detector. It tokenizes the full metric
// surface at each tick using the frozen scrappy vocabulary and runs the model to
// produce an anomaly score. When P([alert]) exceeds the configured threshold,
// it emits an anomaly.
//
// The vocabulary is a flat frozen word list (vocab.json) built from the synthtel
// reference catalogue. Unknown words map to [UNK]. Same file used in training
// and inference — see tokenizer-v1.md and DECISIONS.md P0.
type ScrappyDetector struct {
	config    ScrappyDetectorConfig
	tokenizer *scrappyTokenizer
	backend   scrappyInferenceBackend

	// contextProviders resolves log metric hashes to pattern text.
	contextProviders map[string]observerdef.ContextProvider

	// totalTokens tracks cumulative tokens processed (for telemetry).
	totalTokens int

	// tickCount tracks how many ticks have been processed (for telemetry).
	tickCount int64

	// scoresFile is the optional per-tick scores CSV for evaluation.
	scoresFile *os.File

	// lastScoredDataTime is the data timestamp of the last scored tick, used to
	// gate scoring to once per config.TickWindow seconds.
	lastScoredDataTime int64
}

// NewScrappyDetector creates a detector. If VocabPath is empty (and no
// checkpoint_dir is set), the detector will be inert (Name() works, Detect()
// is a no-op).
func NewScrappyDetector(config ScrappyDetectorConfig) *ScrappyDetector {
	d := &ScrappyDetector{config: config}

	if config.VocabPath == "" {
		return d
	}

	vocab, err := loadScrappyVocab(config.VocabPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scrappy_detector: failed to load vocab from %s: %v\n", config.VocabPath, err)
		return d
	}
	d.tokenizer = newScrappyTokenizer(vocab)
	fmt.Fprintf(os.Stderr, "scrappy_detector: vocab loaded (%d tokens) from %s\n", vocab.size(), config.VocabPath)

	if config.ModelPath != "" {
		backend, err := newInferenceBackend(config.ModelPath, vocab, config.Threshold)
		if err != nil {
			fmt.Fprintf(os.Stderr, "scrappy_detector: failed to load model from %s: %v (running in tokenize-only mode)\n", config.ModelPath, err)
		} else {
			d.backend = backend
		}
	}

	if config.ScoresOutput != "" {
		f, err := os.Create(config.ScoresOutput)
		if err != nil {
			fmt.Fprintf(os.Stderr, "scrappy_detector: failed to create scores file %s: %v\n", config.ScoresOutput, err)
		} else {
			d.scoresFile = f
			fmt.Fprintln(f, "timestamp,p_alert,p_normal,prediction,tick_tokens,series_count,inference_ms,salience,truncated")
		}
	}

	return d
}

// SetContextProviders sets the context providers used to resolve log pattern
// hashes to their pattern text.
func (d *ScrappyDetector) SetContextProviders(providers map[string]observerdef.ContextProvider) {
	d.contextProviders = providers
}

// Name implements observerdef.Detector.
func (d *ScrappyDetector) Name() string { return "scrappy_detector" }

// Detect implements observerdef.Detector. It reads the full metric surface,
// tokenizes it, runs inference, and emits anomalies when P([alert]) exceeds
// the threshold.
func (d *ScrappyDetector) Detect(storage observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	if d.tokenizer == nil {
		return observerdef.DetectionResult{}
	}

	// Gate scoring to once per TickWindow seconds (mirrors ScrappyCollector).
	window := d.config.TickWindow
	if window < 1 {
		window = 1
	}
	if d.lastScoredDataTime > 0 && dataTime-d.lastScoredDataTime < window {
		return observerdef.DetectionResult{}
	}
	d.lastScoredDataTime = dataTime

	// 1. Read the full metric surface (same approach as ScrappyCollector).
	allSeries := storage.ListSeries(observerdef.WorkloadSeriesFilter())
	seriesInputs := d.buildSeriesInputs(storage, allSeries, dataTime)

	// 2. Observe structural signatures for gating.
	for i := range seriesInputs {
		if seriesInputs[i].namespace == "log_metrics_extractor" && seriesInputs[i].pattern != "" {
			d.tokenizer.sigSlots.observe(seriesInputs[i].pattern)
		}
	}

	// 3. Tokenize this tick.
	tickTokens := d.tokenizer.tokenizeTick(seriesInputs)
	d.tickCount++

	// 4. Track context size for telemetry (even without a model).
	d.totalTokens += len(tickTokens)
	d.tickCount++

	// 5. Build telemetry (always — useful even without a model).
	detTag := []string{"detector:scrappy_detector"}
	telemetry := []observerdef.ObserverTelemetry{
		newTelemetryGauge(detTag, telemetryScrappyTickTokens, float64(len(tickTokens)), dataTime),
		newTelemetryGauge(detTag, telemetryScrappyContextTokens, float64(d.totalTokens), dataTime),
		newTelemetryGauge(detTag, telemetryScrappySeriesCount, float64(len(seriesInputs)), dataTime),
	}

	// 6. Run inference if a backend is loaded.
	if d.backend == nil {
		return observerdef.DetectionResult{Telemetry: telemetry}
	}

	// Pass just this tick's tokens. Stateful backends (torch) process
	// incrementally; stateless backends (lookup, ONNX) ignore the context
	// and consume scores in order.
	inferStart := time.Now()
	pAlert, err := d.backend.Score(tickTokens, len(tickTokens)-1)
	inferMs := float64(time.Since(inferStart).Microseconds()) / 1000.0
	if err != nil {
		fmt.Fprintf(os.Stderr, "scrappy_detector: inference error: %v\n", err)
		return observerdef.DetectionResult{Telemetry: telemetry}
	}
	pNormal := 1.0 - pAlert

	// Report both probabilities so replay eval can see the full distribution.
	telemetry = append(telemetry,
		newTelemetryGauge(detTag, telemetryScrappyPAlert, pAlert, dataTime),
		newTelemetryGauge(detTag, telemetryScrappyPNormal, pNormal, dataTime),
	)

	// Per-tick prediction.
	predLabel := "normal"
	prediction := 0.0
	if pAlert >= d.config.Threshold {
		prediction = 1.0
		predLabel = "alert"
	}
	telemetry = append(telemetry,
		newTelemetryGauge(detTag, telemetryScrappyPrediction, prediction, dataTime),
	)

	// Write per-tick scores CSV for evaluation.
	if d.scoresFile != nil {
		salience := d.formatSalience()
		truncated := 0
		if d.config.ContextWindow > 0 && len(tickTokens) > d.config.ContextWindow {
			truncated = 1
			fmt.Fprintf(os.Stderr, "scrappy: tick token count %d exceeds context_window %d — native engine truncates to MAX_TOKENS\n",
				len(tickTokens), d.config.ContextWindow)
		}
		fmt.Fprintf(d.scoresFile, "%d,%.6f,%.6f,%s,%d,%d,%.1f,%s,%d\n",
			dataTime, pAlert, pNormal, predLabel, len(tickTokens), len(seriesInputs), inferMs, salience, truncated)
	}

	// 7. Emit anomaly if the model predicts alert.
	if pAlert >= d.config.Threshold {
		scoreVal := pAlert
		salience := d.parseSalience()
		anomaly := observerdef.Anomaly{
			Source: observerdef.SeriesDescriptor{
				Namespace: "scrappy",
				Name:      "model_alert",
				Tags:      salience.entities,
			},
			DetectorName: "scrappy_detector",
			Title:        fmt.Sprintf("Scrappy model alert: P(alert)=%.3f", pAlert),
			Description:  salience.description(pAlert, pNormal, d.config.Threshold, len(tickTokens), len(seriesInputs)),
			Context:      salience.metricContext(),
			Timestamp:    dataTime,
			Score:        &scoreVal,
		}
		return observerdef.DetectionResult{
			Anomalies: []observerdef.Anomaly{anomaly},
			Telemetry: telemetry,
		}
	}

	return observerdef.DetectionResult{Telemetry: telemetry}
}

// buildSeriesInputs reads the metric surface and converts to tokenizer inputs.
func (d *ScrappyDetector) buildSeriesInputs(
	storage observerdef.StorageReader,
	allSeries []observerdef.SeriesMeta,
	dataTime int64,
) []scrappySeriesInput {
	inputs := make([]scrappySeriesInput, 0, len(allSeries))

	for _, meta := range allSeries {
		window := d.config.TickWindow
		if window < 1 {
			window = 1
		}
		sr := storage.GetSeriesRange(meta.Ref, dataTime-window, dataTime, observerdef.AggregateAverage)
		if sr == nil || len(sr.Points) == 0 {
			continue
		}

		val := sr.Points[len(sr.Points)-1].Value
		if math.IsNaN(val) || math.IsInf(val, 0) {
			continue
		}

		input := scrappySeriesInput{
			namespace: meta.Namespace,
			tags:      meta.Tags,
			value:     val,
		}

		if isLogMetric(meta.Name) {
			input.pattern = d.resolvePattern(meta.Name, meta.Tags)
			if input.pattern == "" {
				input.pattern = meta.Name // fallback
			}
		} else {
			input.name = meta.Name
		}

		inputs = append(inputs, input)
	}

	return inputs
}

// resolvePattern looks up pattern text for a log metric (same logic as ScrappyCollector).
func (d *ScrappyDetector) resolvePattern(metricName string, tags []string) string {
	if len(d.contextProviders) == 0 {
		return ""
	}
	cleanTags := make([]string, 0, len(tags))
	for _, t := range tags {
		if !strings.HasPrefix(t, "observer_source:") {
			cleanTags = append(cleanTags, t)
		}
	}
	contextKey := seriesKey("", metricName, cleanTags)
	for _, provider := range d.contextProviders {
		ctx, ok := provider.GetContextByKey(contextKey)
		if ok {
			return ctx.Pattern
		}
	}
	return ""
}

// signalTokenDescriptions maps structural tokens to human-readable descriptions.
var signalTokenDescriptions = map[string]string{
	"[DELTA_NEW]":          "new series appeared",
	"[DELTA_GONE]":         "series disappeared",
	"[DELTA_UP_EXTREME]":   "extreme spike (>100x)",
	"[DELTA_UP_LARGE]":     "large spike (10-100x)",
	"[DELTA_UP_MED]":       "moderate increase (3-10x)",
	"[DELTA_UP_SMALL]":     "small increase (1.5-3x)",
	"[DELTA_DOWN_LARGE]":   "large drop (>10x)",
	"[DELTA_DOWN_MED]":     "moderate drop (3-10x)",
	"[DELTA_DOWN_SMALL]":   "small drop (1.5-3x)",
	"[DELTA_STABLE]":       "stable",
	"[V_ZERO]":             "value at zero",
	"[V_TINY]":             "very small value",
	"[V_EXTREME]":          "extreme value",
	"[V_MEGA]":             "very large value",
	"[V_GIGA]":             "extremely large value",
	"[TAG_LEVEL_ERROR]":    "error-level logs",
	"[TAG_LEVEL_WARN]":     "warning-level logs",
	"[TAG_LEVEL_CRITICAL]": "critical-level logs",
}

// noiseTokens are structural tokens filtered from salience output.
var noiseTokens = map[string]bool{
	"[PAD]": true, "[UNK]": true, "[TICK_START]": true, "[TICK_END]": true,
	"[BOS]": true, "[EOS]": true, "[SEP]": true,
	"[normal]": true, "[alert]": true,
	"[V_LOW]": true, "[V_NORMAL]": true, "[V_HIGH]": true,
	"[V_VERY_HIGH]": true, "[V_TRUE]": true, "[V_FALSE]": true,
}

// scrappySalience holds parsed and categorized salience from the model.
type scrappySalience struct {
	entities []string // resolved tag key:value pairs (e.g. "service:nginx-proxy")
	signals  []string // human-readable signal descriptions
	patterns []string // resolved log signatures
}

// parseSalience extracts and categorizes salience from the last Score call.
func (d *ScrappyDetector) parseSalience() scrappySalience {
	var s scrappySalience
	seenEntities := make(map[string]bool)

	tb, ok := d.backend.(*torchBackend)
	if !ok || len(tb.LastSalience) == 0 || d.tokenizer == nil {
		return s
	}

	for _, entry := range tb.LastSalience {
		tokenStr := ""
		if word, ok := d.tokenizer.vocab.idToToken[entry.TokenID]; ok {
			tokenStr = word
		}
		if tokenStr == "" || noiseTokens[tokenStr] {
			continue
		}

		// Resolve tag slots → entities
		if resolved := d.tokenizer.tagSlots.resolveSlot(tokenStr); resolved != "" {
			tagKey := tagKeyFromSlotToken(tokenStr)
			if tagKey != "" {
				tag := tagKey + ":" + resolved
				if !seenEntities[tag] {
					s.entities = append(s.entities, tag)
					seenEntities[tag] = true
				}
			}
			continue
		}

		// Resolve sig slots → log patterns
		if resolved := d.tokenizer.sigSlots.resolveSlot(tokenStr); resolved != "" {
			s.patterns = append(s.patterns, resolved)
			continue
		}

		// Known signal tokens → human descriptions
		if desc, ok := signalTokenDescriptions[tokenStr]; ok {
			s.signals = append(s.signals, desc)
			continue
		}

		// Remaining vocab words are content signals (metric names, log words)
		if !strings.HasPrefix(tokenStr, "[") {
			s.signals = append(s.signals, tokenStr)
		}
	}

	return s
}

// tagKeyFromSlotToken extracts the tag key from a slot token name.
// e.g. "[TAG_SERVICE_5]" → "service", "[TAG_HOST_0]" → "host"
func tagKeyFromSlotToken(token string) string {
	for slotKey, prefix := range tagKeyToPrefix {
		if strings.HasPrefix(token, "["+prefix+"_") {
			return slotKey
		}
	}
	return ""
}

// description builds a human-readable anomaly description including salience.
func (s scrappySalience) description(pAlert, pNormal, threshold float64, tickTokens, seriesCount int) string {
	var parts []string
	parts = append(parts, fmt.Sprintf(
		"Scrappy model alert: P(alert)=%.3f, P(normal)=%.3f (threshold=%.3f). "+
			"Tick: %d tokens, %d series.",
		pAlert, pNormal, threshold, tickTokens, seriesCount,
	))

	if len(s.entities) > 0 {
		sorted := make([]string, len(s.entities))
		copy(sorted, s.entities)
		sort.Strings(sorted)
		parts = append(parts, "Entities: "+strings.Join(sorted, ", "))
	}

	if len(s.signals) > 0 {
		parts = append(parts, "Signals: "+strings.Join(s.signals, ", "))
	}

	if len(s.patterns) > 0 {
		truncated := make([]string, len(s.patterns))
		for i, p := range s.patterns {
			if len(p) > 80 {
				truncated[i] = p[:80] + "..."
			} else {
				truncated[i] = p
			}
		}
		parts = append(parts, "Log patterns: "+strings.Join(truncated, "; "))
	}

	return strings.Join(parts, "\n")
}

// metricContext builds a MetricContext for the anomaly.
func (s scrappySalience) metricContext() *observerdef.MetricContext {
	if len(s.patterns) == 0 {
		return nil
	}
	return &observerdef.MetricContext{
		Source:  "scrappy_detector",
		Pattern: s.patterns[0],
	}
}

// formatSalience returns a compact JSON string of salience entries from the
// last Score call, or empty string if none. Token IDs are decoded to vocab
// words, and tag slot tokens are resolved to their original values.
// Format: [{"token":"[TAG_HOST_0]","value":"web-server-3","w":0.12},...]
func (d *ScrappyDetector) formatSalience() string {
	tb, ok := d.backend.(*torchBackend)
	if !ok || len(tb.LastSalience) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tb.LastSalience))
	for _, s := range tb.LastSalience {
		tokenStr := fmt.Sprintf("%d", s.TokenID)
		if d.tokenizer != nil && d.tokenizer.vocab != nil {
			if word, ok := d.tokenizer.vocab.idToToken[s.TokenID]; ok {
				tokenStr = word
			}
		}
		// Resolve slot tokens to their original values.
		resolved := ""
		if d.tokenizer != nil {
			resolved = d.tokenizer.tagSlots.resolveSlot(tokenStr)
			if resolved == "" {
				resolved = d.tokenizer.sigSlots.resolveSlot(tokenStr)
			}
		}
		if resolved != "" {
			parts = append(parts, fmt.Sprintf("{\"token\":%q,\"value\":%q,\"w\":%.4f}", tokenStr, resolved, s.Weight))
		} else {
			parts = append(parts, fmt.Sprintf("{\"token\":%q,\"w\":%.4f}", tokenStr, s.Weight))
		}
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// Close releases resources.
func (d *ScrappyDetector) Close() error {
	if d.scoresFile != nil {
		d.scoresFile.Close()
	}
	if d.backend != nil {
		return d.backend.Close()
	}
	return nil
}
