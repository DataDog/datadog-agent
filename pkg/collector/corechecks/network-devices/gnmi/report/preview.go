// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
)

// PreviewOptions controls which metric families are collected during preview.
type PreviewOptions struct {
	IncludeHealth          bool
	IncludeInterfaceStatus bool
	IncludeMetadata        bool
}

// DefaultPreviewOptions returns the default preview collection options.
func DefaultPreviewOptions() PreviewOptions {
	return PreviewOptions{
		IncludeHealth:          true,
		IncludeInterfaceStatus: true,
		IncludeMetadata:        true,
	}
}

// PreviewClient exposes the gNMI client state needed for metric preview.
type PreviewClient interface {
	Snapshot() []client.CachedValue
	StreamState() client.StreamState
	Synchronized() bool
	ReconnectAttempts() int
	ReceivedSamples() int
}

// PreviewCollection mirrors the metric emission path of the gNMI check Run method.
func PreviewCollection(
	capture *CaptureSender,
	cfg *config.CheckConfig,
	gnmiClient PreviewClient,
	bandwidthState BandwidthState,
	lastMetadataReport time.Time,
	now time.Time,
	opts PreviewOptions,
) (time.Time, error) {
	if capture == nil {
		return lastMetadataReport, errors.New("capture sender is nil")
	}
	if cfg == nil {
		return lastMetadataReport, errors.New("check config is nil")
	}
	if gnmiClient == nil {
		return lastMetadataReport, errors.New("gNMI client is nil")
	}
	if bandwidthState == nil {
		return lastMetadataReport, errors.New("bandwidth state is nil")
	}

	capture.Reset()

	interval := time.Duration(cfg.Instance.MinCollectionInterval) * time.Second
	snapshot := gnmiClient.Snapshot()
	stalenessThreshold := DefaultStalenessThreshold(interval)
	freshSnapshot := FilterStale(snapshot, stalenessThreshold, now)

	healthStats := HealthStats{
		StreamState:      gnmiClient.StreamState(),
		ReconnectCount:   gnmiClient.ReconnectAttempts(),
		ReceivedSamples:  gnmiClient.ReceivedSamples(),
		SampleAgeSeconds: OldestSampleAgeSeconds(snapshot, now),
	}
	if opts.IncludeHealth {
		if err := ReportHealth(capture, cfg, snapshot, healthStats); err != nil {
			return lastMetadataReport, err
		}
	}

	if len(freshSnapshot) > 0 {
		if err := ReportMetrics(capture, cfg, freshSnapshot, snapshot); err != nil {
			return lastMetadataReport, err
		}
		if err := ReportDerivedMetrics(capture, cfg, freshSnapshot, bandwidthState); err != nil {
			return lastMetadataReport, err
		}
	}

	readyForMetadata := gnmiClient.StreamState() == client.StreamStateConnected &&
		gnmiClient.Synchronized() &&
		InterfaceSnapshotComplete(snapshot, cfg.Profile.Metadata)

	if opts.IncludeInterfaceStatus && readyForMetadata {
		if err := ReportInterfaceStatus(capture, cfg, snapshot); err != nil {
			return lastMetadataReport, err
		}
	}

	if opts.IncludeMetadata && readyForMetadata {
		sent, err := ReportMetadata(capture, cfg, snapshot, now)
		if err != nil {
			return lastMetadataReport, err
		}
		if sent {
			lastMetadataReport = now
		}
	}

	capture.Commit()
	return lastMetadataReport, nil
}
