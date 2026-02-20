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

// --- Anomaly Detectors ---
// What we plug after the log processor
type PatternLogAnomalyDetector interface {
	Run()
	Name() string
	// TODO: Pattern id not pattern to avoid multi threading issues
	Process(clustererInput *patterns.ClustererInput, clustererResult *patterns.ClusterResult)
}

type LogAnomalyDetectionProcessInput struct {
	ClustererInput  *patterns.ClustererInput
	ClustererResult *patterns.ClusterResult
}

// AD algorithm similar to Watchdog
type WatchdogLogAnomalyDetector struct {
	ResultChannel   chan *observer.LogProcessorResult
	Period          time.Duration
	InputBatchMutex sync.Mutex
	InputBatch      []*LogAnomalyDetectionProcessInput
}

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
					defer w.InputBatchMutex.Unlock()
					batch = w.InputBatch
					w.InputBatch = make([]*LogAnomalyDetectionProcessInput, 0)
				}

				// Process whole batch
				fmt.Printf("[cc] WatchdogLogAnomalyDetector: Processing batch: %d\n", len(batch))

				// TODO
				w.ResultChannel <- &observer.LogProcessorResult{}
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
