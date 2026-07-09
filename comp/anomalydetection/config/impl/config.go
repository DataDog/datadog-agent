// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package anomalydetectionconfigimpl implements shared anomaly-detection config
// gate evaluation used by multiple runtime entry points.
package anomalydetectionconfigimpl

import anomalydetectionconfig "github.com/DataDog/datadog-agent/comp/anomalydetection/config/def"

const (
	AnomalyDetectionRecordingEnabledConfigKey = anomalydetectionconfig.AnomalyDetectionRecordingEnabledConfigKey
	AnomalyScorerDryRunEnabledConfigKey       = anomalydetectionconfig.AnomalyScorerDryRunEnabledConfigKey
	ReportingEventsEnabledConfigKey           = anomalydetectionconfig.ReportingEventsEnabledConfigKey
	SmartSeverityProfilesEnabledConfigKey     = anomalydetectionconfig.SmartSeverityProfilesEnabledConfigKey
)

// SmartSeverityProfilesEnabled returns whether smart severity profiles are enabled.
func SmartSeverityProfilesEnabled(cfg anomalydetectionconfig.BoolConfig) bool {
	return cfg != nil && cfg.GetBool(SmartSeverityProfilesEnabledConfigKey)
}

// ReportingEventsEnabled returns whether Datadog anomaly events are enabled.
func ReportingEventsEnabled(cfg anomalydetectionconfig.BoolConfig) bool {
	return cfg != nil && cfg.GetBool(ReportingEventsEnabledConfigKey)
}

// AnomalyScorerDryRunEnabled returns whether the scorer should run in shadow
// mode for telemetry without output side effects.
func AnomalyScorerDryRunEnabled(cfg anomalydetectionconfig.BoolConfig) bool {
	return cfg != nil && cfg.GetBool(AnomalyScorerDryRunEnabledConfigKey)
}

// RecordingEnabled returns whether anomaly-detection raw signal recording is enabled.
func RecordingEnabled(cfg anomalydetectionconfig.BoolConfig) bool {
	return cfg != nil && cfg.GetBool(AnomalyDetectionRecordingEnabledConfigKey)
}

// ObserverRequired returns whether the observer pipeline should start.
func ObserverRequired(cfg anomalydetectionconfig.BoolConfig) bool {
	return cfg != nil && (SmartSeverityProfilesEnabled(cfg) ||
		ReportingEventsEnabled(cfg) ||
		AnomalyScorerDryRunEnabled(cfg) ||
		RecordingEnabled(cfg))
}

// ScorerRequired returns whether the anomaly scorer should be constructed.
func ScorerRequired(cfg anomalydetectionconfig.BoolConfig) bool {
	return cfg != nil && (SmartSeverityProfilesEnabled(cfg) ||
		ReportingEventsEnabled(cfg) ||
		AnomalyScorerDryRunEnabled(cfg))
}
