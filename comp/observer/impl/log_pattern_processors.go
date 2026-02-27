// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
	"github.com/DataDog/datadog-agent/comp/observer/impl/queue"
)

// logADTags captures the grouping dimensions for log anomaly detection.
type logADTags struct {
	Env     string
	PodName string
	Service string
	Source  string
	DirName string
}

func parseLogADTags(tags []string) logADTags {
	result := logADTags{}
	for _, tag := range tags {
		switch {
		case strings.HasPrefix(tag, "env:"):
			result.Env = strings.TrimPrefix(tag, "env:")
		case strings.HasPrefix(tag, "pod_name:"):
			result.PodName = strings.TrimPrefix(tag, "pod_name:")
		case strings.HasPrefix(tag, "service:"):
			result.Service = strings.TrimPrefix(tag, "service:")
		case strings.HasPrefix(tag, "source:"):
			result.Source = strings.TrimPrefix(tag, "source:")
		case strings.HasPrefix(tag, "dirname:"):
			result.DirName = strings.TrimPrefix(tag, "dirname:")
		}
	}
	return result
}

func (t *logADTags) fullTags() []string {
	res := make([]string, 0, 5)
	if t.Env != "" {
		res = append(res, "env:"+t.Env)
	}
	if t.PodName != "" {
		res = append(res, "pod_name:"+t.PodName)
	}
	if t.Service != "" {
		res = append(res, "service:"+t.Service)
	}
	if t.Source != "" {
		res = append(res, "source:"+t.Source)
	}
	if t.DirName != "" {
		res = append(res, "dirname:"+t.DirName)
	}
	return res
}

// logADGroupByKey groups anomalies by cluster ID and tags.
type logADGroupByKey struct {
	ClusterID    int
	Tags         *logADTags
	computedHash int64
}

func (key *logADGroupByKey) hash() int64 {
	if key.computedHash != 0 {
		return key.computedHash
	}
	h := fnv.New64()
	_ = binary.Write(h, binary.LittleEndian, int32(key.ClusterID))
	h.Write([]byte{0})
	h.Write([]byte(key.Tags.Env))
	h.Write([]byte{0})
	h.Write([]byte(key.Tags.PodName))
	h.Write([]byte{0})
	h.Write([]byte(key.Tags.Service))
	h.Write([]byte{0})
	h.Write([]byte(key.Tags.Source))
	h.Write([]byte{0})
	h.Write([]byte(key.Tags.DirName))
	key.computedHash = int64(h.Sum64())
	return key.computedHash
}

// logADInput represents a single clustered log entry held in the pending batch.
type logADInput struct {
	ClusterID int
	Tags      *logADTags
}

// logADHistory tracks the time-series history for a single group key.
// It mirrors the Preprocess → Eval → Baseline pipeline from WatchdogLogAnomalyDetector.
type logADHistory struct {
	GroupByKey *logADGroupByKey
	// Preprocess accumulates raw rate samples until the window is full.
	Preprocess *queue.Queue
	// Eval holds the recent averaged values used as the "current" signal.
	Eval *queue.Queue
	// Baseline holds older eval averages used as the reference distribution.
	Baseline *queue.Queue
}

// SimplePatternLogAD detects anomalies in log patterns using the Watchdog algorithm.
// It is intentionally single-threaded: all state is accessed only from the observer
// run-loop goroutine. Batches are finalized on a time-gap basis: when a log arrives
// more than BatchTimeout after the previous log, the accumulated batch is processed
// before adding the new entry.
type SimplePatternLogAD struct {
	Tokenizer        *patterns.Tokenizer
	PatternClusterer *patterns.PatternClusterer

	// BatchTimeout is the idle gap that triggers batch finalization.
	BatchTimeout time.Duration

	// batch holds log entries since the last finalization.
	batch       []*logADInput
	lastLogTime time.Time

	// Alpha controls exponential decay applied to pattern rates after each batch.
	Alpha float64
	// EvictionThreshold removes patterns whose decayed rate falls below this value.
	EvictionThreshold float64

	// patternRate tracks the current (decayed) log count per group key.
	patternRate map[int64]float64
	groupByKeys map[int64]*logADGroupByKey

	// History window sizes mirror WatchdogLogAnomalyDetector.
	PreprocessLen int
	EvalLen       int
	BaselineLen   int
	// history holds the per-group-key time-series history.
	history       map[int64]*logADHistory
	snapshotIndex int

	// ZThreshold is the z-score magnitude that triggers an anomaly.
	ZThreshold float64
	// AlertCooldown prevents repeated alerts for the same pattern.
	AlertCooldown time.Duration
	lastAlerts    map[int64]time.Time
}

