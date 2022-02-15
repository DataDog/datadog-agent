// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package info

// TraceWriterInfo represents statistics from the trace writer.
type TraceWriterInfo struct {
	Payloads          int64
	Traces            int64
	Events            int64
	Spans             int64
	Errors            int64
	Retries           int64
	Bytes             int64
	BytesUncompressed int64
	BytesEstimated    int64
	SingleMaxSize     int64
}

// StatsWriterInfo represents statistics from the stats writer.
type StatsWriterInfo struct {
	Payloads       int64
	ClientPayloads int64
	StatsBuckets   int64
	StatsEntries   int64
	Errors         int64
	Retries        int64
	Splits         int64
	Bytes          int64
}

// UpdateTraceWriterInfo updates internal trace writer stats
func UpdateTraceWriterInfo(tws TraceWriterInfo) {
	infoMu.Lock()
	defer infoMu.Unlock()
	traceWriterInfo = tws
}

func publishTraceWriterInfo() interface{} {
	infoMu.RLock()
	defer infoMu.RUnlock()
	return traceWriterInfo
}

// UpdateStatsWriterInfo updates internal stats writer stats
func UpdateStatsWriterInfo(sws StatsWriterInfo) {
	infoMu.Lock()
	defer infoMu.Unlock()
	statsWriterInfo = sws
}

func publishStatsWriterInfo() interface{} {
	infoMu.RLock()
	defer infoMu.RUnlock()
	return statsWriterInfo
}
