// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package anomalydetectionconfigimpl implements shared anomaly-detection config
// gate evaluation used by multiple runtime entry points.
package anomalydetectionconfigimpl

import anomalydetectionconfig "github.com/DataDog/datadog-agent/comp/anomalydetection/config/def"

const (
	AnomalyDetectionEnabledConfigKey      = anomalydetectionconfig.AnomalyDetectionEnabledConfigKey
	AnomalyScorerEnabledConfigKey         = anomalydetectionconfig.AnomalyScorerEnabledConfigKey
	SmartSeverityProfilesEnabledConfigKey = anomalydetectionconfig.SmartSeverityProfilesEnabledConfigKey
)

// SmartSeverityProfilesEnabled returns whether smart severity profiles are enabled.
func SmartSeverityProfilesEnabled(cfg anomalydetectionconfig.BoolConfig) bool {
	return cfg != nil && cfg.GetBool(SmartSeverityProfilesEnabledConfigKey)
}

// EffectiveAnalysisEnabled returns whether the anomaly-detection analysis path
// should run. Smart severity profiles depend on the observer pipeline even when
// anomaly_detection.enabled is explicitly left false.
func EffectiveAnalysisEnabled(cfg anomalydetectionconfig.BoolConfig) bool {
	return cfg != nil && (cfg.GetBool(AnomalyDetectionEnabledConfigKey) || SmartSeverityProfilesEnabled(cfg))
}

// EffectiveAnomalyScorerEnabled returns whether the anomaly scorer should be
// constructed. Smart severity profiles implicitly require the scorer even when
// anomaly_detection.anomaly_scorer.enabled is unset or false.
func EffectiveAnomalyScorerEnabled(cfg anomalydetectionconfig.BoolConfig) bool {
	if cfg == nil {
		return false
	}
	if SmartSeverityProfilesEnabled(cfg) {
		return true
	}
	return cfg.IsConfigured(AnomalyScorerEnabledConfigKey) && cfg.GetBool(AnomalyScorerEnabledConfigKey)
}
