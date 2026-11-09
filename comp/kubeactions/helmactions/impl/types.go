// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helmactionsimpl

import (
	"context"
	"fmt"
)

// Action type constants
const (
	ActionTypeUnknown  = "unknown"
	ActionTypeRollback = "rollback"
)

// Execution status constants
const (
	StatusClaimed = "claimed"
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusExpired = "expired"
)

// HelmActionsList is the RC payload envelope – exactly one action per config is enforced.
type HelmActionsList struct {
	Actions []*HelmAction `json:"actions"`
}

// HelmAction is a single helm action received from remote config.
type HelmAction struct {
	ActionID    string        `json:"action_id"`
	RequestedBy string        `json:"requested_by"`
	Timestamp   int64         `json:"timestamp"` // Unix seconds
	Rollback    *HelmRollback `json:"rollback,omitempty"`
}

// HelmRollback holds parameters for a `helm rollback` operation.
type HelmRollback struct {
	Release          string `json:"release"`
	ReleaseNamespace string `json:"release_namespace"`
	Revision         int    `json:"revision,omitempty"`
	JobNamespace     string `json:"job_namespace"`
	ServiceAccount   string `json:"service_account"`
	Image            string `json:"image,omitempty"`
	Driver           string `json:"driver,omitempty"`
}

// ExecutionResult represents the outcome of a helm action.
type ExecutionResult struct {
	Status  string
	Message string
}

// ActionExecutor is the interface all helm action executors must satisfy.
type ActionExecutor interface {
	Execute(ctx context.Context, action *HelmAction) ExecutionResult
}

// GetActionType returns the action type string for a HelmAction.
func GetActionType(action *HelmAction) string {
	if action == nil {
		return ActionTypeUnknown
	}
	if action.Rollback != nil {
		return ActionTypeRollback
	}
	return ActionTypeUnknown
}

// ActionKey uniquely identifies an action by RC metadata ID and version.
type ActionKey struct {
	ID      string
	Version uint64
}

// String returns a human-readable representation of the key.
func (ak ActionKey) String() string {
	return fmt.Sprintf("%s:v%d", ak.ID, ak.Version)
}
