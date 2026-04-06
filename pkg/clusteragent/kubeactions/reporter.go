// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// --- Event payload types ---

// ActionResultEvent is the EVP event payload sent to the backend when an action is executed.
// Fields match the backend's UpdateActionRequest / KubeActionEvent schema.
type ActionResultEvent struct {
	ActionID    string                 `json:"action_id"`
	OrgID       int64                  `json:"org_id"`
	EventType   string                 `json:"event_type"`
	Status      string                 `json:"status"`
	ActionType  string                 `json:"action_type"`
	ClusterID   string                 `json:"cluster_id"`
	ResourceID  string                 `json:"resource_id"`
	RequestedBy string                 `json:"requested_by"`
	Timestamp   string                 `json:"timestamp"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// --- Reporter ---

// ResultReporter handles reporting action execution results back to the backend via Event Platform
type ResultReporter struct {
	epForwarder    eventplatform.Forwarder
	httpClient     *http.Client
	intakeEndpoint string
	orgID          int64
	clusterName    string
	clusterID      string
}

// NewResultReporter creates a new ResultReporter with the given Event Platform forwarder
func NewResultReporter(epForwarder eventplatform.Forwarder, clusterName, clusterID string) *ResultReporter {
	// TODO(KUBEACTIONS-POC): Using direct HTTP POST to internal staging intake
	// For production, this should use the epForwarder component
	return &ResultReporter{
		epForwarder: epForwarder,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		intakeEndpoint: "http://all-internal-intake-logs.staging.dog",
		orgID:          2, // Staging org
		clusterName:    clusterName,
		clusterID:      clusterID,
	}
}

// ReportResult reports an action execution result via Event Platform
func (r *ResultReporter) ReportResult(actionKey ActionKey, action *kubeactions.KubeAction, result ExecutionResult, executedAt time.Time) {
	actionType := GetActionType(action)
	if actionType == ActionTypeUnknown {
		log.Warnf("[KubeActions] Reporting result for unknown action type, action_key=%s", actionKey.String())
	}

	// Use action_id from payload, fall back to RC metadata ID
	actionID := action.GetActionId()
	if actionID == "" {
		actionID = actionKey.ID
	}

	// Build resource_id from the action's resource
	var resourceID string
	if res := action.GetResource(); res != nil {
		resourceID = res.GetResourceId()
	}

	// Build the data map with action-specific details and execution context
	data := map[string]interface{}{
		"message":      result.Message,
		"cluster_name": r.clusterName,
	}
	if res := action.GetResource(); res != nil {
		data["resource_kind"] = res.GetKind()
		data["resource_name"] = res.GetName()
		data["resource_namespace"] = res.GetNamespace()
		data["resource_api_version"] = res.GetApiVersion()
	}
	switch actionType {
	case ActionTypeDeletePod:
		if params := action.GetDeletePod(); params != nil && params.GracePeriodSeconds != nil {
			data["grace_period_seconds"] = *params.GracePeriodSeconds
		}
	}

	event := ActionResultEvent{
		ActionID:    actionID,
		OrgID:       r.orgID,
		EventType:   "action_executed",
		Status:      result.Status,
		ActionType:  actionType,
		ClusterID:   r.clusterID,
		ResourceID:  resourceID,
		RequestedBy: action.GetRequestedBy(),
		Timestamp:   executedAt.Format(time.RFC3339),
		Data:        data,
	}

	log.Infof("[KubeActions] Reporting result: action_id=%s, type=%s, status=%s", event.ActionID, event.ActionType, event.Status)

	// Serialize to JSON
	payload, err := json.Marshal(event)
	if err != nil {
		log.Errorf("[KubeActions] Failed to marshal action result for %s: %v", actionKey.String(), err)
		return
	}

	log.Debugf("[KubeActions] EVP payload: %s", string(payload))

	// TODO(KUBEACTIONS-POC): Direct HTTP POST to staging intake
	// For production, use: r.epForwarder.SendEventPlatformEvent(...)
	url := fmt.Sprintf("%s/v2/track/demoalpha/org/%d", r.intakeEndpoint, r.orgID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		log.Errorf("[KubeActions] Failed to create HTTP request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-EVP-ORIGIN", "kubernetes-actions")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		log.Errorf("[KubeActions] Failed to send result to Event Platform: %v", err)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("[KubeActions] Failed to read response body: %v", err)
		return
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		log.Errorf("[KubeActions] Event Platform returned status %d: %s", resp.StatusCode, string(respBody))
		return
	}

	log.Infof("[KubeActions] Reported result for action %s (status=%s)", actionID, result.Status)
}
