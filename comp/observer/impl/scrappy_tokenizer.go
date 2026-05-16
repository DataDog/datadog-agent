// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"strings"
)

// scrappyTokenizer converts metric surface snapshots into token ID sequences
// for the scrappy anomaly detection model. Token IDs are computed
// algorithmically — no external vocabulary file is needed:
//
//   - Structural tokens (special, value buckets, tag/sig slots) have fixed IDs
//   - Words (metric names, log patterns) are FNV-1a hashed into 4096 buckets
//
// This means the tokenizer works for any service on any host: "system.cpu.user"
// always hashes to the same bucket IDs regardless of what other metrics exist.
//
// Each tick produces: [TICK_START] <series tokens>... [TICK_END]
// The model predicts P([alert]) at the [TICK_END] position.
type scrappyTokenizer struct {
	vocab    *scrappyVocab
	tagSlots *tagSlotMap
	sigSlots *sigSlotMap

	// prevValues tracks the previous tick's value for each series identity.
	// Used for delta tokens and smart zero handling.
	prevValues map[string]float64

	// everNonZero tracks whether a series has ever had a non-zero value.
	// Series that have always been zero are suppressed (truly idle).
	// Series that were non-zero then go to zero emit [V_ZERO] + [DELTA_ZERO].
	everNonZero map[string]bool
}

func newScrappyTokenizer(vocab *scrappyVocab) *scrappyTokenizer {
	return &scrappyTokenizer{
		vocab:       vocab,
		tagSlots:    newTagSlotMap(),
		sigSlots:    newSigSlotMap(50, 150),
		prevValues:  make(map[string]float64),
		everNonZero: make(map[string]bool),
	}
}

// tokenizeTick converts one metric surface snapshot into a token ID sequence.
// series is the list of (namespace, name, pattern, tags, value) tuples from
// the observer's StorageReader.
func (t *scrappyTokenizer) tokenizeTick(series []scrappySeriesInput) []int {
	tokens := []int{t.vocab.encode(tokTickStart)}

	seen := make(map[string]struct{}, len(series))
	currentValues := make(map[string]float64, len(series))

	for i := range series {
		s := &series[i]

		// Dedup by identity (name or pattern)
		identity := s.name
		if identity == "" {
			identity = s.pattern
		}
		if identity != "" {
			if _, dup := seen[identity]; dup {
				continue
			}
			seen[identity] = struct{}{}
		}

		// Smart zero handling:
		// - Series that have NEVER been non-zero → suppress (truly idle)
		// - Series that WERE non-zero and went to zero → emit with [V_ZERO] + delta
		if s.value == 0 {
			if identity != "" && !t.everNonZero[identity] {
				continue // truly idle, suppress
			}
			// Was non-zero before, now zero — this is signal (recovery/drop)
		}

		// Track non-zero state
		if s.value != 0 && identity != "" {
			t.everNonZero[identity] = true
		}

		seriesTokens := t.tokenizeSeries(s)
		if len(seriesTokens) > 0 {
			// Append delta token based on change from previous tick
			if identity != "" {
				delta := t.deltaToken(identity, s.value)
				seriesTokens = append(seriesTokens, t.vocab.encode(delta))
				currentValues[identity] = s.value
			}
			tokens = append(tokens, seriesTokens...)
		}
	}

	// Update previous values for next tick
	t.prevValues = currentValues

	tokens = append(tokens, t.vocab.encode(tokTickEnd))
	return tokens
}

// deltaToken computes the logarithmic change magnitude token for a series
// relative to its value in the previous tick. Bands are symmetric around 1.0,
// each covering roughly half an order of magnitude.
func (t *scrappyTokenizer) deltaToken(identity string, current float64) string {
	prev, hasPrev := t.prevValues[identity]

	if !hasPrev {
		return tokDeltaNew
	}

	if current == 0 && prev != 0 {
		return tokDeltaGone
	}
	if prev == 0 {
		if current == 0 {
			return tokDeltaStable
		}
		return tokDeltaNew
	}

	return deltaFromRatio(current / prev)
}

