// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package executors

import (
	"context"
	"testing"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/structpb"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

// mustNewValue creates a structpb.Value from a Go value, panicking on error.
func mustNewValue(v interface{}) *structpb.Value {
	val, err := structpb.NewValue(v)
	if err != nil {
		panic(err)
	}
	return val
}

func TestPatchDeploymentExecutor_ScaleReplicas(t *testing.T) {
	replicas := int32(2)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deployment",
			Namespace: "default",
			UID:       k8stypes.UID("test-uid-123"),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
	}

	clientset := fake.NewSimpleClientset(deployment)
	executor := NewPatchDeploymentExecutor(clientset)

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "Deployment",
			Namespace:  "default",
			Name:       "my-deployment",
			ResourceId: "test-uid-123",
		},
		Action: &kubeactions.KubeAction_PatchDeployment{
			PatchDeployment: &kubeactions.PatchDeploymentParams{
				Patch: mustNewValue(map[string]interface{}{
					"spec": map[string]interface{}{
						"replicas": 5,
					},
				}),
			},
		},
	}

	result := executor.Execute(context.Background(), action)
	assert.Equal(t, "success", result.Status)
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

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "Deployment",
			Namespace:  "default",
			Name:       "my-deployment",
			ResourceId: "wrong-uid",
		},
		Action: &kubeactions.KubeAction_PatchDeployment{
			PatchDeployment: &kubeactions.PatchDeploymentParams{
				Patch: mustNewValue(map[string]interface{}{
					"spec": map[string]interface{}{"replicas": 5},
				}),
			},
		},
	}

	result := executor.Execute(context.Background(), action)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Message, "UID mismatch")
}

func TestPatchDeploymentExecutor_MissingNamespace(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	executor := NewPatchDeploymentExecutor(clientset)

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "Deployment",
			Namespace:  "",
			Name:       "my-deployment",
			ResourceId: "test-uid",
		},
		Action: &kubeactions.KubeAction_PatchDeployment{
			PatchDeployment: &kubeactions.PatchDeploymentParams{
				Patch: mustNewValue(map[string]interface{}{
					"spec": map[string]interface{}{"replicas": 5},
				}),
			},
		},
	}

	result := executor.Execute(context.Background(), action)
	assert.Equal(t, "failed", result.Status)
	// Namespace validation happens in the validator layer before the executor is called.
	// With an empty namespace, the k8s API returns a not-found error.
	assert.Contains(t, result.Message, "not found")
}

func TestPatchDeploymentExecutor_MissingPatch(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	executor := NewPatchDeploymentExecutor(clientset)

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "Deployment",
			Namespace:  "default",
			Name:       "my-deployment",
			ResourceId: "test-uid",
		},
		Action: &kubeactions.KubeAction_PatchDeployment{
			PatchDeployment: &kubeactions.PatchDeploymentParams{
				Patch: nil,
			},
		},
	}

	result := executor.Execute(context.Background(), action)
	assert.Equal(t, "failed", result.Status)
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
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
	}

	clientset := fake.NewSimpleClientset(deployment)
	executor := NewPatchDeploymentExecutor(clientset)

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "Deployment",
			Namespace:  "default",
			Name:       "my-deployment",
			ResourceId: "test-uid-merge",
		},
		Action: &kubeactions.KubeAction_PatchDeployment{
			PatchDeployment: &kubeactions.PatchDeploymentParams{
				Patch: mustNewValue(map[string]interface{}{
					"spec": map[string]interface{}{"replicas": 10},
				}),
				PatchStrategy: "merge",
			},
		},
	}

	result := executor.Execute(context.Background(), action)
	assert.Equal(t, "success", result.Status)
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

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "Deployment",
			Namespace:  "default",
			Name:       "nonexistent",
			ResourceId: "test-uid",
		},
		Action: &kubeactions.KubeAction_PatchDeployment{
			PatchDeployment: &kubeactions.PatchDeploymentParams{
				Patch: mustNewValue(map[string]interface{}{
					"spec": map[string]interface{}{"replicas": 5},
				}),
			},
		},
	}

	result := executor.Execute(context.Background(), action)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Message, "failed to get deployment")
}
