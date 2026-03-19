// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// AnomalyRateLimiter limits the number of anomalies detected grouped by a key.
// If two anomalies have different keys, then they are rate limited independently.
type AnomalyRateLimiter struct {
	CooldownMs int64
	// [anomaly key] = last anomaly time in ms
	LastAnomalyTimesMs map[int64]int64
}

// NewAnomalyRateLimiter creates a new AnomalyRateLimiter with the given cooldown.
func NewAnomalyRateLimiter(cooldownMs int64) *AnomalyRateLimiter {
	return &AnomalyRateLimiter{
		CooldownMs:         cooldownMs,
		LastAnomalyTimesMs: make(map[int64]int64),
	}
}

// TryCreateAnomaly returns true if an anomaly can be created for the given key at nowMs.
func (a *AnomalyRateLimiter) TryCreateAnomaly(key int64, nowMs int64) bool {
	if lastAnomalyTime, ok := a.LastAnomalyTimesMs[key]; ok {
		if nowMs-lastAnomalyTime < a.CooldownMs {
			return false
		}
	}
	a.LastAnomalyTimesMs[key] = nowMs
	return true
}
