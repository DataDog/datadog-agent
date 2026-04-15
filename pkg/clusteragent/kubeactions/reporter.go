// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"encoding/json"
	"time"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EVP event_type values
const (
	EventTypeActionReceived = "action_received"
	EventTypeActionExecuted = "action_executed"
)

// ActionResultEvent is the EVP event payload sent to the backend.
// The backend stores the entire serialized payload as the `data` jsonb column
// and extracts the top-level fields into their respective DB columns.
type ActionResultEvent struct {
	ActionID          string `json:"action_id"`
	OrgID             int64  `json:"org_id"`
	EventType         string `json:"event_type"`
	Status            string `json:"status"`
	ActionType        string `json:"action_type"`
	ClusterID         string `json:"cluster_id"`
	ResourceID        string `json:"resource_id"`
	RequestedBy       string `json:"requested_by"`
	Timestamp         string `json:"timestamp"`
	Message           string `json:"message"`
	ClusterName       string `json:"cluster_name"`
	ResourceKind      string `json:"resource_kind,omitempty"`
	ResourceName      string `json:"resource_name,omitempty"`
	ResourceNamespace string `json:"resource_namespace,omitempty"`
}

// ResultReporter handles reporting action execution results back to the backend via Event Platform
type ResultReporter struct {
	epForwarder eventplatform.Forwarder
	clusterName string
	clusterID   string
}

// NewResultReporter creates a new ResultReporter with the given Event Platform forwarder
func NewResultReporter(epForwarder eventplatform.Forwarder, clusterName, clusterID string) *ResultReporter {
	return &ResultReporter{
		epForwarder: epForwarder,
		clusterName: clusterName,
		clusterID:   clusterID,
	}
}

// ReportReceived sends an action_received event via EVP when an action is first received from RC
func (r *ResultReporter) ReportReceived(actionKey ActionKey, action *kubeactions.KubeAction, orgID int64) {
	r.report(actionKey, action, orgID, EventTypeActionReceived, StatusSuccess, "action received", time.Now())
}

// ReportResult sends an action_executed event via EVP after execution completes
func (r *ResultReporter) ReportResult(actionKey ActionKey, action *kubeactions.KubeAction, result ExecutionResult, orgID int64, executedAt time.Time) {
	r.report(actionKey, action, orgID, EventTypeActionExecuted, result.Status, result.Message, executedAt)
}

func (r *ResultReporter) report(actionKey ActionKey, action *kubeactions.KubeAction, orgID int64, evpEventType, status, msg string, ts time.Time) {
	if r.epForwarder == nil {
		log.Warnf("[KubeActions] Event Platform forwarder not available, skipping %s reporting for action %s", evpEventType, actionKey.String())
		return
	}

	actionType := GetActionType(action)

	// Use action_id from payload, fall back to RC metadata ID
	actionID := action.GetActionId()
	if actionID == "" {
		actionID = actionKey.ID
	}

	var resourceID, resourceKind, resourceName, resourceNamespace string
	if res := action.GetResource(); res != nil {
		resourceID = res.GetResourceId()
		resourceKind = res.GetKind()
		resourceName = res.GetName()
		resourceNamespace = res.GetNamespace()
	}

	event := ActionResultEvent{
		ActionID:          actionID,
		OrgID:             orgID,
		EventType:         evpEventType,
		Status:            status,
		ActionType:        actionType,
		ClusterID:         r.clusterID,
		ResourceID:        resourceID,
		RequestedBy:       action.GetRequestedBy(),
		Timestamp:         ts.Format(time.RFC3339),
		Message:           msg,
		ClusterName:       r.clusterName,
		ResourceKind:      resourceKind,
		ResourceName:      resourceName,
		ResourceNamespace: resourceNamespace,
	}

	log.Infof("[KubeActions] Sending EVP event: event_type=%s, action_id=%s, status=%s", evpEventType, event.ActionID, event.Status)

	payload, err := json.Marshal(event)
	if err != nil {
		log.Errorf("[KubeActions] Failed to marshal %s event for %s: %v", evpEventType, actionKey.String(), err)
		return
	}

	log.Debugf("[KubeActions] EVP payload: %s", string(payload))

	evpMsg := message.NewMessage(payload, nil, "", 0)
	if err := r.epForwarder.SendEventPlatformEventBlocking(evpMsg, eventplatform.EventTypeKubeActions); err != nil {
		log.Errorf("[KubeActions] Failed to send %s event to Event Platform: %v", evpEventType, err)
		return
	}

	log.Infof("[KubeActions] Sent %s event for action %s", evpEventType, actionID)
}
