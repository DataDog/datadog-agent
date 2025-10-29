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
	return fmt.Sprintf("validation error for action %s on %s/%s: %s",
		e.Action.ActionType,
		e.Action.Resource.Kind,
		e.Action.Resource.Name,
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

	// Validate action type is provided
	if action.ActionType == "" {
		return &ValidationError{
			Action:  action,
			Message: "action_type is required",
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

	// TODO: Add more validation logic here as needed:
	// - Check if action type is in allowed list
	// - Check if namespace is allowed
	// - Check if resource type is allowed
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
