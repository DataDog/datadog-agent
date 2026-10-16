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

// PreviewCollection captures the same collection performed by the agent check,
// with optional health, interface status, and metadata output.
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
	if gnmiClient == nil {
		return lastMetadataReport, errors.New("gNMI client is nil")
	}
	capture.Reset()
	lastMetadataReport, err := CollectSnapshot(capture, cfg, gnmiClient, gnmiClient.Snapshot(), bandwidthState, lastMetadataReport, now, opts)
	if err != nil {
		return lastMetadataReport, err
	}
	capture.Commit()
	return lastMetadataReport, nil
}

// CollectSnapshot reports health, profile metrics, derived metrics, interface
// status, and periodic metadata for both the check and preview. The caller owns
// its sender and must commit it after successful collection.
func CollectSnapshot(
	s Sender,
	cfg *config.CheckConfig,
	gnmiClient PreviewClient,
	snapshot []client.CachedValue,
	bandwidthState BandwidthState,
	lastMetadataReport time.Time,
	now time.Time,
	opts PreviewOptions,
) (time.Time, error) {
	if cfg == nil {
		return lastMetadataReport, errors.New("check config is nil")
	}
	if gnmiClient == nil {
		return lastMetadataReport, errors.New("gNMI client is nil")
	}
	if bandwidthState == nil {
		return lastMetadataReport, errors.New("bandwidth state is nil")
	}

	interval := time.Duration(cfg.Instance.MinCollectionInterval) * time.Second
	freshSnapshot := FilterStale(snapshot, DefaultStalenessThreshold(interval), now)

	if opts.IncludeHealth {
		stats := HealthStats{
			StreamState:      gnmiClient.StreamState(),
			ReconnectCount:   gnmiClient.ReconnectAttempts(),
			ReceivedSamples:  gnmiClient.ReceivedSamples(),
			SampleAgeSeconds: OldestSampleAgeSeconds(snapshot, now),
		}
		if err := ReportHealth(s, cfg, snapshot, stats); err != nil {
			return lastMetadataReport, err
		}
	}

	// The state must also be pruned when the stream has no fresh samples.
	bandwidthState.PruneInactive(now, 3*interval)
	if len(freshSnapshot) > 0 {
		if err := ReportMetrics(s, cfg, freshSnapshot, snapshot); err != nil {
			return lastMetadataReport, err
		}
		if err := ReportDerivedMetrics(s, cfg, freshSnapshot, bandwidthState); err != nil {
			return lastMetadataReport, err
		}
	}

	readyForMetadata := gnmiClient.StreamState() == client.StreamStateConnected &&
		gnmiClient.Synchronized() &&
		InterfaceSnapshotComplete(snapshot, cfg.Profile.Metadata)
	if opts.IncludeInterfaceStatus && readyForMetadata {
		if err := ReportInterfaceStatus(s, cfg, snapshot); err != nil {
			return lastMetadataReport, err
		}
	}

	if opts.IncludeMetadata && readyForMetadata && ShouldReportMetadata(lastMetadataReport, MetadataCollectionInterval(cfg), now) {
		sent, err := ReportMetadata(s, cfg, snapshot, now)
		if err != nil {
			return lastMetadataReport, err
		}
		if sent {
			lastMetadataReport = now
		}
	}
	return lastMetadataReport, nil
}
