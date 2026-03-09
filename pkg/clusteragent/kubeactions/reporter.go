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

// ActionResultEvent is the top-level EVP event for a kubeactions execution result.
// Common fields live at the top level. Action-specific result details go under
// a key matching the action type (e.g. "delete_pod", "restart_deployment"),
// mirroring the oneof pattern in the RC schema.
type ActionResultEvent struct {
	// Common fields
	ActionID   string          `json:"action_id"`
	ActionType string          `json:"action_type"`
	Status     string          `json:"status"`
	Message    string          `json:"message,omitempty"`
	ExecutedAt string          `json:"executed_at"`
	Resource   *EventResource  `json:"resource"`
	Cluster    *EventCluster   `json:"cluster"`

	// Action-specific result details (exactly one should be set, matching action_type)
	DeletePod            *DeletePodResult            `json:"delete_pod,omitempty"`
	RestartDeployment    *RestartDeploymentResult    `json:"restart_deployment,omitempty"`
}

// EventResource identifies the Kubernetes resource acted on
type EventResource struct {
	APIVersion string `json:"api_version,omitempty"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
	ResourceID string `json:"resource_id"`
}

// EventCluster identifies the cluster where the action was executed
type EventCluster struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// DeletePodResult holds action-specific result details for delete_pod
type DeletePodResult struct {
	GracePeriodSeconds *int64 `json:"grace_period_seconds,omitempty"`
}

// RestartDeploymentResult holds action-specific result details for restart_deployment
type RestartDeploymentResult struct{}

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

	// Use action_id from payload, fall back to RC metadata ID
	actionID := action.GetActionId()
	if actionID == "" {
		actionID = actionKey.ID
	}

	// Build the common event
	event := ActionResultEvent{
		ActionID:   actionID,
		ActionType: actionType,
		Status:     result.Status,
		Message:    result.Message,
		ExecutedAt: executedAt.Format(time.RFC3339),
		Cluster: &EventCluster{
			Name: r.clusterName,
			ID:   r.clusterID,
		},
	}

	// Build resource from action
	if res := action.GetResource(); res != nil {
		event.Resource = &EventResource{
			APIVersion: res.GetApiVersion(),
			Kind:       res.GetKind(),
			Namespace:  res.GetNamespace(),
			Name:       res.GetName(),
			ResourceID: res.GetResourceId(),
		}
	}

	// Build action-specific result details
	switch actionType {
	case ActionTypeDeletePod:
		dpResult := &DeletePodResult{}
		if params := action.GetDeletePod(); params != nil && params.GracePeriodSeconds != nil {
			dpResult.GracePeriodSeconds = params.GracePeriodSeconds
		}
		event.DeletePod = dpResult
	case ActionTypeRestartDeployment:
		event.RestartDeployment = &RestartDeploymentResult{}
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Errorf("[KubeActions] Event Platform returned status %d: %s", resp.StatusCode, string(respBody))
		return
	}

	log.Infof("[KubeActions] Reported result for action %s (status=%s)", actionID, result.Status)
}
