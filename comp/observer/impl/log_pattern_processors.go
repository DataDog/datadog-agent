// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// PatternLogProcessor is a log processor that clusterizes logs into patterns.
// Patterns will be sent to various pattern based anomaly detectors.

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
	"github.com/DataDog/datadog-agent/comp/observer/impl/queue"
)

const FILTER_ONLY_Q_LOG = false

var tlmAnomalyCount = atomic.Int64{}

// var tlm = atomic.Int64{}

// PatternLogProcessor is a log processor that detects patterns in logs.
type PatternLogProcessor struct {
	ClustererPipeline *patterns.MultiThreadPipeline
	ResultChannel     chan *patterns.MultiThreadResult
	AnomalyDetectors  []PatternLogAnomalyDetector
}

func NewPatternLogProcessor(anomalyDetectors []PatternLogAnomalyDetector) *PatternLogProcessor {
	resultChannel := make(chan *patterns.MultiThreadResult, 4096)
	clustererPipeline := patterns.NewMultiThreadPipeline(runtime.NumCPU(), resultChannel, false)

	p := &PatternLogProcessor{
		ClustererPipeline: clustererPipeline,
		ResultChannel:     resultChannel,
		AnomalyDetectors:  anomalyDetectors,
	}

	for _, anomalyDetector := range p.AnomalyDetectors {
		anomalyDetector.SetProcessor(p)
		anomalyDetector.Run()
	}

	go func() {
		for result := range resultChannel {
			if result.ClusterResult.IsNew {
				fmt.Printf("[cc] Processor: New cluster: (ID: %d) %+v\n", result.ClusterResult.Cluster.ID, result.ClusterResult.Signature)
			}
			for _, anomalyDetector := range p.AnomalyDetectors {
				anomalyDetector.Process(result.ClusterInput, result.ClusterResult)
			}
		}
	}()

	// Metrics
	celiandebug.callbacks = append(celiandebug.callbacks, func(metrics map[string]float64) {
		metrics["anomaly_count"] = float64(tlmAnomalyCount.Swap(0))

		// Number of patterns
		nPatterns := 0
		for _, clusterer := range p.ClustererPipeline.PatternClusterers {
			nPatterns += clusterer.PatternClusterer.NumClusters()
		}
		metrics["pattern_count"] = float64(nPatterns)

		// Memory
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		metrics["mem_total_alloc"] = float64(m.TotalAlloc / 1024 / 1024)
		metrics["mem_sys"] = float64(m.Sys / 1024 / 1024)
	})

	return p
}

func (p *PatternLogProcessor) Name() string {
	return "pattern_log_processor"
}

func (p *PatternLogProcessor) Process(log observer.LogView) observer.LogProcessorResult {
	// Process only q test logs
	if FILTER_ONLY_Q_LOG && !strings.Contains(string(log.GetContent()), "q-log") {
		return observer.LogProcessorResult{}
	}
	fmt.Printf("Processing log: %s\n", string(log.GetContent()))

	p.ClustererPipeline.Process(string(log.GetContent()))

	// Results arrive asynchronously via ResultChannel
	return observer.LogProcessorResult{}
}

// What we plug after the log processor
type PatternLogAnomalyDetector interface {
	SetProcessor(processor *PatternLogProcessor)
	Run()
	Name() string
	// TODO: Pattern id not pattern to avoid multi threading issues
	Process(clustererInput *patterns.ClustererInput, clustererResult *patterns.ClusterResult)
}

type LogAnomalyDetectionProcessInput struct {
	ClustererInput  *patterns.ClustererInput
	ClustererResult *patterns.ClusterResult
}

// --- Watchdog Anomaly Detector ---
// AD algorithm similar to Watchdog
type WatchdogLogAnomalyDetector struct {
	Processor       *PatternLogProcessor
	ResultChannel   chan *observer.LogProcessorResult
	Period          time.Duration
	InputBatchMutex sync.Mutex
	InputBatch      []*LogAnomalyDetectionProcessInput
	// How much decay we apply to the pattern rate
	Alpha             float64
	EvictionThreshold float64
	// "Rate" corresponds to the new patterns count with decay applied
	PatternRate map[int]float64
	// TODO: We might optimize this in log N using segment trees
	// TODO: We should compress the baseline + the smooth part
	// Here is the structure of the history:
	// | History                      |
	// | Baseline | Current           |
	// | Baseline | Eval | Preprocess |
	// We compare the baseline to the eval part, the preprocess part is used to aggregate data to eval
	PreprocessLen int
	EvalLen       int
	BaselineLen   int
	// TODO: How to do eviction with that?
	History       map[int]TimeSeriesHistory
	SnapshotIndex int
	AlertCooldown time.Duration
	LastAlerts    map[int]time.Time
}