// NewSimplePatternLogAD returns a SimplePatternLogAD with sensible defaults.
func NewSimplePatternLogAD() *SimplePatternLogAD {
	return &SimplePatternLogAD{
		Tokenizer:         patterns.NewTokenizer(),
		PatternClusterer:  patterns.NewPatternClusterer(patterns.IDComputeInfo{Offset: 0, Stride: 1, Index: 0}),
		BatchTimeout:      1 * time.Second,
		Alpha:             0.95,
		EvictionThreshold: 0.1,
		patternRate:       make(map[int64]float64),
		groupByKeys:       make(map[int64]*logADGroupByKey),
		// TODO: Update
		PreprocessLen: 2,
		EvalLen:       3,
		BaselineLen:   6,
		history:       make(map[int64]*logADHistory),
		ZThreshold:    3.0,
		AlertCooldown: 15 * time.Second,
		lastAlerts:    make(map[int64]time.Time),
	}
}

func (s *SimplePatternLogAD) Name() string {
	return "simple_pattern_log_ad"
}

// Process implements observerdef.LogProcessor. It clusters the incoming log into a
// pattern, appends it to the current batch, and finalizes the previous batch when the
// idle gap exceeds BatchTimeout.
func (s *SimplePatternLogAD) Process(log observerdef.LogView) observerdef.LogProcessorResult {
	now := time.Now()

	// Finalize the pending batch when a log arrives after a long idle gap.
	var metrics []observerdef.MetricOutput
	if !s.lastLogTime.IsZero() && now.Sub(s.lastLogTime) >= s.BatchTimeout && len(s.batch) > 0 {
		metrics = s.finalizeBatch()
	}
	s.lastLogTime = now

	content := strings.TrimSpace(string(log.GetContent()))
	if content == "" {
		return observerdef.LogProcessorResult{Metrics: metrics}
	}

	result := s.PatternClusterer.Process(content)
	if result == nil {
		return observerdef.LogProcessorResult{Metrics: metrics}
	}

	tags := parseLogADTags(log.GetTags())
	s.batch = append(s.batch, &logADInput{
		ClusterID: result.Cluster.ID,
		Tags:      &tags,
	})

	return observerdef.LogProcessorResult{Metrics: metrics}
}

// finalizeBatch counts pattern occurrences in the current batch, advances the
// history snapshot, optionally runs anomaly detection, then applies decay and
// resets the batch. It returns any metrics to emit.
func (s *SimplePatternLogAD) finalizeBatch() []observerdef.MetricOutput {
	// Count occurrences per group key.
	for _, input := range s.batch {
		key := &logADGroupByKey{ClusterID: input.ClusterID, Tags: input.Tags}
		h := key.hash()
		if _, ok := s.patternRate[h]; !ok {
			s.patternRate[h] = 0
			s.groupByKeys[h] = key
		}
		s.patternRate[h]++
	}
	s.batch = s.batch[:0]

	// Emit the current rate for each tracked pattern so the observer can store
	// them as time-series and feed them into CUSUM.
	metrics := make([]observerdef.MetricOutput, 0, len(s.patternRate))
	for h, rate := range s.patternRate {
		groupByKey := s.groupByKeys[h]
		metrics = append(metrics, observerdef.MetricOutput{
			Name:  fmt.Sprintf("_.simple_log_ad.cluster.%d", groupByKey.ClusterID),
			Value: rate,
			Tags:  groupByKey.Tags.fullTags(),
		})
	}

	// Advance the Preprocess → Eval → Baseline pipeline.
	if s.snapshot() {
		metrics = append(metrics, s.detectAnomalies()...)
	}

	// Apply exponential decay and evict cold patterns.
	for h, rate := range s.patternRate {
		newRate := rate * s.Alpha
		if newRate < s.EvictionThreshold {
			delete(s.patternRate, h)
			delete(s.groupByKeys, h)
		} else {
			s.patternRate[h] = newRate
		}
	}

	return metrics
}

