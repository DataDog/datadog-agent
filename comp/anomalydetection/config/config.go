// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package config defines shared anomaly-detection config helpers used across
// multiple packages.
package config

import (
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	AnomalyDetectionRecordingEnabledConfigKey  = "anomaly_detection.recording.enabled"
	AnomalyScorerDryRunEnabledConfigKey        = "anomaly_detection.anomaly_scorer.dry_run.enabled"
	LogPatternForwardingEnabledConfigKey       = "anomaly_detection.log_pattern_forwarding.enabled"
	LogPatternForwardingEndpointConfigKey      = "anomaly_detection.log_pattern_forwarding.endpoint"
	LogPatternForwardingRetryIntervalConfigKey = "anomaly_detection.log_pattern_forwarding.connect_retry_interval"
	LogPatternForwardingDebugLogsConfigKey     = "anomaly_detection.log_pattern_forwarding.debug_logs"
	AnomalyEventsEnabledConfigKey              = "anomaly_detection.anomaly_events.enabled"
	AnomalyEventsEndpointConfigKey             = "anomaly_detection.anomaly_events.endpoint"
	AnomalyEventsRetryIntervalConfigKey        = "anomaly_detection.anomaly_events.connect_retry_interval"
	ReportingEventsEnabledConfigKey            = "anomaly_detection.reporting.events.enabled"
	SmartSeverityProfilesEnabledConfigKey      = "logs_config.experimental_adaptive_sampling.smart_severity_profiles.enabled"
)

// Defaults for log pattern metric forwarding and anomaly event subscription.
const (
	// DefaultLogPatternForwardingEndpoint is the isolated analysis process's
	// FIT setup endpoint. Every AAD socket and producer configuration file lives
	// in one directory, /tmp/ipc-aad, which the analysis process creates with
	// mode 0700 because the FIT transport refuses a publicly readable parent.
	DefaultLogPatternForwardingEndpoint = "unix:/tmp/ipc-aad/aad.sock"
	// DefaultLogPatternForwardingRetryInterval is how often the forwarder
	// retries a connection or a broken session.
	DefaultLogPatternForwardingRetryInterval = 5 * time.Second
	// DefaultAnomalyEventsEndpoint is the isolated analysis process's FIT
	// broadcast endpoint for anomaly events.
	DefaultAnomalyEventsEndpoint = "unix:/tmp/ipc-aad/events.sock"
	// DefaultAnomalyEventsRetryInterval is how often the event subscriber
	// retries a subscription or a broken session.
	DefaultAnomalyEventsRetryInterval = 5 * time.Second
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
		LogPatternForwardingEnabled(cfg) ||
		RecordingEnabled(cfg)
}

// LogPatternForwardingEnabled returns whether virtual metrics produced from logs
// (log pattern counts) should be forwarded to the isolated analysis process.
func LogPatternForwardingEnabled(cfg pkgconfigmodel.Reader) bool {
	return cfg.GetBool(LogPatternForwardingEnabledConfigKey)
}

// LogPatternForwardingConfig describes where and how log pattern metrics are forwarded.
type LogPatternForwardingConfig struct {
	// Enabled reports whether forwarding is enabled.
	Enabled bool
	// Endpoint is the analysis process's FIT setup endpoint, in the
	// "unix:/path", "tcp:127.0.0.1:port", or bare-path forms FIT accepts.
	Endpoint string
	// RetryInterval is how long to wait before retrying a failed connection or
	// a broken session.
	RetryInterval time.Duration
	// DebugLogs makes the forwarder log one line per observed virtual metric.
	DebugLogs bool
}

// LogPatternForwarding returns the log pattern metric forwarding configuration,
// falling back to the documented defaults for unset or invalid values.
func LogPatternForwarding(cfg pkgconfigmodel.Reader) LogPatternForwardingConfig {
	out := LogPatternForwardingConfig{
		Enabled:       LogPatternForwardingEnabled(cfg),
		Endpoint:      DefaultLogPatternForwardingEndpoint,
		RetryInterval: DefaultLogPatternForwardingRetryInterval,
		DebugLogs:     cfg.GetBool(LogPatternForwardingDebugLogsConfigKey),
	}
	if endpoint := cfg.GetString(LogPatternForwardingEndpointConfigKey); endpoint != "" {
		out.Endpoint = endpoint
	}
	if interval := cfg.GetDuration(LogPatternForwardingRetryIntervalConfigKey); interval > 0 {
		out.RetryInterval = interval
	}
	return out
}

// AnomalyEventsEnabled returns whether the agent subscribes to the anomaly
// events published by the isolated analysis process.
func AnomalyEventsEnabled(cfg pkgconfigmodel.Reader) bool {
	return cfg.GetBool(AnomalyEventsEnabledConfigKey)
}

// AnomalyEventsConfig describes where and how anomaly events are subscribed to.
type AnomalyEventsConfig struct {
	// Enabled reports whether the subscription is enabled.
	Enabled bool
	// Endpoint is the analysis process's FIT broadcast endpoint, in the
	// "unix:/path", "tcp:127.0.0.1:port", or bare-path forms FIT accepts.
	Endpoint string
	// RetryInterval is how long to wait before retrying a failed subscription
	// or a broken session.
	RetryInterval time.Duration
}

// AnomalyEvents returns the anomaly event subscription configuration, falling
// back to the documented defaults for unset or invalid values.
func AnomalyEvents(cfg pkgconfigmodel.Reader) AnomalyEventsConfig {
	out := AnomalyEventsConfig{
		Enabled:       AnomalyEventsEnabled(cfg),
		Endpoint:      DefaultAnomalyEventsEndpoint,
		RetryInterval: DefaultAnomalyEventsRetryInterval,
	}
	if endpoint := cfg.GetString(AnomalyEventsEndpointConfigKey); endpoint != "" {
		out.Endpoint = endpoint
	}
	if interval := cfg.GetDuration(AnomalyEventsRetryIntervalConfigKey); interval > 0 {
		out.RetryInterval = interval
	}
	return out
}

// ScorerRequired returns whether the anomaly scorer should be constructed.
func ScorerRequired(cfg pkgconfigmodel.Reader) bool {
	return SmartSeverityProfilesEnabled(cfg) ||
		ReportingEventsEnabled(cfg) ||
		AnomalyScorerDryRunEnabled(cfg)
}