// deltaFromRatio maps a ratio (current/previous) to a delta token.
// Asymmetric: 4 up-buckets vs 3 down-buckets. Spikes are the primary
// anomaly signal so they get more resolution.
func deltaFromRatio(ratio float64) string {
	switch {
	case ratio > 100:
		return tokDeltaUpExtreme
	case ratio > 10:
		return tokDeltaUpLarge
	case ratio > 3:
		return tokDeltaUpMed
	case ratio > 1.5:
		return tokDeltaUpSmall
	case ratio >= 0.67:
		return tokDeltaStable
	case ratio >= 0.33:
		return tokDeltaDownSmall
	case ratio >= 0.1:
		return tokDeltaDownMed
	default:
		return tokDeltaDownLarge
	}
}

// scrappySeriesInput is the tokenizer's view of one series in a tick.
type scrappySeriesInput struct {
	namespace string
	name      string   // non-empty for named metrics
	pattern   string   // non-empty for log metrics
	tags      []string
	value     float64
}

// tokenizeSeries tokenizes a single series within a tick.
func (t *scrappyTokenizer) tokenizeSeries(s *scrappySeriesInput) []int {
	var tokens []int

	if s.name != "" {
		// Named metric: split on . and _
		tokens = append(tokens, tokenizeMetricName(s.name, t.vocab)...)
	} else if s.pattern != "" {
		switch s.namespace {
		case "log_pattern_extractor":
			// Drain pattern: prefix + tokenize words
			tokens = append(tokens, t.vocab.encode(tokLogPattern))
			tokens = append(tokens, tokenizeDrainPattern(s.pattern, t.vocab)...)
		case "log_metrics_extractor":
			// Structural C/D signature: gated slot token
			slot := t.sigSlots.getSlotToken(s.pattern)
			if slot == "" {
				return nil // below threshold or over budget
			}
			tokens = append(tokens, t.vocab.encode(slot))
		default:
			return nil
		}
	}

	if len(tokens) == 0 {
		return nil
	}

	// Tags
	tokens = append(tokens, tokenizeTags(s.tags, t.tagSlots, t.vocab)...)

	// Value bucket
	bucket := valueBucket(s.value, s.name)
	tokens = append(tokens, t.vocab.encode(bucket))

	return tokens
}

// --- Metric name tokenization ---

// tokenizeMetricName splits a metric name on . and _, encoding each word.
// Single-char fragments are kept — they carry unit and direction semantics
// (e.g. "s" = per-second, "w"/"r" = write/read, "q" = queue).
func tokenizeMetricName(name string, vocab *scrappyVocab) []int {
	var tokens []int
	for _, word := range splitMetricName(name) {
		w := strings.ToLower(word)
		if len(w) >= 1 {
			tokens = append(tokens, vocab.encode(w))
		}
	}
	return tokens
}

// splitMetricName splits on . and _ (matching Python re.split(r"[._]", name)).
func splitMetricName(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool {
		return r == '.' || r == '_'
	})
}

// --- Drain pattern tokenization ---

// stopwords matches the Python STOPWORDS set.
var stopwords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "to": {}, "of": {}, "for": {}, "in": {},
	"on": {}, "at": {}, "by": {}, "with": {}, "is": {}, "was": {}, "are": {},
	"were": {}, "be": {}, "been": {}, "being": {}, "and": {}, "or": {},
	"but": {}, "if": {}, "then": {}, "than": {}, "that": {}, "this": {},
	"it": {}, "its": {},
}

// tokenizeDrainPattern tokenizes a Drain log pattern using the shared vocabulary.
func tokenizeDrainPattern(pattern string, vocab *scrappyVocab) []int {
	// Extract msg value if JSON-wrapped
	text := pattern
	if strings.HasPrefix(text, `{"msg":`) {
		// Simple extraction without full JSON parse — find the value string
		if idx := strings.Index(text, `"msg":"`); idx >= 0 {
			rest := text[idx+7:]
			if end := strings.Index(rest, `"`); end >= 0 {
				text = rest[:end]
			}
		}
	}

	var tokens []int
	for _, word := range splitLogPattern(text) {
		w := strings.ToLower(strings.Trim(word, "."))
		if w == "" {
			continue
		}
		if w == "*" || w == "***" {
			tokens = append(tokens, vocab.encode(tokWild))
		} else if strings.HasPrefix(w, "/") || strings.HasPrefix(w, "http") {
			tokens = append(tokens, vocab.encode(tokPath))
		} else if _, stop := stopwords[w]; stop {
			continue
		} else if len(w) >= 2 {
			tokens = append(tokens, vocab.encode(w))
		}
	}
	return tokens
}

