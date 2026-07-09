// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package config defines shared anomaly-detection config helpers used across
// multiple packages.
package config

const (
	AnomalyDetectionRecordingEnabledConfigKey = "anomaly_detection.recording.enabled"
	AnomalyScorerDryRunEnabledConfigKey       = "anomaly_detection.anomaly_scorer.dry_run.enabled"
	ReportingEventsEnabledConfigKey           = "anomaly_detection.reporting.events.enabled"
	SmartSeverityProfilesEnabledConfigKey     = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.enabled"
)

// BoolConfig is the subset of config readers needed for anomaly-detection gate
// evaluation. Both the core config component and the global pkg/config reader
// satisfy this interface.
type BoolConfig interface {
	GetBool(key string) bool
	IsConfigured(key string) bool
}

// SmartSeverityProfilesEnabled returns whether smart severity profiles are enabled.
func SmartSeverityProfilesEnabled(cfg BoolConfig) bool {
	return cfg.GetBool(SmartSeverityProfilesEnabledConfigKey)
}

// ReportingEventsEnabled returns whether Datadog anomaly events are enabled.
func ReportingEventsEnabled(cfg BoolConfig) bool {
	return cfg.GetBool(ReportingEventsEnabledConfigKey)
}

// AnomalyScorerDryRunEnabled returns whether the scorer should run in shadow
// mode for telemetry without output side effects.
func AnomalyScorerDryRunEnabled(cfg BoolConfig) bool {
	return cfg.GetBool(AnomalyScorerDryRunEnabledConfigKey)
}

// RecordingEnabled returns whether anomaly-detection raw signal recording is enabled.
func RecordingEnabled(cfg BoolConfig) bool {
	return cfg.GetBool(AnomalyDetectionRecordingEnabledConfigKey)
}

// ObserverRequired returns whether the observer pipeline should start.
func ObserverRequired(cfg BoolConfig) bool {
	return SmartSeverityProfilesEnabled(cfg) ||
		ReportingEventsEnabled(cfg) ||
		AnomalyScorerDryRunEnabled(cfg) ||
		RecordingEnabled(cfg)
}

// ScorerRequired returns whether the anomaly scorer should be constructed.
func ScorerRequired(cfg BoolConfig) bool {
	return SmartSeverityProfilesEnabled(cfg) ||
		ReportingEventsEnabled(cfg) ||
		AnomalyScorerDryRunEnabled(cfg)
}
