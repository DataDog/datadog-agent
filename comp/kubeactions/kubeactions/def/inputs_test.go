// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package kubeactions

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validRef returns a resource reference that passes the common preflight checks.
func validRef(kind string) ResourceRef {
	return ResourceRef{
		Kind:       kind,
		Name:       "my-resource",
		Namespace:  "default",
		ResourceID: "uid-123",
	}
}

func TestResourceRefValidateCommon(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ResourceRef)
		wantErr bool
	}{
		{"valid", func(*ResourceRef) {}, false},
		{"missing kind", func(r *ResourceRef) { r.Kind = "" }, true},
		{"missing name", func(r *ResourceRef) { r.Name = "" }, true},
		{"missing namespace", func(r *ResourceRef) { r.Namespace = "" }, true},
		{"missing resource_id is allowed (optional UID guard)", func(r *ResourceRef) { r.ResourceID = "" }, false},
		{"protected namespace kube-system", func(r *ResourceRef) { r.Namespace = "kube-system" }, true},
		{"protected namespace kube-public", func(r *ResourceRef) { r.Namespace = "kube-public" }, true},
		{"protected namespace kube-node-lease", func(r *ResourceRef) { r.Namespace = "kube-node-lease" }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validRef("Pod")
			tt.mutate(&r)
			err := r.validateCommon()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeletePodInputsValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, DeletePodInputs{ResourceRef: validRef("Pod")}.Validate())
	})
	t.Run("wrong kind", func(t *testing.T) {
		assert.Error(t, DeletePodInputs{ResourceRef: validRef("Deployment")}.Validate())
	})
	t.Run("protected namespace rejected before kind", func(t *testing.T) {
		in := DeletePodInputs{ResourceRef: validRef("Pod")}
		in.Namespace = "kube-system"
		assert.Error(t, in.Validate())
	})
}

func TestRestartDeploymentInputsValidate(t *testing.T) {
	assert.NoError(t, RestartDeploymentInputs{ResourceRef: validRef("Deployment")}.Validate())
	assert.Error(t, RestartDeploymentInputs{ResourceRef: validRef("Pod")}.Validate())
}

func TestPatchDeploymentInputsValidate(t *testing.T) {
	patch := json.RawMessage(`{"spec":{"replicas":3}}`)
	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, PatchDeploymentInputs{ResourceRef: validRef("Deployment"), Patch: patch}.Validate())
	})
	t.Run("wrong kind", func(t *testing.T) {
		assert.Error(t, PatchDeploymentInputs{ResourceRef: validRef("Pod"), Patch: patch}.Validate())
	})
	t.Run("missing patch", func(t *testing.T) {
		assert.Error(t, PatchDeploymentInputs{ResourceRef: validRef("Deployment")}.Validate())
	})
	t.Run("empty patch", func(t *testing.T) {
		assert.Error(t, PatchDeploymentInputs{ResourceRef: validRef("Deployment"), Patch: json.RawMessage{}}.Validate())
	})
}

func TestRollbackDeploymentInputsValidate(t *testing.T) {
	assert.NoError(t, RollbackDeploymentInputs{ResourceRef: validRef("Deployment")}.Validate())
	assert.Error(t, RollbackDeploymentInputs{ResourceRef: validRef("Pod")}.Validate())
}

func TestGetResourceInputsValidate(t *testing.T) {
	withAPI := func(kind string) GetResourceInputs {
		in := GetResourceInputs{ResourceRef: validRef(kind)}
		in.APIVersion = "v1"
		return in
	}

	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, withAPI("ConfigMap").Validate())
	})
	t.Run("missing api_version", func(t *testing.T) {
		in := GetResourceInputs{ResourceRef: validRef("ConfigMap")}
		assert.Error(t, in.Validate())
	})

	// Protected kinds must be rejected regardless of case.
	for _, kind := range []string{"pods", "Pods", "secrets", "Secrets", "serviceaccounts", "ServiceAccounts"} {
		t.Run("protected kind "+kind, func(t *testing.T) {
			err := withAPI(kind).Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "protected kind")
		})
	}
}
