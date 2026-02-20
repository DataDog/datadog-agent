// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// PatternLogProcessor is a log processor that clusterizes logs into patterns.
// Patterns will be sent to various pattern based anomaly detectors.

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
)

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
			fmt.Printf("[cc] Processor: Result: (ID: %d) %+v\n", result.ClusterResult.Cluster.ID, result.ClusterResult.Signature)
			for _, anomalyDetector := range p.AnomalyDetectors {
				anomalyDetector.Process(result.ClusterInput, result.ClusterResult)
			}
		}
	}()

	return p
}

func (p *PatternLogProcessor) Name() string {
	return "pattern_log_processor"
}

func (p *PatternLogProcessor) Process(log observer.LogView) observer.LogProcessorResult {
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
	fmt.Printf("[cc] WatchdogLogAnomalyDetector: Processing batch: %d\n", len(batch))
	for _, input := range batch {
		if _, ok := w.PatternRate[input.ClustererResult.Cluster.ID]; !ok {
			w.PatternRate[input.ClustererResult.Cluster.ID] = 0
		}
		w.PatternRate[input.ClustererResult.Cluster.ID] += 1
	}

	maxRate := 0.0
	maxRatePatternID := 0
	for patternID, rate := range w.PatternRate {
		w.PatternRate[patternID] = rate * w.Alpha

		if rate < w.EvictionThreshold {
			delete(w.PatternRate, patternID)
			fmt.Printf("[cc] WatchdogLogAnomalyDetector: Evicting pattern: %d\n", patternID)
		}

		if rate > maxRate {
			maxRate = rate
			maxRatePatternID = patternID
		}
	}

	// TODO: We can't use the string repr since it's not thread safe, we should block this when necessary (once the anomaly is detected)
	fmt.Printf("[cc] WatchdogLogAnomalyDetector: Max rate: %f (pattern: %d)\n", maxRate, maxRatePatternID)
	fmt.Printf("[cc] WatchdogLogAnomalyDetector: Pattern rates: %d\n", len(w.PatternRate))
}
