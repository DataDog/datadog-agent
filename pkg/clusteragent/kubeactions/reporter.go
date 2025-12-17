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

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ResultReporter handles reporting action execution results back to the backend via Event Platform
type ResultReporter struct {
	epForwarder    eventplatform.Forwarder
	httpClient     *http.Client
	intakeEndpoint string
	orgID          int64
}

// NewResultReporter creates a new ResultReporter with the given Event Platform forwarder
func NewResultReporter(epForwarder eventplatform.Forwarder) *ResultReporter {
	// TODO(KUBEACTIONS-POC): Using direct HTTP POST to internal staging intake
	// For production, this should use the proper EVP endpoint or the epForwarder
	return &ResultReporter{
		epForwarder: epForwarder,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		intakeEndpoint: "http://all-internal-intake-logs.staging.dog",
		orgID:          2, // Staging org
	}
}

// ActionEventPayload represents the EVP event payload format (matches service format)
type ActionEventPayload struct {
	ActionID    string            `json:"action_id"`
	ActionType  string            `json:"action_type"`
	Version     int64             `json:"version"`
	Status      string            `json:"status"`
	Message     string            `json:"message,omitempty"`
	ExecutedAt  string            `json:"executed_at"`
	Service     string            `json:"service"`
	DDTags      string            `json:"ddtags"`
	Project     string            `json:"project"`
	Resource    *ResourcePayload  `json:"resource"`
	Parameters  map[string]string `json:"parameters,omitempty"`
	Target      *TargetPayload    `json:"target,omitempty"`
}

// ResourcePayload represents a Kubernetes resource in the event
type ResourcePayload struct {
	APIVersion string `json:"api_version"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
}

// TargetPayload represents the target information in the event
type TargetPayload struct {
	Hostname    string   `json:"hostname,omitempty"`
	ClusterName string   `json:"cluster_name,omitempty"`
	ClusterID   string   `json:"cluster_id,omitempty"`
	TeamTags    []string `json:"team_tags,omitempty"`
}

// ReportResult reports an action execution result via Event Platform
// TODO(KUBEACTIONS-POC): Using direct HTTP POST to match service implementation
// This sends a JSON event directly to the demoalpha track, same as the service
func (r *ResultReporter) ReportResult(actionKey ActionKey, action interface{}, result ExecutionResult, executedAt time.Time, hostname string) {
	log.Infof("[KubeActions] ReportResult called for action %s with status %s", actionKey.String(), result.Status)

	// Try to extract KubeAction details if available
	var actionType string
	var resource *ResourcePayload
	var parameters map[string]string

	// Type assert to get the actual action
	if kubeAction, ok := action.(interface {
		GetActionType() string
		GetResource() interface{ GetKind() string; GetName() string; GetNamespace() string; GetApiVersion() string }
		GetParameters() map[string]string
	}); ok {
		actionType = kubeAction.GetActionType()
		res := kubeAction.GetResource()
		resource = &ResourcePayload{
			APIVersion: res.GetApiVersion(),
			Kind:       res.GetKind(),
			Namespace:  res.GetNamespace(),
			Name:       res.GetName(),
		}
		parameters = kubeAction.GetParameters()
	}

	// Create the event payload matching the service format
	event := ActionEventPayload{
		ActionID:   actionKey.ID,
		ActionType: actionType,
		Version:    int64(actionKey.Version),
		Status:     result.Status,
		Message:    result.Message,
		ExecutedAt: executedAt.Format(time.RFC3339),
		Service:    "kubernetes-actions",
		DDTags:     "kubernetes-actions,demoalpha",
		Project:    "kubernetes-actions-poc",
		Resource:   resource,
		Parameters: parameters,
		Target: &TargetPayload{
			Hostname: hostname,
		},
	}

	log.Infof("[KubeActions] Created event payload: action_id=%s, version=%d, status=%s, executed_at=%s",
		event.ActionID, event.Version, event.Status, event.ExecutedAt)

	// Serialize to JSON
	payload, err := json.Marshal(event)
	if err != nil {
		log.Errorf("[KubeActions] Failed to marshal action result for %s: %v", actionKey.String(), err)
		return
	}

	log.Infof("[KubeActions] Full EVP JSON payload: %s", string(payload))

	// Build the intake URL using demoalpha track (same as service)
	url := fmt.Sprintf("%s/v2/track/demoalpha/org/%d", r.intakeEndpoint, r.orgID)
	log.Infof("[KubeActions] Sending status update to: %s", url)

	// Create HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		log.Errorf("[KubeActions] Failed to create HTTP request: %v", err)
		return
	}

	// Set headers (matching service implementation)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-EVP-ORIGIN", "kubernetes-actions")

	log.Infof("[KubeActions] Sending HTTP POST request...")

	// Send the request
	resp, err := r.httpClient.Do(req)
	if err != nil {
		log.Errorf("[KubeActions] ERROR: Failed to send HTTP request to Event Platform: %v", err)
		return
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("[KubeActions] Failed to read response body: %v", err)
		return
	}

	// Check status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Errorf("[KubeActions] ERROR: Event Platform returned non-2xx status: %d, body: %s", resp.StatusCode, string(respBody))
		return
	}

	log.Infof("[KubeActions] SUCCESS: Sent status update to Event Platform for action %s (action_id=%s, status=%s, http_status=%d)",
		actionKey.String(), actionKey.ID, result.Status, resp.StatusCode)
}
