// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func initAnomalyDetection(config pkgconfigmodel.Setup) {
	// Master switch — off by default to ensure zero overhead in normal deployments.
	config.BindEnvAndSetDefault("anomaly_detection.enabled", false)

	// Per-signal ingestion toggles
	config.BindEnvAndSetDefault("anomaly_detection.logs.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.metrics.enabled", true)

	// Agent-internal log capture
	config.BindEnvAndSetDefault("anomaly_detection.agent_logs.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.agent_logs.sample_rate_info", 0.2)
	config.BindEnvAndSetDefault("anomaly_detection.agent_logs.sample_rate_debug", 0.05)
	config.BindEnvAndSetDefault("anomaly_detection.agent_logs.sample_rate_trace", 0.0)

	// Anomaly event reporting
	config.BindEnvAndSetDefault("anomaly_detection.reporting.enabled", false)

	// Parquet recording
	config.BindEnvAndSetDefault("anomaly_detection.recording.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.recording.output_dir", "/var/run/datadog/anomaly_detection")
	config.BindEnvAndSetDefault("anomaly_detection.recording.flush_interval", 60*time.Second)
	config.BindEnvAndSetDefault("anomaly_detection.recording.retention", 24*time.Hour)

	// High-frequency check overrides
	config.BindEnvAndSetDefault("anomaly_detection.checks.high_frequency_system", false)
	config.BindEnvAndSetDefault("anomaly_detection.checks.high_frequency_containers", false)

	// Debug tooling
	config.BindEnvAndSetDefault("anomaly_detection.debug.dump_path", "")
	config.BindEnvAndSetDefault("anomaly_detection.debug.dump_interval", 0)
	config.BindEnvAndSetDefault("anomaly_detection.debug.events_dump_path", "")

	// Detector toggles
	config.BindEnvAndSetDefault("anomaly_detection.detectors.log_metrics_extractor.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.connection_error_extractor.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.log_pattern_extractor.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.cusum.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.bocpd.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.rrcf.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.scanmw.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.scanwelch.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.cross_signal.enabled", false)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.time_cluster.enabled", true)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.time_cluster.min_cluster_size", 0)
	config.BindEnvAndSetDefault("anomaly_detection.detectors.passthrough.enabled", false)
}
