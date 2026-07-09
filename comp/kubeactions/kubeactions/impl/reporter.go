// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeactionsimpl

import (
	"encoding/json"
	"time"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ActionResultEvent is the EVP event payload sent to the backend. The backend
// stores the entire serialized payload as the `data` jsonb column and extracts
// the top-level fields into their respective DB columns.
type ActionResultEvent struct {
	ActionID          string            `json:"action_id"`
	OrgID             int64             `json:"org_id"`
	EventType         string            `json:"event_type"`
	Status            string            `json:"status"`
	ActionType        string            `json:"action_type"`
	ClusterID         string            `json:"cluster_id"`
	ResourceID        string            `json:"resource_id"`
	RequestedBy       string            `json:"requested_by"`
	Timestamp         string            `json:"timestamp"`
	Message           string            `json:"message"`
	Payloads          map[string][]byte `json:"payloads,omitempty"`
	ClusterName       string            `json:"cluster_name"`
	ResourceKind      string            `json:"resource_kind,omitempty"`
	ResourceName      string            `json:"resource_name,omitempty"`
	ResourceNamespace string            `json:"resource_namespace,omitempty"`
}

// resultReporter reports Kubernetes action lifecycle events to the backend via
// the Event Platform. It is lifted from the remote-config implementation so PAR
// handlers can emit received / progress / executed events in a single call.
type resultReporter struct {
	epForwarder eventplatform.Forwarder
	clusterName string
	clusterID   string
}

// newResultReporter creates a resultReporter. epForwarder may be nil, in which
// case reporting is skipped (and logged).
func newResultReporter(epForwarder eventplatform.Forwarder, clusterName, clusterID string) *resultReporter {
	return &resultReporter{
		epForwarder: epForwarder,
		clusterName: clusterName,
		clusterID:   clusterID,
	}
}

// ReportReceived sends an action_received event when a handler begins processing.
func (r *resultReporter) ReportReceived(report kubeactions.ActionReport) {
	r.report(report, kubeactions.EventTypeActionReceived, kubeactions.StatusSuccess, "action received", nil, time.Now())
}

// ReportProgress sends an intermediate action_progress event.
func (r *resultReporter) ReportProgress(report kubeactions.ActionReport, msg string) {
	r.report(report, kubeactions.EventTypeActionProgress, "in_progress", msg, nil, time.Now())
}

// ReportResult sends the terminal action_executed event.
func (r *resultReporter) ReportResult(report kubeactions.ActionReport, result kubeactions.ExecutionResult) {
	r.report(report, kubeactions.EventTypeActionExecuted, result.Status, result.Message, result.Payloads, time.Now())
}

func (r *resultReporter) report(report kubeactions.ActionReport, evpEventType, status, msg string, payloads map[string][]byte, ts time.Time) {
	if r.epForwarder == nil {
		log.Warnf("[KubeActions] Event Platform forwarder not available, skipping %s reporting for action %s", evpEventType, report.ActionID)
		return
	}

	event := ActionResultEvent{
		ActionID:          report.ActionID,
		OrgID:             report.OrgID,
		EventType:         evpEventType,
		Status:            status,
		ActionType:        report.ActionType,
		ClusterID:         r.clusterID,
		ResourceID:        report.ResourceID,
		RequestedBy:       report.RequestedBy,
		Timestamp:         ts.Format(time.RFC3339),
		Message:           msg,
		Payloads:          payloads,
		ClusterName:       r.clusterName,
		ResourceKind:      report.ResourceKind,
		ResourceName:      report.ResourceName,
		ResourceNamespace: report.ResourceNamespace,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		log.Errorf("[KubeActions] Failed to marshal %s event for %s: %v", evpEventType, report.ActionID, err)
		return
	}

	evpMsg := message.NewMessage(payload, nil, "", 0)
	if err := r.epForwarder.SendEventPlatformEventBlocking(evpMsg, eventplatform.EventTypeKubeActions); err != nil {
		log.Errorf("[KubeActions] Failed to send %s event to Event Platform: %v", evpEventType, err)
		return
	}

	log.Debugf("[KubeActions] Sent %s event for action %s", evpEventType, report.ActionID)
}
