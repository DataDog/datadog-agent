// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package events holds events related files
package events

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// AnomalyDetectionLimiter limiter specific to anomaly detection
type AnomalyDetectionLimiter struct {
	limiter *utils.Limiter[string]
}

// Allow returns whether the event is allowed
func (al *AnomalyDetectionLimiter) Allow(event Event) bool {
	return al.limiter.Allow(event.GetWorkloadID())
}

// SwapStats return dropped and allowed stats
func (al *AnomalyDetectionLimiter) SwapStats() []utils.LimiterStat {
	return al.limiter.SwapStats()
}

// NewAnomalyDetectionLimiter returns a new rate limiter which is bucketed by workload ID
func NewAnomalyDetectionLimiter(numWorkloads int, numEventsAllowedPerPeriod int, period time.Duration) (*AnomalyDetectionLimiter, error) {
	limiter, err := utils.NewLimiter[string](numWorkloads, numEventsAllowedPerPeriod, period)
	if err != nil {
		return nil, err
	}

	return &AnomalyDetectionLimiter{
		limiter: limiter,
	}, nil
}
