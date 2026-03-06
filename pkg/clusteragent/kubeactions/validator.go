// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"fmt"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
)

// ValidationError represents an error during action validation
type ValidationError struct {
	Action  *kubeactions.KubeAction
	Message string
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	actionType := GetActionType(e.Action)
	resourceKind := ""
	resourceName := ""
	if e.Action != nil && e.Action.Resource != nil {
		resourceKind = e.Action.Resource.Kind
		resourceName = e.Action.Resource.Name
	}
	return fmt.Sprintf("validation error for action %s on %s/%s: %s",
		actionType,
		resourceKind,
		resourceName,
		e.Message)
}

// ActionValidator validates actions before execution
type ActionValidator struct {
	// Add configuration fields here as needed
	// For example: allowedActions, allowedNamespaces, etc.
}

// NewActionValidator creates a new ActionValidator
func NewActionValidator() *ActionValidator {
	return &ActionValidator{}
}

// ValidateAction validates a single action before execution
// This is the main hook where we can add safety checks
func (v *ActionValidator) ValidateAction(action *kubeactions.KubeAction) error {
	if action == nil {
		return &ValidationError{
			Action:  action,
			Message: "action is nil",
		}
	}

	// Validate action type is provided (one of the oneof fields must be set)
	actionType := GetActionType(action)
	if actionType == ActionTypeUnknown {
		return &ValidationError{
			Action:  action,
			Message: "action type is required (must specify delete_pod, restart_deployment, etc.)",
		}
	}

	// Validate resource is provided
	if action.Resource == nil {
		return &ValidationError{
			Action:  action,
			Message: "resource is required",
		}
	}

	// Validate resource has required fields
	if action.Resource.Kind == "" {
		return &ValidationError{
			Action:  action,
			Message: "resource.kind is required",
		}
	}

	if action.Resource.Name == "" {
		return &ValidationError{
			Action:  action,
			Message: "resource.name is required",
		}
	}

	// Action-specific validation
	switch actionType {
	case ActionTypeDeletePod:
		// Validate delete_pod specific requirements
		if action.Resource.Namespace == "" {
			return &ValidationError{
				Action:  action,
				Message: "resource.namespace is required for delete_pod action",
			}
		}
		if action.Resource.Kind != "Pod" {
			return &ValidationError{
				Action:  action,
				Message: "resource.kind must be 'Pod' for delete_pod action",
			}
		}
	case ActionTypeRestartDeployment:
		// Validate restart_deployment specific requirements
		if action.Resource.Namespace == "" {
			return &ValidationError{
				Action:  action,
				Message: "resource.namespace is required for restart_deployment action",
			}
		}
		if action.Resource.Kind != "Deployment" {
			return &ValidationError{
				Action:  action,
				Message: "resource.kind must be 'Deployment' for restart_deployment action",
			}
		}
	}

	// TODO: Add more validation logic here as needed:
	// - Check if namespace is allowed
	// - Rate limiting checks
	// - Time-based restrictions
	// - Custom business logic

	return nil
}

// ValidateActions validates a list of actions
func (v *ActionValidator) ValidateActions(actions []*kubeactions.KubeAction) []error {
	var errors []error

	for _, action := range actions {
		if err := v.ValidateAction(action); err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}
