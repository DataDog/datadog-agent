// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package kubeactions

// ActionName constants are the action segment of the FQN
// (com.datadoghq.kubernetes.kubeactions.<name>) and the keys the runner uses to
// dispatch a task to its handler. They are camelCase to match the workflow
// manifest convention shared by the other kubernetes bundles (testConnection,
// restartDeployment, ...) — the backend dispatches by this exact name.
const (
	ActionNameDeletePod          = "deletePod"
	ActionNameRestartDeployment  = "restartDeployment"
	ActionNamePatchDeployment    = "patchDeployment"
	ActionNameRollbackDeployment = "rollbackDeployment"
	ActionNameGetResource        = "getResource"
)

// ActionType constants are the snake_case identifiers emitted in the EVP
// action_type field. They intentionally match the legacy kube-actions intake
// schema (pkg/clusteragent/kubeactions) so downstream storage is unchanged.
const (
	ActionTypeDeletePod          = "delete_pod"
	ActionTypeRestartDeployment  = "restart_deployment"
	ActionTypePatchDeployment    = "patch_deployment"
	ActionTypeRollbackDeployment = "rollback_deployment"
	ActionTypeGetResource        = "get_resource"
)

// Execution status constants.
const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusExpired = "expired"
)

// Event Platform event_type values emitted by the reporter.
const (
	EventTypeActionReceived = "action_received"
	EventTypeActionProgress = "action_progress"
	EventTypeActionExecuted = "action_executed"
)

// ExecutionResult represents the result of executing an action.
type ExecutionResult struct {
	Status   string
	Message  string
	Payloads map[string][]byte
}

// ActionReport carries the per-action metadata needed to emit an Event Platform
// event. Cluster identity (cluster name/ID) is held by the component, so
// handlers only supply the action- and resource-level fields.
type ActionReport struct {
	ActionID          string
	ActionType        string
	OrgID             int64
	RequestedBy       string
	ResourceID        string
	ResourceKind      string
	ResourceName      string
	ResourceNamespace string
}

// ReportFromResource builds an ActionReport from an action type and the common
// resource reference, filling the resource-level fields. Callers set ActionID,
// OrgID and RequestedBy from the task afterwards as needed.
func ReportFromResource(actionType string, r ResourceRef) ActionReport {
	return ActionReport{
		ActionType:        actionType,
		ResourceID:        r.ResourceID,
		ResourceKind:      r.Kind,
		ResourceName:      r.Name,
		ResourceNamespace: r.Namespace,
	}
}