func NewWatchdogLogAnomalyDetector(resultChannel chan *observer.LogProcessorResult) *WatchdogLogAnomalyDetector {
	// TODO: Increase delay / sizes
	return &WatchdogLogAnomalyDetector{
		ResultChannel:     resultChannel,
		Period:            100 * time.Millisecond,
		Alpha:             0.95,
		EvictionThreshold: 0.1,
		PatternRate:       make(map[int]float64),
		PreprocessLen:     10,
		EvalLen:           5,
		BaselineLen:       20,
		History:           make(map[int]TimeSeriesHistory),
		SnapshotIndex:     0,
		AlertCooldown:     15 * time.Second,
		LastAlerts:        make(map[int]time.Time),
	}
}

// TODO: We need to wait for the history to be (fully?) filled before we can start comparing it
// Represents the history of pattern rate metrics
type TimeSeriesHistory struct {
	ClusterID  int
	Preprocess *queue.Queue
	Eval       *queue.Queue
	Baseline   *queue.Queue
}

type AlertInfo struct {
	ClusterID int
	ZScore    float64
}

// TODO: Is it better to have this in the processor and not each anomaly detector?
func (w *WatchdogLogAnomalyDetector) Run() {
	go func() {
		ticker := time.NewTicker(w.Period)
		for {
			select {
			case <-ticker.C:
				// Swap the batch and clear it
				var batch []*LogAnomalyDetectionProcessInput
				{
					w.InputBatchMutex.Lock()
					batch = w.InputBatch
					w.InputBatch = make([]*LogAnomalyDetectionProcessInput, 0)
					w.InputBatchMutex.Unlock()
				}

				// Process whole batch
				w.ProcessBatch(batch)
			}
		}
	}()
}

func (w *WatchdogLogAnomalyDetector) Name() string {
	return "watchdog_log_anomaly_detector"
}

func (w *WatchdogLogAnomalyDetector) Process(clustererInput *patterns.ClustererInput, clustererResult *patterns.ClusterResult) {
	w.InputBatchMutex.Lock()
	defer w.InputBatchMutex.Unlock()
	w.InputBatch = append(w.InputBatch, &LogAnomalyDetectionProcessInput{ClustererInput: clustererInput, ClustererResult: clustererResult})
}

func (w *WatchdogLogAnomalyDetector) SetProcessor(processor *PatternLogProcessor) {
	w.Processor = processor
}

func (w *WatchdogLogAnomalyDetector) ProcessBatch(batch []*LogAnomalyDetectionProcessInput) {
	// fmt.Printf("[cc] WatchdogLogAnomalyDetector: Processing batch: %d\n", len(batch))
	for _, input := range batch {
		if _, ok := w.PatternRate[input.ClustererResult.Cluster.ID]; !ok {
			w.PatternRate[input.ClustererResult.Cluster.ID] = 0
		}
		w.PatternRate[input.ClustererResult.Cluster.ID] += 1
	}

	// We can do anomaly detection
	if w.Snapshot() {
		/*
			if len(w.PatternRate) > 0 {
				maxRate := 0.0
				maxRatePatternID := 0
				for patternID, rate := range w.PatternRate {
					w.PatternRate[patternID] = rate * w.Alpha

					if rate < w.EvictionThreshold {
						delete(w.PatternRate, patternID)
						// fmt.Printf("[cc] WatchdogLogAnomalyDetector: Evicting pattern: %d\n", patternID)
					}

					if rate > maxRate {
						maxRate = rate
						maxRatePatternID = patternID
					}
				}

				clusterInfo, err := w.Processor.ClustererPipeline.GetClusterInfo(maxRatePatternID)
				if err != nil {
					fmt.Printf("[cc] WatchdogLogAnomalyDetector: Error getting cluster info: %v\n", err)
					return
				}

				fmt.Printf("[cc] WatchdogLogAnomalyDetector: Max Rate Cluster: Rate: %f, ID: %d, Pattern: %s\n", maxRate, maxRatePatternID, clusterInfo.PatternString)
				fmt.Printf("[cc] WatchdogLogAnomalyDetector: Pattern rates: %d\n", len(w.PatternRate))
			}
		*/

		w.DetectAnomalies()
	}
}

