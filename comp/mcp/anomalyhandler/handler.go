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
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
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
	ctx           context.Context
	cancel        context.CancelFunc
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

	// Create a cancellable context for this component's lifetime
	h.ctx, h.cancel = context.WithCancel(context.Background())

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

	// Cancel the context to stop any ongoing AI investigations
	if h.cancel != nil {
		h.cancel()
	}

	// Unsubscribe from anomaly detector
	detector := h.demultiplexer.GetAnomalyDetector()
	if detector != nil {
		detector.Unsubscribe(h)
		h.logger.Info("MCP anomaly handler unsubscribed from anomaly detector")
	}

	// Wait for all ongoing investigations to complete
	h.wg.Wait()

	return nil
}

// OnAnomaly is called when an anomaly is detected
// Implements the anomaly.AnomalyObserver interface
func (h *anomalyHandler) OnAnomaly(anom anomaly.Anomaly) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Generate conversation ID for tracking this anomaly investigation
	conversationID := mcpagent.GenerateConversationID()

	h.logger.Warnf(
		"[MCP Anomaly Handler][%s] Anomaly detected: %s (type: %s, value: %.2f, baseline: %.2f, severity: %.2f)",
		conversationID,
		anom.MetricName,
		anom.Type,
		anom.Value,
		anom.Baseline,
		anom.Severity,
	)

	// Submit Datadog event for anomaly detection
	sender, err := h.demultiplexer.GetDefaultSender()
	if err != nil {
		h.logger.Errorf(
			"[MCP Anomaly Handler][%s] Failed to get sender: %v",
			conversationID,
			err,
		)
	} else {
		alertType := h.getAlertType(anom.Severity)

		ev := event.Event{
			Title: fmt.Sprintf(
				"Anomaly Detected: %s",
				anom.MetricName,
			),
			Text: fmt.Sprintf(
				"Anomaly type: %s\nValue: %.2f\nBaseline: %.2f\nSeverity: %s (%.2f)\nConversation ID: %s",
				anom.Type,
				anom.Value,
				anom.Baseline,
				h.determineSeverity(anom.Severity),
				anom.Severity,
				conversationID,
			),
			AlertType:      alertType,
			Priority:       event.PriorityNormal,
			SourceTypeName: "mcp_anomaly_handler",
			AggregationKey: fmt.Sprintf(
				"mcp_anomaly:%s",
				anom.MetricName,
			),
			Tags: []string{
				fmt.Sprintf(
					"metric_name:%s",
					anom.MetricName,
				),
				fmt.Sprintf(
					"anomaly_type:%s",
					string(anom.Type),
				),
				fmt.Sprintf(
					"severity:%s",
					h.determineSeverity(anom.Severity),
				),
				fmt.Sprintf(
					"conversation_id:%s",
					conversationID,
				),
				"component:mcp",
			},
		}

		sender.Event(ev)
	}

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
			"metric_name":     anom.MetricName,
			"anomaly_type":    string(anom.Type),
			"value":           anom.Value,
			"baseline":        anom.Baseline,
			"severity":        anom.Severity,
			"timestamp":       anom.Timestamp,
			"conversation_id": conversationID,
		},
	}

	// Launch AI agent in a goroutine to avoid blocking
	h.wg.Add(1)
	go h.solveAnomaly(
		issue,
		conversationID,
	)
}

