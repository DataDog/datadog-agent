// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package telemetry

import "sync"

// StatsTelemetrySender contains methods needed for sending stats metrics
type StatsTelemetrySender interface {
	Count(metric string, value float64, hostname string, tags []string)
	Gauge(metric string, value float64, hostname string, tags []string)
}

// StatsTelemetryProvider handles stats telemetry and passes it on to a sender
type StatsTelemetryProvider struct {
	sender StatsTelemetrySender
	m      sync.RWMutex
}

var (
	statsProvider = &StatsTelemetryProvider{}
)

// RegisterStatsSender regsiters a sender to send the stats metrics
func RegisterStatsSender(sender StatsTelemetrySender) {
	statsProvider.m.Lock()
	defer statsProvider.m.Unlock()
	statsProvider.sender = sender
}

// GetStatsTelemetryProvider gets an instance of the current stats telemetry provider
func GetStatsTelemetryProvider() *StatsTelemetryProvider {
	return statsProvider
}

// Count reports a count metric to the sender
func (s *StatsTelemetryProvider) Count(metric string, value float64, tags []string) {
	s.m.RLock()
	defer s.m.RUnlock()
	if s.sender == nil {
		return
	}

	s.sender.Count(metric, value, "", tags)
}

// Gauge reports a gauge metric to the sender
func (s *StatsTelemetryProvider) Gauge(metric string, value float64, tags []string) {
	s.m.RLock()
	defer s.m.RUnlock()
	if s.sender == nil {
		return
	}

	s.sender.Gauge(metric, value, "", tags)
}