// Will update the history with the current pattern rate metrics
// Returns whether we updated the eval / baseline part of the history (=> preprocess buffer is full)
func (w *WatchdogLogAnomalyDetector) Snapshot() bool {
	for clusterID, rate := range w.PatternRate {
		if _, ok := w.History[clusterID]; !ok {
			w.History[clusterID] = TimeSeriesHistory{ClusterID: clusterID, Preprocess: queue.NewQueue(), Eval: queue.NewQueue(), Baseline: queue.NewQueue()}
		}
		w.History[clusterID].Preprocess.Enqueue(rate)
	}
	w.SnapshotIndex++
	if w.SnapshotIndex >= w.PreprocessLen {
		w.SnapshotIndex = 0

		// Update eval and baseline with the new preprocess data
		for _, history := range w.History {
			// Do a simple average
			preprocessData := 0.0
			nItem := 0
			for {
				value, ok := history.Preprocess.Dequeue()
				if !ok {
					break
				}
				preprocessData += value
				nItem++
			}
			preprocessData /= float64(nItem)

			history.Eval.Enqueue(preprocessData)
			if history.Eval.Len() > w.EvalLen {
				value, ok := history.Eval.Dequeue()
				if !ok {
					// TODO: We should panic
					break
				}
				history.Baseline.Enqueue(value)
			}

			if history.Baseline.Len() > w.BaselineLen {
				_, ok := history.Baseline.Dequeue()
				if !ok {
					// TODO: We should panic
					break
				}
			}
		}

		return true
	}

	return false
}

// DetectAnomalies detects anomalies in the history
func (w *WatchdogLogAnomalyDetector) DetectAnomalies() {
	// TODO
	zThreshold := 3.0

	for clusterID, history := range w.History {
		// TODO: Is half filled enough?
		if history.Baseline.Len() < w.BaselineLen/2 || history.Eval.Len() < w.EvalLen {
			continue
		}

		// fmt.Printf("[cc] WatchdogLogAnomalyDetector: Detecting anomalies for cluster: %d\n", clusterID)

		// Simple z-score
		baselineMean := 0.0
		baselineStddev := 0.0
		baselineSlice := history.Baseline.Slice()
		for _, val := range baselineSlice {
			baselineMean += val
		}
		baselineMean /= float64(len(baselineSlice))
		for _, val := range baselineSlice {
			baselineStddev += (val - baselineMean) * (val - baselineMean)
		}
		baselineStddev /= float64(len(baselineSlice))
		baselineStddev = math.Sqrt(baselineStddev)
		baselineStddev = math.Max(baselineStddev, 1e-6)

		// TODO: What to do with the eval part? Not useful for this method
		// TODO: Directly send the rates to metrics AD?
		evalMean := 0.0
		evalSlice := history.Eval.Slice()
		for _, val := range evalSlice {
			evalMean += val
		}
		evalMean /= float64(len(evalSlice))

		zScore := (evalMean - baselineMean) / baselineStddev
		// TODO: Could be +Inf on some cases
		if math.Abs(zScore) > zThreshold {
			// TODO: Lock...
			alertInfo := AlertInfo{ClusterID: clusterID, ZScore: zScore}
			w.OnAnomalyDetected(alertInfo)
		}
	}
}

func (w *WatchdogLogAnomalyDetector) OnAnomalyDetected(alert AlertInfo) {
	tlmAnomalyCount.Add(1)
	if _, ok := w.LastAlerts[alert.ClusterID]; !ok || time.Since(w.LastAlerts[alert.ClusterID]) >= w.AlertCooldown {
		w.LastAlerts[alert.ClusterID] = time.Now()

		clusterInfo, err := w.Processor.ClustererPipeline.GetClusterInfo(alert.ClusterID)
		if err != nil {
			fmt.Printf("[cc] WatchdogLogAnomalyDetector: Error getting cluster info: %v\n", err)
			return
		}

		fmt.Printf("[cc] WatchdogLogAnomalyDetector: ALERT: Cluster: %d, Z-Score: %f, Pattern: %s\n", alert.ClusterID, alert.ZScore, clusterInfo.PatternString)
	}
}
