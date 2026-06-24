// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// helperEventIntegrationID pins the helper events to the edge-intelligence integration,
	// consistent with the reporter's correlation events.
	helperEventIntegrationID = "edge-intelligence"

	// helperEventSourceTag is the distinguishing tag applied to all severity events sent
	// by the helper. It separates them from the reporter's correlation change events
	// (which carry source:edge-intelligence / pattern:<x>).
	helperEventSourceTag = "edge_intelligence_event_type:scorer_helper"

	// helperEventSourceIntegrationTag mirrors the reporter's "source:edge-intelligence"
	// tag so that helper events are findable by the same source filter.
	helperEventSourceIntegrationTag = "source:edge-intelligence"
)

// severityEventSender handles the Datadog v2 change event submission for severity transitions.
// It is self-contained in observer/impl so the observer does not import reporter internals.
type severityEventSender struct {
	forwarder eventplatform.Forwarder
	hostname  hostname.Component
}

// send emits one v2 change event for the given severity transition.
func (s *severityEventSender) send(scorerName string, evt observerdef.SeverityEvent) {
	direction := "escalation"
	if evt.Direction == observerdef.ScorerEventDeescalation {
		direction = "deescalation"
	}
	title := fmt.Sprintf("anomaly severity %s: %s -> %s",
		direction,
		severityLevelName(evt.FromLevel),
		severityLevelName(evt.ToLevel),
	)
	msg := fmt.Sprintf("Anomaly scorer %q severity %s at t=%d: %s → %s",
		scorerName, direction, evt.Timestamp,
		severityLevelName(evt.FromLevel), severityLevelName(evt.ToLevel),
	)
	ts := time.Unix(evt.Timestamp, 0).UTC().Format(time.RFC3339)
	aggKey := "observer:scorer:" + scorerName

	var host string
	if s.hostname != nil {
		host = s.hostname.GetSafe(context.TODO())
	}

	attrs := map[string]any{
		"title":           title,
		"message":         msg,
		"category":        "change",
		"integration_id":  helperEventIntegrationID,
		"tags":            []string{helperEventSourceIntegrationTag, helperEventSourceTag, "scorer:" + scorerName, "direction:" + direction},
		"timestamp":       ts,
		"aggregation_key": aggKey,
		"attributes": map[string]any{
			"changed_resource": map[string]any{
				"name": scorerName,
				"type": "anomaly",
			},
			"author": map[string]any{
				"name": "datadog-agent-observer",
				"type": "automation",
			},
		},
	}
	if host != "" {
		attrs["host"] = host
	}
	payload := map[string]any{
		"data": map[string]any{
			"type":       "event",
			"attributes": attrs,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		pkglog.Warnf("[observer] anomaly_scorer_helper: failed to marshal severity event: %v", err)
		return
	}
	pkglog.Infof("[observer] anomaly_scorer_helper: sending severity event scorer=%s direction=%s aggKey=%s", scorerName, direction, aggKey)
	epMsg := message.NewMessage(body, nil, "", time.Now().UnixNano())
	if err := s.forwarder.SendEventPlatformEventBlocking(epMsg, eventplatform.EventTypeEventManagement); err != nil {
		pkglog.Warnf("[observer] anomaly_scorer_helper: failed to send severity event: %v", err)
	}
}

// anomalyScorerHelper is the built-in ScorerListener registered by the observer
// when anomaly_detection.detectors.anomaly_scorer.helper.enabled is true.
//
// On each OnSeverityTransition call it:
//  1. Logs the transition via pkglog.
//  2. Sets the observer.scorer.state gauge (numeric severity level, tagged by scorer name + direction).
//  3. Optionally sends a Datadog v2 change event (when report_events=true and the forwarder is available).
//
// The helper is subscribed with a zero-value filter so it receives every transition.
type anomalyScorerHelper struct {
	scorerName   string
	stateGauge   telemetry.Gauge
	reportEvents bool
	sender       *severityEventSender // nil unless report_events && forwarder available
}

// newAnomalyScorerHelper creates the helper. sender may be nil if report_events is disabled
// or the forwarder was unavailable at startup.
func newAnomalyScorerHelper(scorerName string, stateGauge telemetry.Gauge, reportEvents bool, sender *severityEventSender) *anomalyScorerHelper {
	return &anomalyScorerHelper{
		scorerName:   scorerName,
		stateGauge:   stateGauge,
		reportEvents: reportEvents,
		sender:       sender,
	}
}

// OnSeverityTransition implements observerdef.ScorerListener.
func (h *anomalyScorerHelper) OnSeverityTransition(evt observerdef.SeverityEvent) {
	direction := "escalation"
	if evt.Direction == observerdef.ScorerEventDeescalation {
		direction = "deescalation"
	}

	pkglog.Infof("[observer] anomaly scorer %s severity %s to %s (was %s, t=%d)",
		h.scorerName,
		direction,
		severityLevelName(evt.ToLevel),
		severityLevelName(evt.FromLevel),
		evt.Timestamp,
	)

	h.stateGauge.Set(float64(evt.ToLevel), "scorer:"+h.scorerName, direction)

	if h.reportEvents && h.sender != nil {
		h.sender.send(h.scorerName, evt)
	}
}

// severityLevelName returns a human-readable label for a SeverityLevel.
func severityLevelName(l observerdef.SeverityLevel) string {
	switch l {
	case observerdef.SeverityLow:
		return "Low"
	case observerdef.SeverityMedium:
		return "Medium"
	case observerdef.SeverityHigh:
		return "High"
	default:
		return fmt.Sprintf("SeverityLevel(%d)", int(l))
	}
}