// solveAnomaly uses the AI agent to investigate and resolve the anomaly
func (h *anomalyHandler) solveAnomaly(
	issue mcpagent.Issue,
	conversationID string,
) {
	defer h.wg.Done()

	h.logger.Infof(
		"[MCP Anomaly Handler][%s] Launching AI agent to solve anomaly: %s",
		conversationID,
		issue.Description,
	)

	// Create a context with timeout, using the handler's context as parent
	// This ensures the investigation stops when the agent shuts down
	ctx, cancel := context.WithTimeout(
		h.ctx,
		30*time.Minute,
	)
	defer cancel()

	// Call the AI agent to solve the issue
	result, err := h.aiAgent.Solve(
		ctx,
		issue,
	)
	if err != nil {
		h.logger.Errorf(
			"[MCP Anomaly Handler][%s] AI agent failed to solve anomaly: %v",
			conversationID,
			err,
		)

		// Submit event for agent failure
		sender, senderErr := h.demultiplexer.GetDefaultSender()
		if senderErr != nil {
			h.logger.Errorf(
				"[MCP Anomaly Handler][%s] Failed to get sender for error event: %v",
				conversationID,
				senderErr,
			)
		} else {
			errorEvent := event.Event{
				Title: fmt.Sprintf(
					"MCP AI Agent Failed: %s",
					issue.Metadata["metric_name"],
				),
				Text: fmt.Sprintf(
					"Issue: %s\n\nError: %v\nConversation ID: %s",
					issue.Description,
					err,
					conversationID,
				),
				AlertType:      event.AlertTypeError,
				Priority:       event.PriorityNormal,
				SourceTypeName: "mcp_ai_agent",
				AggregationKey: fmt.Sprintf(
					"mcp_agent_error:%s",
					issue.Metadata["metric_name"],
				),
				Tags: []string{
					fmt.Sprintf(
						"metric_name:%s",
						issue.Metadata["metric_name"],
					),
					fmt.Sprintf(
						"conversation_id:%s",
						conversationID,
					),
					"component:mcp",
					"status:error",
				},
			}
			sender.Event(errorEvent)
		}

		return
	}

	// Log the results
	if result.Success {
		h.logger.Infof(
			"[MCP Anomaly Handler][%s] AI agent successfully resolved anomaly: %s",
			conversationID,
			result.FinalState,
		)
	} else {
		h.logger.Warnf(
			"[MCP Anomaly Handler][%s] AI agent could not resolve anomaly: %s",
			conversationID,
			result.FinalState,
		)
	}

	// Submit Datadog event for AI agent resolution
	sender, senderErr := h.demultiplexer.GetDefaultSender()
	if senderErr != nil {
		h.logger.Errorf(
			"[MCP Anomaly Handler][%s] Failed to get sender for resolution event: %v",
			conversationID,
			senderErr,
		)
	} else {
		var resolutionEvent event.Event

		if result.Success {
			resolutionEvent = event.Event{
				Title: fmt.Sprintf(
					"MCP AI Agent Resolved: %s",
					issue.Metadata["metric_name"],
				),
				Text: fmt.Sprintf(
					"Issue: %s\n\nResolution: %s\n\nSteps taken: %d\nConversation ID: %s",
					issue.Description,
					result.FinalState,
					result.Iterations,
					conversationID,
				),
				AlertType:      event.AlertTypeSuccess,
				Priority:       event.PriorityNormal,
				SourceTypeName: "mcp_ai_agent",
				AggregationKey: fmt.Sprintf(
					"mcp_resolution:%s",
					issue.Metadata["metric_name"],
				),
				Tags: []string{
					fmt.Sprintf(
						"metric_name:%s",
						issue.Metadata["metric_name"],
					),
					fmt.Sprintf(
						"conversation_id:%s",
						conversationID,
					),
					"component:mcp",
					"status:resolved",
				},
			}
		} else {
			resolutionEvent = event.Event{
				Title: fmt.Sprintf(
					"MCP AI Agent Could Not Resolve: %s",
					issue.Metadata["metric_name"],
				),
				Text: fmt.Sprintf(
					"Issue: %s\n\nFinal state: %s\n\nSteps attempted: %d\nConversation ID: %s",
					issue.Description,
					result.FinalState,
					result.Iterations,
					conversationID,
				),
				AlertType:      event.AlertTypeWarning,
				Priority:       event.PriorityNormal,
				SourceTypeName: "mcp_ai_agent",
				AggregationKey: fmt.Sprintf(
					"mcp_resolution:%s",
					issue.Metadata["metric_name"],
				),
				Tags: []string{
					fmt.Sprintf(
						"metric_name:%s",
						issue.Metadata["metric_name"],
					),
					fmt.Sprintf(
						"conversation_id:%s",
						conversationID,
					),
					"component:mcp",
					"status:unresolved",
				},
			}
		}

		sender.Event(resolutionEvent)
	}

	// Log the agent's steps
	h.logger.Debugf(
		"[MCP Anomaly Handler][%s] AI agent completed %d iterations with %d total steps (think/act/observe):",
		conversationID,
		result.Iterations,
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

// getAlertType maps numeric severity to Datadog AlertType
func (h *anomalyHandler) getAlertType(severity float64) event.AlertType {
	switch {
	case severity >= 0.6: // critical/high
		return event.AlertTypeError
	case severity >= 0.4: // medium
		return event.AlertTypeWarning
	default: // low
		return event.AlertTypeInfo
	}
}
