// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalyhandler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	mcpagent "github.com/DataDog/datadog-agent/comp/mcp/agent"
	mcpconfig "github.com/DataDog/datadog-agent/comp/mcp/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/anomaly"
)

// dependencies defines all components this handler needs
type dependencies struct {
	fx.In
	Lc            fx.Lifecycle
	MCPConfig     mcpconfig.Component
	Logger        log.Component
	AIAgent       mcpagent.Component
	Demultiplexer demultiplexer.Component
}

// provides defines what this component provides
type provides struct {
	fx.Out

	Comp           Component
	StatusProvider status.InformationProvider
}

// anomalyHandler is the internal implementation
// Implements anomaly.AnomalyObserver interface
type anomalyHandler struct {
	config        mcpconfig.Component
	logger        log.Component
	aiAgent       mcpagent.Component
	demultiplexer demultiplexer.Component
	mu            sync.Mutex
	wg            sync.WaitGroup
}

// newAnomalyHandler creates a new anomaly handler
func newAnomalyHandler(deps dependencies) provides {
	mcpConf := deps.MCPConfig.Get()

	deps.Logger.Infof(
		"MCP anomaly handler initialization: enabled=%v, anomaly_detection_enabled=%v",
		mcpConf.Enabled,
		mcpConf.AnomalyDetectionEnabled,
	)

	// Check if MCP and anomaly detection are enabled
	if !mcpConf.Enabled || !mcpConf.AnomalyDetectionEnabled {
		deps.Logger.Info("MCP anomaly handler is disabled")
		return provides{
			Comp: &anomalyHandler{
				config: deps.MCPConfig,
				logger: deps.Logger,
			},
			StatusProvider: status.NewInformationProvider(nil),
		}
	}

	deps.Logger.Info("MCP anomaly handler is ENABLED - creating handler")

	handler := &anomalyHandler{
		config:        deps.MCPConfig,
		logger:        deps.Logger,
		aiAgent:       deps.AIAgent,
		demultiplexer: deps.Demultiplexer,
	}

	// Register lifecycle hooks
	deps.Lc.Append(
		fx.Hook{
			OnStart: func(ctx context.Context) error {
				return handler.Start()
			},
			OnStop: func(ctx context.Context) error {
				return handler.Stop()
			},
		},
	)

	return provides{
		Comp:           handler,
		StatusProvider: status.NewInformationProvider(nil), // Can add status info later
	}
}

// Start starts the anomaly handler and subscribes to anomaly detector
func (h *anomalyHandler) Start() error {
	h.logger.Info("MCP anomaly handler Start() called")

	if h.aiAgent == nil {
		h.logger.Warn("MCP anomaly handler Start(): aiAgent is nil, handler disabled")
		return nil // Handler is disabled
	}

	h.logger.Info("MCP anomaly handler Starting...")

	// Get the anomaly detector from the demultiplexer
	detector := h.demultiplexer.GetAnomalyDetector()
	if detector == nil {
		h.logger.Error("MCP anomaly handler: detector is nil!")
		return nil
	}

	h.logger.Info("MCP anomaly handler: subscribing to detector...")
	detector.Subscribe(h)
	h.logger.Info("MCP anomaly handler: SUCCESSFULLY subscribed to anomaly detector")

	return nil
}

// Stop stops the anomaly handler and unsubscribes from detector
func (h *anomalyHandler) Stop() error {
	if h.aiAgent == nil {
		return nil // Handler is disabled
	}

	h.logger.Info("Stopping MCP anomaly handler")

	// Unsubscribe from anomaly detector
	detector := h.demultiplexer.GetAnomalyDetector()
	if detector != nil {
		detector.Unsubscribe(h)
		h.logger.Info("MCP anomaly handler unsubscribed from anomaly detector")
	}

	h.wg.Wait()

	return nil
}

// OnAnomaly is called when an anomaly is detected
// Implements the anomaly.AnomalyObserver interface
func (h *anomalyHandler) OnAnomaly(anom anomaly.Anomaly) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.logger.Warnf(
		"[MCP HANDLER] Anomaly detected: %s (type: %s, value: %.2f, baseline: %.2f, severity: %.2f)",
		anom.MetricName,
		anom.Type,
		anom.Value,
		anom.Baseline,
		anom.Severity,
	)

	// Convert anomaly to an issue for the AI agent
	issue := mcpagent.Issue{
		Description: fmt.Sprintf(
			"Anomaly detected on metric '%s': %s detected with value %.2f (baseline: %.2f)",
			anom.MetricName,
			anom.Type,
			anom.Value,
			anom.Baseline,
		),
		Severity: h.determineSeverity(anom.Severity),
		Metadata: map[string]interface{}{
			"metric_name":  anom.MetricName,
			"anomaly_type": string(anom.Type),
			"value":        anom.Value,
			"baseline":     anom.Baseline,
			"severity":     anom.Severity,
			"timestamp":    anom.Timestamp,
		},
	}

	// Launch AI agent in a goroutine to avoid blocking
	h.wg.Add(1)
	go h.solveAnomaly(issue)
}

// solveAnomaly uses the AI agent to investigate and resolve the anomaly
func (h *anomalyHandler) solveAnomaly(issue mcpagent.Issue) {
	defer h.wg.Done()

	h.logger.Infof(
		"Launching AI agent to solve anomaly: %s",
		issue.Description,
	)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Minute,
	)
	defer cancel()

	// Call the AI agent to solve the issue
	result, err := h.aiAgent.Solve(
		ctx,
		issue,
	)
	if err != nil {
		h.logger.Errorf(
			"AI agent failed to solve anomaly: %v",
			err,
		)
		return
	}

	// Log the results
	if result.Success {
		h.logger.Infof(
			"AI agent successfully resolved anomaly: %s",
			result.FinalState,
		)
	} else {
		h.logger.Warnf(
			"AI agent could not resolve anomaly: %s",
			result.FinalState,
		)
	}

	// Log the agent's steps
	h.logger.Debugf(
		"AI agent took %d steps:",
		len(result.Steps),
	)
	for i, step := range result.Steps {
		h.logger.Debugf(
			"  Step %d [%s]: %s",
			i+1,
			step.Type,
			step.Content,
		)
	}
}

// determineSeverity converts numerical severity to string severity
func (h *anomalyHandler) determineSeverity(severity float64) string {
	switch {
	case severity >= 0.8:
		return "critical"
	case severity >= 0.6:
		return "high"
	case severity >= 0.4:
		return "medium"
	default:
		return "low"
	}
}
