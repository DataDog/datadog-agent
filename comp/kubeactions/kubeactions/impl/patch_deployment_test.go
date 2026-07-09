// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeactionsimpl

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
)

// mustJSON marshals v to a json.RawMessage, panicking on error.
func mustJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func patchInputs(namespace, name, resourceID string, patch json.RawMessage, strategy string) kubeactions.PatchDeploymentInputs {
	return kubeactions.PatchDeploymentInputs{
		ResourceRef: kubeactions.ResourceRef{
			Kind:       "Deployment",
			Namespace:  namespace,
			Name:       name,
			ResourceID: resourceID,
		},
		Patch:         patch,
		PatchStrategy: strategy,
	}
}

func TestPatchDeploymentExecutor_ScaleReplicas(t *testing.T) {
	replicas := int32(2)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deployment",
			Namespace: "default",
			UID:       k8stypes.UID("test-uid-123"),
		},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas},
	}

	clientset := fake.NewSimpleClientset(deployment)
	executor := NewPatchDeploymentExecutor(clientset)

	in := patchInputs("default", "my-deployment", "test-uid-123",
		mustJSON(map[string]interface{}{"spec": map[string]interface{}{"replicas": 5}}), "")

	result := executor.Execute(context.Background(), in)
	assert.Equal(t, kubeactions.StatusSuccess, result.Status)
	assert.Contains(t, result.Message, "patched")
}

func TestPatchDeploymentExecutor_UIDMismatch(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deployment",
			Namespace: "default",
			UID:       k8stypes.UID("real-uid"),
		},
	}

	clientset := fake.NewSimpleClientset(deployment)
	executor := NewPatchDeploymentExecutor(clientset)

	in := patchInputs("default", "my-deployment", "wrong-uid",
		mustJSON(map[string]interface{}{"spec": map[string]interface{}{"replicas": 5}}), "")

	result := executor.Execute(context.Background(), in)
	assert.Equal(t, kubeactions.StatusFailed, result.Status)
	assert.Contains(t, result.Message, "UID mismatch")
}

func TestPatchDeploymentExecutor_MissingNamespace(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	executor := NewPatchDeploymentExecutor(clientset)

	in := patchInputs("", "my-deployment", "test-uid",
		mustJSON(map[string]interface{}{"spec": map[string]interface{}{"replicas": 5}}), "")

	result := executor.Execute(context.Background(), in)
	assert.Equal(t, kubeactions.StatusFailed, result.Status)
	// Namespace validation happens in the validator layer before the executor is called.
	// With an empty namespace, the k8s API returns a not-found error.
	assert.Contains(t, result.Message, "not found")
}

func TestPatchDeploymentExecutor_MissingPatch(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	executor := NewPatchDeploymentExecutor(clientset)

	in := patchInputs("default", "my-deployment", "test-uid", nil, "")

	result := executor.Execute(context.Background(), in)
	assert.Equal(t, kubeactions.StatusFailed, result.Status)
	assert.Contains(t, result.Message, "patch is required")
}

func TestPatchDeploymentExecutor_WithMergeStrategy(t *testing.T) {
	replicas := int32(2)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deployment",
			Namespace: "default",
			UID:       k8stypes.UID("test-uid-merge"),
		},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas},
	}

	clientset := fake.NewSimpleClientset(deployment)
	executor := NewPatchDeploymentExecutor(clientset)

	in := patchInputs("default", "my-deployment", "test-uid-merge",
		mustJSON(map[string]interface{}{"spec": map[string]interface{}{"replicas": 10}}), "merge")

	result := executor.Execute(context.Background(), in)
	assert.Equal(t, kubeactions.StatusSuccess, result.Status)
	assert.Contains(t, result.Message, "patched")
}

func TestResolvePatchType(t *testing.T) {
	assert.Equal(t, k8stypes.StrategicMergePatchType, resolvePatchType(""))
	assert.Equal(t, k8stypes.StrategicMergePatchType, resolvePatchType("strategic-merge"))
	assert.Equal(t, k8stypes.MergePatchType, resolvePatchType("merge"))
	assert.Equal(t, k8stypes.JSONPatchType, resolvePatchType("json"))
	assert.Equal(t, k8stypes.StrategicMergePatchType, resolvePatchType("unknown"))
}

func TestPatchDeploymentExecutor_DeploymentNotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset() // empty — no deployments
	executor := NewPatchDeploymentExecutor(clientset)

	in := patchInputs("default", "nonexistent", "test-uid",
		mustJSON(map[string]interface{}{"spec": map[string]interface{}{"replicas": 5}}), "")

	result := executor.Execute(context.Background(), in)
	assert.Equal(t, kubeactions.StatusFailed, result.Status)
	assert.Contains(t, result.Message, "failed to get deployment")
}
