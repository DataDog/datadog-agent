// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package info

import (
	"encoding/json"

	"go.uber.org/atomic"
)

// TraceWriterInfo represents statistics from the trace writer.
type TraceWriterInfo struct {
	// all atomic values are included as values in this struct, to simplify
	// initialization of the type.  The atomic values _must_ occur first in the
	// struct.

	Payloads          atomic.Int64
	Traces            atomic.Int64
	Events            atomic.Int64
	Spans             atomic.Int64
	Errors            atomic.Int64
	Retries           atomic.Int64
	Bytes             atomic.Int64
	BytesUncompressed atomic.Int64
	SingleMaxSize     atomic.Int64
}

// Acc accumulates stats from the incoming update info
func (twi *TraceWriterInfo) Acc(update *TraceWriterInfo) {
	twi.Payloads.Add(update.Payloads.Load())
	twi.Traces.Add(update.Traces.Load())
	twi.Events.Add(update.Events.Load())
	twi.Spans.Add(update.Spans.Load())
	twi.Errors.Add(update.Errors.Load())
	twi.Retries.Add(update.Retries.Load())
	twi.Bytes.Add(update.Bytes.Load())
	twi.BytesUncompressed.Add(update.BytesUncompressed.Load())
	twi.SingleMaxSize.Add(update.SingleMaxSize.Load())
}

// Reset all the stored info to zero
func (twi *TraceWriterInfo) Reset() {
	twi.Payloads.Store(0)
	twi.Traces.Store(0)
	twi.Events.Store(0)
	twi.Spans.Store(0)
	twi.Errors.Store(0)
	twi.Retries.Store(0)
	twi.Bytes.Store(0)
	twi.BytesUncompressed.Store(0)
	twi.SingleMaxSize.Store(0)
}

// StatsWriterInfo represents statistics from the stats writer.
type StatsWriterInfo struct {
	// all atomic values are included as values in this struct, to simplify
	// initialization of the type.  The atomic values _must_ occur first in the
	// struct.

	Payloads       atomic.Int64
	ClientPayloads atomic.Int64
	StatsBuckets   atomic.Int64
	StatsEntries   atomic.Int64
	Errors         atomic.Int64
	Retries        atomic.Int64
	Splits         atomic.Int64
	Bytes          atomic.Int64
}

// Acc accumulates stats from the incoming update info
func (swi *StatsWriterInfo) Acc(update *StatsWriterInfo) {
	swi.Payloads.Add(update.Payloads.Load())
	swi.ClientPayloads.Add(update.ClientPayloads.Load())
	swi.StatsBuckets.Add(update.StatsBuckets.Load())
	swi.StatsEntries.Add(update.StatsEntries.Load())
	swi.Errors.Add(update.Errors.Load())
	swi.Retries.Add(update.Retries.Load())
	swi.Splits.Add(update.Splits.Load())
	swi.Bytes.Add(update.Bytes.Load())
}

// Reset all the stored info to zero
func (swi *StatsWriterInfo) Reset() {
	swi.Payloads.Store(0)
	swi.ClientPayloads.Store(0)
	swi.StatsBuckets.Store(0)
	swi.StatsEntries.Store(0)
	swi.Errors.Store(0)
	swi.Retries.Store(0)
	swi.Splits.Store(0)
	swi.Bytes.Store(0)
}

// UpdateTraceWriterInfo updates internal trace writer stats
func UpdateTraceWriterInfo(tws *TraceWriterInfo) {
	ift.infoMu.Lock()
	defer ift.infoMu.Unlock()
	ift.traceWriterInfo = tws
}

func publishTraceWriterInfo() interface{} {
	ift.infoMu.RLock()
	defer ift.infoMu.RUnlock()
	return ift.traceWriterInfo
}

// MarshalJSON implements encoding/json.MarshalJSON.
func (twi *TraceWriterInfo) MarshalJSON() ([]byte, error) {
	asMap := map[string]float64{
		"Payloads":          float64(twi.Payloads.Load()),
		"Traces":            float64(twi.Traces.Load()),
		"Events":            float64(twi.Events.Load()),
		"Spans":             float64(twi.Spans.Load()),
		"Errors":            float64(twi.Errors.Load()),
		"Retries":           float64(twi.Retries.Load()),
		"Bytes":             float64(twi.Bytes.Load()),
		"BytesUncompressed": float64(twi.BytesUncompressed.Load()),
		"SingleMaxSize":     float64(twi.SingleMaxSize.Load()),
	}
	return json.Marshal(asMap)
}

// UpdateStatsWriterInfo updates internal stats writer stats
func UpdateStatsWriterInfo(sws *StatsWriterInfo) {
	ift.infoMu.Lock()
	defer ift.infoMu.Unlock()
	ift.statsWriterInfo = sws
}

func publishStatsWriterInfo() interface{} {
	ift.infoMu.RLock()
	defer ift.infoMu.RUnlock()
	return ift.statsWriterInfo
}

// MarshalJSON implements encoding/json.MarshalJSON.
func (swi *StatsWriterInfo) MarshalJSON() ([]byte, error) {
	asMap := map[string]float64{
		"Payloads":       float64(swi.Payloads.Load()),
		"ClientPayloads": float64(swi.ClientPayloads.Load()),
		"StatsBuckets":   float64(swi.StatsBuckets.Load()),
		"StatsEntries":   float64(swi.StatsEntries.Load()),
		"Errors":         float64(swi.Errors.Load()),
		"Retries":        float64(swi.Retries.Load()),
		"Splits":         float64(swi.Splits.Load()),
		"Bytes":          float64(swi.Bytes.Load()),
	}
	return json.Marshal(asMap)
}
