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
	profileBuffer       [][]byte
	profileMutex        sync.Mutex
	metricSums          map[string]float64
	metricSumsMutex     sync.Mutex
	traceBuffer         []*traceObs
	traceMutex          sync.Mutex
	tracePercentileCalc *TracePercentileCalculator
	stopChan            chan struct{}
	wg                  sync.WaitGroup
}

func NewAnomalyDetection(log logger.Component) *AnomalyDetection {
	a := &AnomalyDetection{
		log:                 log,
		profileProcessor:    anomaly.NewProfileProcessor(),
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
		a.metricSums[metric.name] += metric.value
		a.metricSumsMutex.Unlock()
	}
}

func (a *AnomalyDetection) ProcessLog(log *logObs) {
	a.log.Debugf("Processing log: %v", log)
}

func (a *AnomalyDetection) ProcessTrace(trace *traceObs) {
	a.traceMutex.Lock()
	a.traceBuffer = append(a.traceBuffer, trace)
	a.traceMutex.Unlock()
}

func (a *AnomalyDetection) ProcessProfile(profile *profileObs) {
	if profile.profileType == "go" {
		// Store the profile in the buffer
		a.profileMutex.Lock()
		if len(a.profileBuffer) < 1000 {
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
			} else if topFuncs != nil {
				a.displayTopFunctions(topFuncs)
			}

			metricSums := a.drainMetrics()
			if len(metricSums) > 0 {
				a.log.Infof("Processing %d accumulated metrics", len(metricSums))
				a.displayMetricSums(&MetricSums{Sums: metricSums})
			}
			traces := a.drainTraces()

			percentiles := a.tracePercentileCalc.CalculatePercentiles(traces)
			a.log.Infof("Trace percentiles (from %d traces): P50=%d, P95=%d, P99=%d",
				len(traces), percentiles.P50, percentiles.P95, percentiles.P99)

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

// displayTopFunctions displays the top CPU and memory consuming functions
func (a *AnomalyDetection) displayTopFunctions(topFuncs *anomaly.TopFunctions) {
	if len(topFuncs.CPU) > 0 {
		a.log.Infof("Top 10 CPU consuming functions:")
		for i, fn := range topFuncs.CPU {
			a.log.Infof("  %d. %s: %d samples", i+1, fn.Name, fn.Flat)
		}
	}

	if len(topFuncs.Memory) > 0 {
		a.log.Infof("Top 10 memory consuming functions:")
		for i, fn := range topFuncs.Memory {
			a.log.Infof("  %d. %s: %d bytes", i+1, fn.Name, fn.Bytes)
		}
	}
}

// displayMetricSums displays the aggregated metric sums
func (a *AnomalyDetection) displayMetricSums(metricSums *MetricSums) {
	if len(metricSums.Sums) > 0 {
		a.log.Infof("Aggregated metric sums (%d metrics):", len(metricSums.Sums))
		for name, sum := range metricSums.Sums {
			a.log.Infof("  %s: %f", name, sum)
		}
	}
}