// splitLogPattern splits on whitespace and common punctuation
// (matching Python re.split(r"[\s:;,()=\[\]{}<>\"']+", text)).
func splitLogPattern(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\r', ':', ';', ',', '(', ')', '=',
			'[', ']', '{', '}', '<', '>', '"', '\'':
			return true
		}
		return false
	})
}

// --- Tag slot mapping ---

// tagSlotMap maps high-cardinality tag values to opaque slot token IDs.
//
// Slots are generic entity handles — TAG_SERVICE_0 doesn't mean "webapp",
// it means "the first service I encountered this session." The model learns
// relational structure between entities, not slot semantics. The Mamba SSM
// tracks per-entity state across ticks via these stable handles.
//
// In production, slots assign sequentially as values appear. During training,
// the Python tokenizer randomizes assignment per episode.
type tagSlotMap struct {
	slots   map[string]map[string]string // key → value → slot token
	budgets map[string]int
}

// Production-realistic tag budgets. Sized for DaemonSet agents on busy k8s
// nodes where the agent sees metrics from all pods on the node.
var defaultTagBudgets = map[string]int{
	"device":          32,  // NVMe arrays, network interfaces, mounts
	"device_name":     32,  // same slots as device
	"core":            128, // 96+ core servers are common
	"service":         128, // DaemonSet sees all pods on node
	"host":            128, // pod hostnames on a k8s node
	"source":          16,  // multiple log pipelines
	"kube_namespace":  64,  // k8s namespace grouping
	"kube_deployment": 128, // k8s deployment identity
	"container_name":  128, // container identity
}

var tagKeyToPrefix = map[string]string{
	"device":          "TAG_DEVICE",
	"core":            "TAG_CORE",
	"service":         "TAG_SERVICE",
	"host":            "TAG_HOST",
	"source":          "TAG_SOURCE",
	"kube_namespace":  "TAG_KUBE_NS",
	"kube_deployment": "TAG_KUBE_DEPLOY",
	"container_name":  "TAG_CONTAINER",
}

func newTagSlotMap() *tagSlotMap {
	budgets := make(map[string]int, len(defaultTagBudgets))
	for k, v := range defaultTagBudgets {
		budgets[k] = v
	}
	return &tagSlotMap{
		slots:   make(map[string]map[string]string),
		budgets: budgets,
	}
}

func (m *tagSlotMap) getSlotToken(key, value string) string {
	// Low-cardinality literal tokens
	if key == "level" {
		switch strings.ToLower(value) {
		case "info":
			return "[TAG_LEVEL_INFO]"
		case "warn", "warning":
			return "[TAG_LEVEL_WARN]"
		case "error":
			return "[TAG_LEVEL_ERROR]"
		case "debug":
			return "[TAG_LEVEL_DEBUG]"
		case "critical":
			return "[TAG_LEVEL_CRITICAL]"
		}
		return ""
	}

	// device_name uses same slots as device
	slotKey := key
	if key == "device_name" {
		slotKey = "device"
	}
	budget, ok := m.budgets[slotKey]
	if !ok {
		return ""
	}

	slotMap, ok := m.slots[slotKey]
	if !ok {
		slotMap = make(map[string]string)
		m.slots[slotKey] = slotMap
	}

	if tok, ok := slotMap[value]; ok {
		return tok
	}
	idx := len(slotMap)
	if idx >= budget {
		return "" // budget exhausted
	}
	prefix, ok := tagKeyToPrefix[slotKey]
	if !ok {
		return ""
	}
	tok := fmt.Sprintf("[%s_%d]", prefix, idx)
	slotMap[value] = tok
	return tok
}

// --- Structural signature slot mapping ---

type sigSlotMap struct {
	counts    map[string]int
	slots     map[string]string
	nextSlot  int
	threshold int
	budget    int
}

func newSigSlotMap(threshold, budget int) *sigSlotMap {
	return &sigSlotMap{
		counts:    make(map[string]int),
		slots:     make(map[string]string),
		threshold: threshold,
		budget:    budget,
	}
}

// observe counts a signature occurrence (call during scan pass or on each tick).
func (m *sigSlotMap) observe(signature string) {
	m.counts[signature]++
}

