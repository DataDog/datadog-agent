// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package semantics

// DDSpanAccessor provides typed access to combined DD span meta and metrics maps.
// meta (map[string]string) is the primary store for string attributes.
// metrics (map[string]float64) is the primary store for numeric attributes.
type DDSpanAccessor struct {
	meta    StringMapAccessor
	metrics MetricsMapAccessor
}

// NewDDSpanAccessor returns a DDSpanAccessor for DD span meta and metrics maps.
func NewDDSpanAccessor(meta map[string]string, metrics map[string]float64) DDSpanAccessor {
	return DDSpanAccessor{
		meta:    NewStringMapAccessor(meta),
		metrics: NewMetricsMapAccessor(metrics),
	}
}

// GetString returns the value from meta, or "" if missing.
func (a DDSpanAccessor) GetString(key string) string {
	return a.meta.GetString(key)
}

// GetFloat64 returns the value from metrics, or (0, false) if missing.
func (a DDSpanAccessor) GetFloat64(key string) (float64, bool) {
	return a.metrics.GetFloat64(key)
}

// GetInt64 returns the value from metrics as int64 if it is an exact integer, or (0, false) otherwise.
func (a DDSpanAccessor) GetInt64(key string) (int64, bool) {
	return a.metrics.GetInt64(key)
}
