// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package anomalydetectionconfig defines shared anomaly-detection config
// contracts used across multiple components.
// It is useful to get implicit configurations, for instance if smart log
// sampling enabled, it will enable automatically anomaly detection.
package anomalydetectionconfig

const (
	AnomalyDetectionEnabledConfigKey      = "anomaly_detection.enabled"
	AnomalyScorerEnabledConfigKey         = "anomaly_detection.anomaly_scorer.enabled"
	SmartSeverityProfilesEnabledConfigKey = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.enabled"
)

// BoolConfig is the subset of config readers needed for anomaly-detection gate
// evaluation. Both the core config component and the global pkg/config reader
// satisfy this interface.
type BoolConfig interface {
	GetBool(key string) bool
	IsConfigured(key string) bool
}
