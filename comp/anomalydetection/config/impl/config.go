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
	return anomalydetectionconfig.SmartSeverityProfilesEnabled(cfg)
}

// ReportingEventsEnabled returns whether Datadog anomaly events are enabled.
func ReportingEventsEnabled(cfg anomalydetectionconfig.BoolConfig) bool {
	return anomalydetectionconfig.ReportingEventsEnabled(cfg)
}

// AnomalyScorerDryRunEnabled returns whether the scorer should run in shadow
// mode for telemetry without output side effects.
func AnomalyScorerDryRunEnabled(cfg anomalydetectionconfig.BoolConfig) bool {
	return anomalydetectionconfig.AnomalyScorerDryRunEnabled(cfg)
}

// RecordingEnabled returns whether anomaly-detection raw signal recording is enabled.
func RecordingEnabled(cfg anomalydetectionconfig.BoolConfig) bool {
	return anomalydetectionconfig.RecordingEnabled(cfg)
}

// ObserverRequired returns whether the observer pipeline should start.
func ObserverRequired(cfg anomalydetectionconfig.BoolConfig) bool {
	return anomalydetectionconfig.ObserverRequired(cfg)
}

// ScorerRequired returns whether the anomaly scorer should be constructed.
func ScorerRequired(cfg anomalydetectionconfig.BoolConfig) bool {
	return anomalydetectionconfig.ScorerRequired(cfg)
}
