// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"context"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
)

// Action type constants
const (
	ActionTypeUnknown            = "unknown"
	ActionTypeDeletePod          = "delete_pod"
	ActionTypeRestartDeployment  = "restart_deployment"
	ActionTypePatchDeployment    = "patch_deployment"
	ActionTypeRollbackDeployment = "rollback_deployment"
)

// Execution status constants
const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
	StatusExpired = "expired"
)

// ExecutionResult represents the result of executing an action
type ExecutionResult struct {
	Status  string
	Message string
}

// ActionExecutor is the interface that all action executors must implement
type ActionExecutor interface {
	// Execute performs the action and returns the result
	Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult
}

// GetActionType extracts the action type string from a KubeAction's oneof field.
// Returns ActionTypeUnknown if no action type is set.
func GetActionType(action *kubeactions.KubeAction) string {
	if action == nil {
		return ActionTypeUnknown
	}

	switch action.GetAction().(type) {
	case *kubeactions.KubeAction_DeletePod:
		return ActionTypeDeletePod
	case *kubeactions.KubeAction_RestartDeployment:
		return ActionTypeRestartDeployment
	case *kubeactions.KubeAction_PatchDeployment:
		return ActionTypePatchDeployment
	case *kubeactions.KubeAction_RollbackDeployment:
		return ActionTypeRollbackDeployment
	default:
		return ActionTypeUnknown
	}
}
