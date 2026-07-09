// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package kubeactions

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ResourceRef identifies the Kubernetes resource an action targets. It is the
// workflow-input equivalent of the protobuf KubeResource message used by the
// remote-config implementation.
//
// JSON field names mirror the original proto so the backend workflow input
// spec maps onto them directly.
type ResourceRef struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	ResourceID string `json:"resourceId"`
	APIVersion string `json:"apiVersion,omitempty"`
}

// DeletePodInputs are the inputs for the delete_pod action.
type DeletePodInputs struct {
	ResourceRef
	GracePeriodSeconds *int64 `json:"gracePeriodSeconds,omitempty"`
}

// RestartDeploymentInputs are the inputs for the restart_deployment action.
type RestartDeploymentInputs struct {
	ResourceRef
}

// PatchDeploymentInputs are the inputs for the patch_deployment action.
type PatchDeploymentInputs struct {
	ResourceRef
	// Patch is the raw JSON patch body applied to the deployment.
	Patch json.RawMessage `json:"patch"`
	// PatchStrategy selects the Kubernetes patch type: "merge", "json", or
	// (default) strategic merge.
	PatchStrategy string `json:"patchStrategy,omitempty"`
}

// RollbackDeploymentInputs are the inputs for the rollback_deployment action.
type RollbackDeploymentInputs struct {
	ResourceRef
	// TargetRevision is the revision to roll back to; 0 (default) means the
	// previous revision (matching kubectl behaviour).
	TargetRevision int64 `json:"targetRevision,omitempty"`
}

// GetResourceInputs are the inputs for the get_resource action.
type GetResourceInputs struct {
	ResourceRef
}

// protectedNamespaces are Kubernetes system namespaces where actions must not
// be executed.
var protectedNamespaces = map[string]struct{}{
	"kube-system":     {},
	"kube-public":     {},
	"kube-node-lease": {},
}

// protectedKinds are resource kinds that get_resource must never return, even
// when the caller has RBAC permission to read them.
var protectedKinds = map[string]struct{}{
	"pods":            {},
	"secrets":         {},
	"serviceaccounts": {},
}

func isProtectedKind(kind string) bool {
	_, ok := protectedKinds[strings.ToLower(kind)]
	return ok
}

// validateCommon enforces the preflight checks shared by every action: required
// resource fields and the protected-namespace block.
//
// ResourceID (the resource UID) is intentionally NOT required: it is an optional
// safety guard. When provided, executors verify the live object's UID matches it
// before mutating; when empty, the guard is skipped and the action targets the
// resource by namespace+name alone.
func (r ResourceRef) validateCommon() error {
	if r.Kind == "" {
		return fmt.Errorf("resource.kind is required")
	}
	if r.Name == "" {
		return fmt.Errorf("resource.name is required")
	}
	if r.Namespace == "" {
		return fmt.Errorf("resource.namespace is required")
	}
	if _, ok := protectedNamespaces[r.Namespace]; ok {
		return fmt.Errorf("actions are not allowed on protected namespace %q", r.Namespace)
	}
	return nil
}

// Validate checks the delete_pod inputs.
func (in DeletePodInputs) Validate() error {
	if err := in.validateCommon(); err != nil {
		return err
	}
	if in.Kind != "Pod" {
		return fmt.Errorf("resource.kind must be 'Pod' for delete_pod action")
	}
	return nil
}

// Validate checks the restart_deployment inputs.
func (in RestartDeploymentInputs) Validate() error {
	if err := in.validateCommon(); err != nil {
		return err
	}
	if in.Kind != "Deployment" {
		return fmt.Errorf("resource.kind must be 'Deployment' for restart_deployment action")
	}
	return nil
}

// Validate checks the patch_deployment inputs.
func (in PatchDeploymentInputs) Validate() error {
	if err := in.validateCommon(); err != nil {
		return err
	}
	if in.Kind != "Deployment" {
		return fmt.Errorf("resource.kind must be 'Deployment' for patch_deployment action")
	}
	if len(in.Patch) == 0 {
		return fmt.Errorf("patch is required for patch_deployment action")
	}
	return nil
}

// Validate checks the rollback_deployment inputs.
func (in RollbackDeploymentInputs) Validate() error {
	if err := in.validateCommon(); err != nil {
		return err
	}
	if in.Kind != "Deployment" {
		return fmt.Errorf("resource.kind must be 'Deployment' for rollback_deployment action")
	}
	return nil
}

// Validate checks the get_resource inputs.
func (in GetResourceInputs) Validate() error {
	if err := in.validateCommon(); err != nil {
		return err
	}
	if in.APIVersion == "" {
		return fmt.Errorf("resource.api_version must be set for get_resource action")
	}
	if isProtectedKind(in.Kind) {
		return fmt.Errorf("actions are not allowed to get protected kind %s", in.Kind)
	}
	return nil
}
