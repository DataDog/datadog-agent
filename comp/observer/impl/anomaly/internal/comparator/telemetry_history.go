// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package comparator

import (
	"log"

	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/collector"
	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/detector"
)

// TelemetryHistory manages historical telemetry data for comparison purposes
type TelemetryHistory struct {
	telemetries    []collector.Telemetry
	debug          bool
	logger         *log.Logger
	comparisonMode collector.ComparisonMode
	comparator     *TelemetryComparator
}

// NewTelemetryHistory creates a new telemetry history manager
func NewTelemetryHistory(debug bool, logger *log.Logger, mode collector.ComparisonMode, comparator *TelemetryComparator) *TelemetryHistory {
	return &TelemetryHistory{
		debug:          debug,
		logger:         logger,
		comparisonMode: mode,
		comparator:     comparator,
	}
}

// Add adds a new telemetry to the history and computes comparison scores
func (h *TelemetryHistory) Add(t collector.Telemetry) (detector.TelemetryResult, error) {
	highestResult, err := h.computeScore(t)
	h.telemetries = append(h.telemetries, t)
	if len(h.telemetries) > 10000 {
		h.telemetries = h.telemetries[1:]
	}

	return highestResult, err
}

// computeScore compares the given telemetry against all historical telemetries
// and returns the minimum comparison scores across all metrics
func (h *TelemetryHistory) computeScore(t collector.Telemetry) (detector.TelemetryResult, error) {

	// First pass: find min value for each field across all results
	minResults := detector.TelemetryResult{
		CPU:                10000,
		Mem:                10000,
		Err:                10000,
		ClientSentByClient: 10000,
		ClientSentByServer: 10000,
		ServerSentByClient: 10000,
		ServerSentByServer: 10000,
		TraceP50:           10000,
		TraceP95:           10000,
		TraceP99:           10000,
		Metrics:            10000,
	}

	// Track which telemetry had the min for each field
	type minTimestamp struct {
		cpu                string
		mem                string
		err                string
		clientSentByClient string
		clientSentByServer string
		serverSentByClient string
		serverSentByServer string
		traceP50           string
		traceP95           string
		traceP99           string
		metrics            string
	}
	minTimes := minTimestamp{}

	for _, t2 := range h.telemetries {
		results := h.comparator.Compare(t, t2, h.comparisonMode)
		timeStr := t2.Time.Format("01-02-2006 15:04:05")

		if results.CPU < minResults.CPU {
			minResults.CPU = results.CPU
			minTimes.cpu = timeStr
		}
		if results.Mem < minResults.Mem {
			minResults.Mem = results.Mem
			minTimes.mem = timeStr
		}
		if results.Err < minResults.Err {
			minResults.Err = results.Err
			minTimes.err = timeStr
		}
		if results.ClientSentByClient < minResults.ClientSentByClient {
			minResults.ClientSentByClient = results.ClientSentByClient
			minTimes.clientSentByClient = timeStr
		}
		if results.ClientSentByServer < minResults.ClientSentByServer {
			minResults.ClientSentByServer = results.ClientSentByServer
			minTimes.clientSentByServer = timeStr
		}
		if results.ServerSentByClient < minResults.ServerSentByClient {
			minResults.ServerSentByClient = results.ServerSentByClient
			minTimes.serverSentByClient = timeStr
		}
		if results.ServerSentByServer < minResults.ServerSentByServer {
			minResults.ServerSentByServer = results.ServerSentByServer
			minTimes.serverSentByServer = timeStr
		}
		if results.TraceP50 < minResults.TraceP50 {
			minResults.TraceP50 = results.TraceP50
			minTimes.traceP50 = timeStr
		}
		if results.TraceP95 < minResults.TraceP95 {
			minResults.TraceP95 = results.TraceP95
			minTimes.traceP95 = timeStr
		}
		if results.TraceP99 < minResults.TraceP99 {
			minResults.TraceP99 = results.TraceP99
			minTimes.traceP99 = timeStr
		}
		if results.Metrics < minResults.Metrics {
			minResults.Metrics = results.Metrics
			minTimes.metrics = timeStr
		}
	}

	// Log debug information if debug mode is enabled
	if h.debug && h.logger != nil {
		h.logger.Printf("=== Compute Score Debug ===\n")
		h.logger.Printf("Min values and timestamps:\n")
		h.logger.Printf("  CPU:%.3f @ %s\n", minResults.CPU, minTimes.cpu)
		h.logger.Printf("  Mem:%.3f @ %s\n", minResults.Mem, minTimes.mem)
		h.logger.Printf("  Err:%.3f @ %s\n", minResults.Err, minTimes.err)
		h.logger.Printf("  CC:%.3f @ %s\n", minResults.ClientSentByClient, minTimes.clientSentByClient)
		h.logger.Printf("  CS:%.3f @ %s\n", minResults.ClientSentByServer, minTimes.clientSentByServer)
		h.logger.Printf("  SC:%.3f @ %s\n", minResults.ServerSentByClient, minTimes.serverSentByClient)
		h.logger.Printf("  SS:%.3f @ %s\n", minResults.ServerSentByServer, minTimes.serverSentByServer)
		h.logger.Printf("  p50:%.3f @ %s\n", minResults.TraceP50, minTimes.traceP50)
		h.logger.Printf("  p95:%.3f @ %s\n", minResults.TraceP95, minTimes.traceP95)
		h.logger.Printf("  p99:%.3f @ %s\n", minResults.TraceP99, minTimes.traceP99)
		h.logger.Printf("  Metrics:%.3f @ %s\n", minResults.Metrics, minTimes.metrics)
		h.logger.Printf("===========================\n\n")
	}

	return minResults, nil
}
