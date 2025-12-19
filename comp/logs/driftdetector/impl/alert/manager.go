// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package alert

import (
	"context"
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Manager handles anomaly detection and alerting
type Manager struct {
	config    common.AlertConfig
	inputChan chan common.DMDResult
	ctx       context.Context
	cancel    context.CancelFunc

	// Telemetry metrics
	anomaliesDetected   telemetry.Counter
	reconstructionError telemetry.SimpleGauge
	normalizedError     telemetry.SimpleGauge
}

// NewManager creates a new alert manager
func NewManager(config common.AlertConfig, inputChan chan common.DMDResult, tel telemetry.Component) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		config:    config,
		inputChan: inputChan,
		ctx:       ctx,
		cancel:    cancel,

		// Initialize telemetry metrics
		anomaliesDetected: tel.NewCounter(
			"logdrift",
			"anomalies_detected_total",
			[]string{"severity"},
			"Total number of anomalies detected by severity",
		),
		reconstructionError: tel.NewSimpleGauge(
			"logdrift",
			"dmd_reconstruction_error",
			"Current DMD reconstruction error",
		),
		normalizedError: tel.NewSimpleGauge(
			"logdrift",
			"dmd_normalized_error",
			"Current DMD normalized error (in standard deviations)",
		),
	}
}

// Start begins processing DMD results
func (m *Manager) Start() {
	go m.run()
}

// Stop stops the alert manager
func (m *Manager) Stop() {
	m.cancel()
}

func (m *Manager) run() {
	for {
		select {
		case <-m.ctx.Done():
			return

		case result, ok := <-m.inputChan:
			if !ok {
				return
			}

			m.processDMDResult(result)
		}
	}
}

func (m *Manager) processDMDResult(result common.DMDResult) {
	// Update metrics
	m.reconstructionError.Set(result.ReconstructionError)
	m.normalizedError.Set(result.NormalizedError)

	// Check thresholds
	var severity common.Severity
	alertTriggered := false

	if result.NormalizedError >= m.config.CriticalThreshold {
		severity = common.SeverityCritical
		alertTriggered = true
	} else if result.NormalizedError >= m.config.WarningThreshold {
		severity = common.SeverityWarning
		alertTriggered = true
	}

	if alertTriggered {
		alert := common.Alert{
			SourceKey:           result.SourceKey,
			Timestamp:           time.Now(),
			WindowID:            result.WindowID,
			ReconstructionError: result.ReconstructionError,
			NormalizedError:     result.NormalizedError,
			Severity:            severity,
			Templates:           result.Templates,
		}

		// Update counter
		m.anomaliesDetected.Inc(string(severity))

		// Log the alert
		m.logAlert(alert)
	}
}

func (m *Manager) logAlert(alert common.Alert) {
	// Create structured log entry
	logEntry := map[string]interface{}{
		"timestamp":            alert.Timestamp.Format(time.RFC3339),
		"source_key":           alert.SourceKey,
		"level":                m.severityToLogLevel(alert.Severity),
		"message":              "Anomaly detected in log stream",
		"window_id":            alert.WindowID,
		"reconstruction_error": alert.ReconstructionError,
		"normalized_error":     alert.NormalizedError,
		"severity":             string(alert.Severity),
		"template_count":       len(alert.Templates),
		"templates":            alert.Templates,
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(logEntry)
	if err != nil {
		log.Errorf("Failed to marshal alert to JSON: %v", err)
		return
	}

	// Log based on severity (include source key in message for easy filtering)
	switch alert.Severity {
	case common.SeverityCritical:
		log.Errorf("DRIFT DETECTION CRITICAL [%s]: %s", alert.SourceKey, string(jsonBytes))
	case common.SeverityWarning:
		log.Warnf("DRIFT DETECTION WARNING [%s]: %s", alert.SourceKey, string(jsonBytes))
	}
}

func (m *Manager) severityToLogLevel(severity common.Severity) string {
	switch severity {
	case common.SeverityCritical:
		return "ERROR"
	case common.SeverityWarning:
		return "WARN"
	default:
		return "INFO"
	}
}