// getSlotToken returns the slot token for a signature, or "" if below threshold or over budget.
func (m *sigSlotMap) getSlotToken(signature string) string {
	count := m.counts[signature]
	if count < m.threshold {
		return ""
	}
	if tok, ok := m.slots[signature]; ok {
		return tok
	}
	if m.nextSlot >= m.budget {
		return ""
	}
	tok := fmt.Sprintf("[SIG_%d]", m.nextSlot)
	m.slots[signature] = tok
	m.nextSlot++
	return tok
}

// --- Tag tokenization ---

func tokenizeTags(tags []string, tagSlots *tagSlotMap, vocab *scrappyVocab) []int {
	var tokens []int
	for _, tag := range tags {
		idx := strings.Index(tag, ":")
		if idx < 0 {
			continue
		}
		key := tag[:idx]
		value := tag[idx+1:]
		slot := tagSlots.getSlotToken(key, value)
		if slot != "" && vocab.contains(slot) {
			tokens = append(tokens, vocab.encode(slot))
		}
	}
	return tokens
}

// --- Value bucketing ---

// valueBucket maps a metric value to a bucket token string.
func valueBucket(val float64, name string) string {
	if val == 0 {
		return tokVZero
	}

	// Percentage metrics — guard against nanosecond counters that happen
	// to match .cpu. patterns (e.g. container.cpu.usage is nanoseconds,
	// not a percentage). Values > 200 are clearly not percentages.
	if name != "" && isPercentageMetric(name) && val <= 200 {
		return pctBucket(val)
	}

	// Latency metrics
	if name != "" && isLatencyMetric(name) {
		return latencyBucket(val)
	}

	// Boolean metrics
	if val == 1.0 && name != "" && isBooleanMetric(name) {
		return tokVTrue
	}

	// Universal magnitude buckets
	abs := val
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs < 0.01:
		return tokVTiny
	case abs < 1:
		return tokVLow
	case abs < 100:
		return tokVNormal
	case abs < 10000:
		return tokVHigh
	case abs < 1000000:
		return tokVVeryHigh
	case abs < 100000000:
		return tokVMega // 10^6 to 10^8 (large counters, MB-range byte counts)
	case abs < 10000000000:
		return tokVGiga // 10^8 to 10^10 (memory bytes, nanosecond CPU counters)
	default:
		return tokVTera // > 10^10 (cumulative nanoseconds, no-limit sentinels)
	}
}

func isPercentageMetric(name string) bool {
	if strings.HasSuffix(name, ".pct_usable") || strings.HasSuffix(name, ".pct_free") || strings.HasSuffix(name, ".in_use") {
		return true
	}
	return strings.Contains(name, ".cpu.") || strings.Contains(name, ".idle") ||
		strings.Contains(name, ".iowait") || strings.Contains(name, ".stolen")
}

func isLatencyMetric(name string) bool {
	return strings.Contains(name, ".await") || strings.Contains(name, ".svctm") ||
		strings.Contains(name, ".latency") || strings.Contains(name, ".duration")
}

func isBooleanMetric(name string) bool {
	return strings.Contains(name, ".ready") || strings.Contains(name, ".healthy") ||
		strings.Contains(name, ".isLeader") || strings.Contains(name, ".alive")
}

func pctBucket(val float64) string {
	switch {
	case val <= 10:
		return "[V_PCT_0]"
	case val <= 25:
		return "[V_PCT_1]"
	case val <= 50:
		return "[V_PCT_2]"
	case val <= 75:
		return "[V_PCT_3]"
	case val <= 90:
		return "[V_PCT_4]"
	case val <= 95:
		return "[V_PCT_5]"
	case val <= 99:
		return "[V_PCT_6]"
	default:
		return "[V_PCT_7]"
	}
}

func latencyBucket(val float64) string {
	switch {
	case val < 1:
		return "[V_LAT_0]"
	case val < 2:
		return "[V_LAT_1]"
	case val < 5:
		return "[V_LAT_2]"
	case val < 10:
		return "[V_LAT_3]"
	case val < 50:
		return "[V_LAT_4]"
	case val < 100:
		return "[V_LAT_5]"
	case val < 1000:
		return "[V_LAT_6]"
	default:
		return "[V_LAT_7]"
	}
}
