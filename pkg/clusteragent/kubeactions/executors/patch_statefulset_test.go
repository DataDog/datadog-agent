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
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPatchStatefulSetExecutor_ScaleReplicas(t *testing.T) {
	replicas := int32(2)
	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-statefulset",
			Namespace: "default",
			UID:       k8stypes.UID("sts-uid-123"),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
		},
	}

	clientset := fake.NewSimpleClientset(statefulset)
	executor := NewPatchStatefulSetExecutor(clientset)

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "StatefulSet",
			Namespace:  "default",
			Name:       "my-statefulset",
			ResourceId: "sts-uid-123",
		},
		Action: &kubeactions.KubeAction_PatchStatefulset{
			PatchStatefulset: &kubeactions.PatchStatefulSetParams{
				Patch: mustNewValue(map[string]interface{}{
					"spec": map[string]interface{}{"replicas": 5},
				}),
			},
		},
	}

	result := executor.Execute(context.Background(), action)
	assert.Equal(t, "success", result.Status)
	assert.Contains(t, result.Message, "patched")
}

func TestPatchStatefulSetExecutor_UIDMismatch(t *testing.T) {
	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-statefulset",
			Namespace: "default",
			UID:       k8stypes.UID("real-uid"),
		},
	}

	clientset := fake.NewSimpleClientset(statefulset)
	executor := NewPatchStatefulSetExecutor(clientset)

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "StatefulSet",
			Namespace:  "default",
			Name:       "my-statefulset",
			ResourceId: "wrong-uid",
		},
		Action: &kubeactions.KubeAction_PatchStatefulset{
			PatchStatefulset: &kubeactions.PatchStatefulSetParams{
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

func TestPatchStatefulSetExecutor_MissingPatch(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	executor := NewPatchStatefulSetExecutor(clientset)

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "StatefulSet",
			Namespace:  "default",
			Name:       "my-statefulset",
			ResourceId: "sts-uid",
		},
		Action: &kubeactions.KubeAction_PatchStatefulset{
			PatchStatefulset: &kubeactions.PatchStatefulSetParams{
				Patch: nil,
			},
		},
	}

	result := executor.Execute(context.Background(), action)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Message, "patch is required")
}

func TestPatchStatefulSetExecutor_NotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset() // empty — no statefulsets
	executor := NewPatchStatefulSetExecutor(clientset)

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "StatefulSet",
			Namespace:  "default",
			Name:       "nonexistent",
			ResourceId: "sts-uid",
		},
		Action: &kubeactions.KubeAction_PatchStatefulset{
			PatchStatefulset: &kubeactions.PatchStatefulSetParams{
				Patch: mustNewValue(map[string]interface{}{
					"spec": map[string]interface{}{"replicas": 5},
				}),
			},
		},
	}

	result := executor.Execute(context.Background(), action)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Message, "failed to get statefulset")
}
