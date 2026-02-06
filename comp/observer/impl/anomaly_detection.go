// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package observerimpl

import (
	"strings"
	"sync"
	"time"

	logger "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly"
)

// MetricSums contains aggregated metric sums by metric name
type MetricSums struct {
	Sums map[string]float64
}

type AnomalyDetection struct {
	log                 logger.Component
	profileProcessor    *anomaly.ProfileProcessor
	detectorProcessor   *anomaly.DetectorProcessor
	profileBuffer       [][]byte
	profileMutex        sync.Mutex
	metricSums          map[string]float64
	metricSumsMutex     sync.Mutex
	traceBuffer         []*traceObs
	traceMutex          sync.Mutex
	tracePercentileCalc *TracePercentileCalculator
	logBuffer           []*logObs
	logMutex            sync.Mutex
	stopChan            chan struct{}
	wg                  sync.WaitGroup
}

func NewAnomalyDetection(log logger.Component) *AnomalyDetection {
	a := &AnomalyDetection{
		log:                 log,
		profileProcessor:    anomaly.NewProfileProcessor(),
		detectorProcessor:   anomaly.NewDetectorProcessor(log),
		profileBuffer:       make([][]byte, 0),
		metricSums:          make(map[string]float64),
		tracePercentileCalc: NewTracePercentileCalculator(),
		stopChan:            make(chan struct{}),
	}

	// Start the background processing goroutine
	a.wg.Add(1)
	go a.processProfilesPeriodically()

	return a
}

// Stop gracefully stops the background processing
func (a *AnomalyDetection) Stop() {
	close(a.stopChan)
	a.wg.Wait()
}

func (a *AnomalyDetection) ProcessMetric(metric *metricObs) {
	if !strings.HasPrefix(metric.name, "datadog") && !strings.HasPrefix(metric.name, "runtime.") {
		a.metricSumsMutex.Lock()
		if len(a.metricSums) < 10000 {
			a.metricSums[metric.name] += metric.value
		}
		a.metricSumsMutex.Unlock()
	}
}

func (a *AnomalyDetection) ProcessLog(log *logObs) {
	a.logMutex.Lock()
	defer a.logMutex.Unlock()
	if len(a.logBuffer) < 10000 {
		a.logBuffer = append(a.logBuffer, log)
	}
}

func (a *AnomalyDetection) ProcessTrace(trace *traceObs) {
	a.traceMutex.Lock()
	if len(a.traceBuffer) < 10000 {
		a.traceBuffer = append(a.traceBuffer, trace)
	}
	a.traceMutex.Unlock()
}

func (a *AnomalyDetection) ProcessProfile(profile *profileObs) {
	if profile.profileType == "go" {
		// Store the profile in the buffer
		a.profileMutex.Lock()
		if len(a.profileBuffer) < 10000 {
			a.profileBuffer = append(a.profileBuffer, profile.rawData)
		}
		a.profileMutex.Unlock()
	}
}

// processProfilesPeriodically drains and processes profiles every 10 seconds
func (a *AnomalyDetection) processProfilesPeriodically() {
	defer a.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			profiles := a.drainBuffer()
			a.log.Infof("Processing %d accumulated profiles", len(profiles))
			topFuncs, err := a.profileProcessor.GetTopFunctions(profiles, 10)
			if err != nil {
				a.log.Warnf("Failed to process profiles: %v", err)
			}

			metricSums := a.drainMetrics()
			traces := a.drainTraces()

			percentiles := a.tracePercentileCalc.CalculatePercentiles(traces)

			logs := a.drainLogs()
			logsMessages := make([]string, 0, len(logs))
			for _, log := range logs {
				if log != nil && len(log.content) > 0 {
					content := string(log.content)
					if len(content) > 300 {
						content = content[:300]
					}
					logsMessages = append(logsMessages, content)
				}
			}

			a.processAnomalyScores(topFuncs, metricSums, logsMessages, percentiles)
		case <-a.stopChan:

			return
		}
	}
}

// drainBuffer drains the profile buffer and returns all accumulated profiles
func (a *AnomalyDetection) drainBuffer() [][]byte {
	a.profileMutex.Lock()
	defer a.profileMutex.Unlock()

	profiles := make([][]byte, len(a.profileBuffer))
	copy(profiles, a.profileBuffer)
	a.profileBuffer = a.profileBuffer[:0]
	return profiles
}

func (a *AnomalyDetection) drainLogs() []*logObs {
	a.logMutex.Lock()
	defer a.logMutex.Unlock()

	logs := a.logBuffer
	a.logBuffer = nil
	return logs
}

func (a *AnomalyDetection) drainTraces() []*traceObs {
	a.traceMutex.Lock()
	defer a.traceMutex.Unlock()

	traces := a.traceBuffer
	a.traceBuffer = a.traceBuffer[:0]
	return traces
}

// drainMetrics drains the metric sums map and returns it, then resets the map
func (a *AnomalyDetection) drainMetrics() map[string]float64 {
	a.metricSumsMutex.Lock()
	defer a.metricSumsMutex.Unlock()

	// Return the existing map and reset it
	result := a.metricSums
	a.metricSums = make(map[string]float64)
	return result
}

func (a *AnomalyDetection) processAnomalyScores(
	topFuncs anomaly.TopFunctions,
	metrics map[string]float64,
	logs []string,
	percentiles TracePercentiles,
) {
	scoreResults, err := a.detectorProcessor.ComputeScores(
		topFuncs,
		metrics,
		logs,
		percentiles,
	)
	if err != nil {
		a.log.Warnf("Failed to compute anomaly scores: %v", err)
		return
	}
	a.log.Debugf("Anomaly detector scores: %v", scoreResults)
}
