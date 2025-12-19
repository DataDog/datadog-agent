// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package impl provides the implementation for the drift detector component
package impl

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/def"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type dependencies struct {
	Config    config.Component
	Telemetry telemetry.Component
}

type driftDetector struct {
	config  common.Config
	manager *driftDetectorManager
	enabled bool
}

// NewProvides creates a new drift detector component
func NewProvides(deps dependencies) def.Component {
	// Load configuration from agent config
	config := loadConfigFromAgent(deps.Config)

	// Set telemetry component
	config.Telemetry = deps.Telemetry

	// Create manager for per-source drift detection
	manager := newDriftDetectorManager(config)

	detector := &driftDetector{
		config:  config,
		manager: manager,
		enabled: config.Embedding.Enabled,
	}

	return detector
}

// ProcessLog implements Component.ProcessLog
// This is the legacy single-pipeline API - routes to a default source
func (d *driftDetector) ProcessLog(timestamp time.Time, content string) {
	if !d.enabled {
		return
	}
	d.manager.ProcessLog("default", timestamp, content)
}

// ProcessLogWithSource processes a log from a specific source
func (d *driftDetector) ProcessLogWithSource(sourceKey string, timestamp time.Time, content string) {
	if !d.enabled {
		return
	}
	d.manager.ProcessLog(sourceKey, timestamp, content)
}

// Start implements Component.Start
func (d *driftDetector) Start() error {
	if !d.enabled {
		log.Info("Drift detector is disabled")
		return nil
	}

	log.Info("Starting drift detector manager (per-source mode)")
	return d.manager.Start()
}

// Stop implements Component.Stop
func (d *driftDetector) Stop() {
	if !d.enabled {
		return
	}

	log.Info("Stopping drift detector manager")
	d.manager.Stop()
}

// IsEnabled implements Component.IsEnabled
func (d *driftDetector) IsEnabled() bool {
	return d.enabled
}

// GetStats returns statistics about active drift detectors
func (d *driftDetector) GetStats() map[string]interface{} {
	return d.manager.GetStats()
}

// loadConfigFromAgent loads drift detection configuration from the agent config
func loadConfigFromAgent(cfg config.Component) common.Config {
	config := common.NewDefaultConfig()

	// Load from agent config if available
	config.Embedding.Enabled = cfg.GetBool("logs_config.drift_detection.enabled")
	config.Embedding.ServerURL = cfg.GetString("logs_config.drift_detection.embedding_url")
	config.Embedding.Model = cfg.GetString("logs_config.drift_detection.embedding_model")
	config.Window.Size = cfg.GetDuration("logs_config.drift_detection.window_size")
	config.Window.Step = cfg.GetDuration("logs_config.drift_detection.window_step")

	config.Template.EntropyThreshold = cfg.GetFloat64("logs_config.drift_detection.entropy_threshold")

	config.Alert.WarningThreshold = cfg.GetFloat64("logs_config.drift_detection.warning_threshold")

	config.Alert.CriticalThreshold = cfg.GetFloat64("logs_config.drift_detection.critical_threshold")

	config.DMD.TimeDelay = cfg.GetInt("logs_config.drift_detection.dmd_time_delay")

	config.DMD.RLSLambda = cfg.GetFloat64("logs_config.drift_detection.rls_lambda")

	config.DMD.ErrorHistory = cfg.GetInt("logs_config.drift_detection.error_history_size")

	config.Manager.CleanupInterval = cfg.GetDuration("logs_config.drift_detection.cleanup_interval")

	config.Manager.MaxIdleTime = cfg.GetDuration("logs_config.drift_detection.max_idle_time")

	return config
}
