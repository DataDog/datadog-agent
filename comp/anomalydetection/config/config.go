// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package config defines shared anomaly-detection config helpers used across
// multiple packages.
package config

import pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"

const (
	AnomalyDetectionRecordingEnabledConfigKey = "anomaly_detection.recording.enabled"
	AnomalyScorerDryRunEnabledConfigKey       = "anomaly_detection.anomaly_scorer.dry_run.enabled"
	ReportingEventsEnabledConfigKey           = "anomaly_detection.reporting.events.enabled"
	SmartSeverityProfilesEnabledConfigKey     = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.enabled"
)

// SmartSeverityProfilesEnabled returns whether smart severity profiles are enabled.
func SmartSeverityProfilesEnabled(cfg pkgconfigmodel.Reader) bool {
	return cfg.GetBool(SmartSeverityProfilesEnabledConfigKey)
}

// ReportingEventsEnabled returns whether Datadog anomaly events are enabled.
func ReportingEventsEnabled(cfg pkgconfigmodel.Reader) bool {
	return cfg.GetBool(ReportingEventsEnabledConfigKey)
}

// AnomalyScorerDryRunEnabled returns whether the scorer should run in shadow
// mode for telemetry without output side effects.
func AnomalyScorerDryRunEnabled(cfg pkgconfigmodel.Reader) bool {
	return cfg.GetBool(AnomalyScorerDryRunEnabledConfigKey)
}

// RecordingEnabled returns whether anomaly-detection raw signal recording is enabled.
func RecordingEnabled(cfg pkgconfigmodel.Reader) bool {
	return cfg.GetBool(AnomalyDetectionRecordingEnabledConfigKey)
}

// ObserverRequired returns whether the observer pipeline should start.
func ObserverRequired(cfg pkgconfigmodel.Reader) bool {
	return SmartSeverityProfilesEnabled(cfg) ||
		ReportingEventsEnabled(cfg) ||
		AnomalyScorerDryRunEnabled(cfg) ||
		RecordingEnabled(cfg)
}

// ScorerRequired returns whether the anomaly scorer should be constructed.
func ScorerRequired(cfg pkgconfigmodel.Reader) bool {
	return SmartSeverityProfilesEnabled(cfg) ||
		ReportingEventsEnabled(cfg) ||
		AnomalyScorerDryRunEnabled(cfg)
}
