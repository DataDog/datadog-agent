// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
)

const (
	metricStreamState      = "datadog.gnmi.stream_state"
	metricReconnectCount   = "datadog.gnmi.reconnect_count"
	metricReceivedSamples  = "datadog.gnmi.received_samples"
	metricSampleAgeSeconds = "datadog.gnmi.sample_age_seconds"
)

const (
	streamStateNotReady     = 0
	streamStateConnected    = 1
	streamStateReconnecting = 2
)

// HealthStats holds operational telemetry for a gNMI client snapshot.
type HealthStats struct {
	StreamState      client.StreamState
	ReconnectCount   int
	ReceivedSamples  int
	SampleAgeSeconds float64
}

// DefaultStalenessThreshold returns the default maximum sample age before suppression.
func DefaultStalenessThreshold(minCollectionInterval time.Duration) time.Duration {
	return 2 * minCollectionInterval
}

// FilterStale removes cached values older than maxAge relative to now.
func FilterStale(snapshot []client.CachedValue, maxAge time.Duration, now time.Time) []client.CachedValue {
	if maxAge <= 0 || len(snapshot) == 0 {
		return snapshot
	}

	filtered := make([]client.CachedValue, 0, len(snapshot))
	for _, cached := range snapshot {
		if cached.Entry.Timestamp.IsZero() {
			continue
		}
		if now.Sub(cached.Entry.Timestamp) <= maxAge {
			filtered = append(filtered, cached)
		}
	}
	return filtered
}

// OldestSampleAgeSeconds returns the age in seconds of the oldest timestamp in snapshot.
func OldestSampleAgeSeconds(snapshot []client.CachedValue, now time.Time) float64 {
	var oldest time.Time
	for _, cached := range snapshot {
		if cached.Entry.Timestamp.IsZero() {
			continue
		}
		if oldest.IsZero() || cached.Entry.Timestamp.Before(oldest) {
			oldest = cached.Entry.Timestamp
		}
	}
	if oldest.IsZero() {
		return 0
	}
	return now.Sub(oldest).Seconds()
}

// ReportHealth submits datadog.gnmi.* operational metrics.
func ReportHealth(s sender.Sender, cfg *config.CheckConfig, stats HealthStats) error {
	if s == nil {
		return errors.New("sender is nil")
	}
	if cfg == nil {
		return errors.New("check config is nil")
	}

	tags := buildBaseTags(cfg)
	s.Gauge(metricStreamState, streamStateValue(stats.StreamState), "", tags)
	s.Gauge(metricReconnectCount, float64(stats.ReconnectCount), "", tags)
	s.Gauge(metricReceivedSamples, float64(stats.ReceivedSamples), "", tags)
	s.Gauge(metricSampleAgeSeconds, stats.SampleAgeSeconds, "", tags)
	return nil
}

func streamStateValue(state client.StreamState) float64 {
	switch state {
	case client.StreamStateConnected:
		return streamStateConnected
	case client.StreamStateReconnecting:
		return streamStateReconnecting
	default:
		return streamStateNotReady
	}
}