// snapshot appends the current pattern rates to the Preprocess window for each
// group key. When the preprocess window fills up it is averaged and promoted into
// the Eval window, which in turn promotes its oldest value into the Baseline window.
// Returns true when the Eval/Baseline windows were updated (i.e., the preprocess
// window was full), signalling that anomaly detection may be run.
func (s *SimplePatternLogAD) snapshot() bool {
	for h, rate := range s.patternRate {
		if _, ok := s.history[h]; !ok {
			s.history[h] = &logADHistory{
				GroupByKey: s.groupByKeys[h],
				Preprocess: queue.NewQueue(),
				Eval:       queue.NewQueue(),
				Baseline:   queue.NewQueue(),
			}
		}
		s.history[h].Preprocess.Enqueue(rate)
	}

	s.snapshotIndex++
	if s.snapshotIndex < s.PreprocessLen {
		return false
	}
	s.snapshotIndex = 0

	for _, history := range s.history {
		if history.Preprocess.Len() == 0 {
			continue
		}
		sum := 0.0
		n := 0
		for {
			v, ok := history.Preprocess.Dequeue()
			if !ok {
				break
			}
			sum += v
			n++
		}
		avg := sum / float64(n)

		history.Eval.Enqueue(avg)
		if history.Eval.Len() > s.EvalLen {
			evicted, _ := history.Eval.Dequeue()
			history.Baseline.Enqueue(evicted)
		}
		if history.Baseline.Len() > s.BaselineLen {
			history.Baseline.Dequeue()
		}
	}

	return true
}

// detectAnomalies compares each group key's eval window mean against its baseline
// distribution using a z-score. When the z-score magnitude exceeds ZThreshold an
// anomaly metric is emitted (subject to AlertCooldown).
func (s *SimplePatternLogAD) detectAnomalies() []observerdef.MetricOutput {
	var metrics []observerdef.MetricOutput
	now := time.Now()

	for h, history := range s.history {
		if history.Baseline.Len() < s.BaselineLen/2 || history.Eval.Len() < s.EvalLen {
			continue
		}

		baselineSlice := history.Baseline.Slice()
		baselineMean := 0.0
		for _, v := range baselineSlice {
			baselineMean += v
		}
		baselineMean /= float64(len(baselineSlice))

		baselineVariance := 0.0
		for _, v := range baselineSlice {
			d := v - baselineMean
			baselineVariance += d * d
		}
		baselineStddev := math.Sqrt(baselineVariance / float64(len(baselineSlice)))
		baselineStddev = math.Max(baselineStddev, 1e-6)

		evalSlice := history.Eval.Slice()
		evalMean := 0.0
		for _, v := range evalSlice {
			evalMean += v
		}
		evalMean /= float64(len(evalSlice))

		zScore := (evalMean - baselineMean) / baselineStddev
		if math.Abs(zScore) <= s.ZThreshold {
			continue
		}

		groupByKey := history.GroupByKey
		if lastAlert, ok := s.lastAlerts[h]; ok && now.Sub(lastAlert) < s.AlertCooldown {
			continue
		}
		s.lastAlerts[h] = now

		// TODO: Return anomaly as well
		cluster, err := s.PatternClusterer.GetCluster(groupByKey.ClusterID)
		if err != nil {
			fmt.Printf("[SimplePatternLogAD] Error getting cluster: %v\n", err)
			continue
		}
		fmt.Printf("[SimplePatternLogAD] Anomaly detected: %f, Pattern: %s, Tags: %v\n", zScore, cluster.PatternString(), groupByKey.Tags.fullTags())

		metrics = append(metrics, observerdef.MetricOutput{
			Name:  fmt.Sprintf("_.simple_log_ad.anomaly.cluster.%d", groupByKey.ClusterID),
			Value: zScore,
			Tags:  groupByKey.Tags.fullTags(),
		})
	}

	return metrics
}
