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

func TestPatchDaemonSetExecutor_Patch(t *testing.T) {
	daemonset := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-daemonset",
			Namespace: "default",
			UID:       k8stypes.UID("ds-uid-123"),
		},
	}

	clientset := fake.NewSimpleClientset(daemonset)
	executor := NewPatchDaemonSetExecutor(clientset)

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "DaemonSet",
			Namespace:  "default",
			Name:       "my-daemonset",
			ResourceId: "ds-uid-123",
		},
		Action: &kubeactions.KubeAction_PatchDaemonset{
			PatchDaemonset: &kubeactions.PatchDaemonSetParams{
				Patch: mustNewValue(map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{"name": "app", "image": "app:v2"},
								},
							},
						},
					},
				}),
			},
		},
	}

	result := executor.Execute(context.Background(), action)
	assert.Equal(t, "success", result.Status)
	assert.Contains(t, result.Message, "patched")
}

func TestPatchDaemonSetExecutor_UIDMismatch(t *testing.T) {
	daemonset := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-daemonset",
			Namespace: "default",
			UID:       k8stypes.UID("real-uid"),
		},
	}

	clientset := fake.NewSimpleClientset(daemonset)
	executor := NewPatchDaemonSetExecutor(clientset)

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "DaemonSet",
			Namespace:  "default",
			Name:       "my-daemonset",
			ResourceId: "wrong-uid",
		},
		Action: &kubeactions.KubeAction_PatchDaemonset{
			PatchDaemonset: &kubeactions.PatchDaemonSetParams{
				Patch: mustNewValue(map[string]interface{}{
					"spec": map[string]interface{}{"minReadySeconds": 5},
				}),
			},
		},
	}

	result := executor.Execute(context.Background(), action)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Message, "UID mismatch")
}

func TestPatchDaemonSetExecutor_MissingPatch(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	executor := NewPatchDaemonSetExecutor(clientset)

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "DaemonSet",
			Namespace:  "default",
			Name:       "my-daemonset",
			ResourceId: "ds-uid",
		},
		Action: &kubeactions.KubeAction_PatchDaemonset{
			PatchDaemonset: &kubeactions.PatchDaemonSetParams{
				Patch: nil,
			},
		},
	}

	result := executor.Execute(context.Background(), action)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Message, "patch is required")
}

func TestPatchDaemonSetExecutor_NotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset() // empty — no daemonsets
	executor := NewPatchDaemonSetExecutor(clientset)

	action := &kubeactions.KubeAction{
		Resource: &kubeactions.KubeResource{
			Kind:       "DaemonSet",
			Namespace:  "default",
			Name:       "nonexistent",
			ResourceId: "ds-uid",
		},
		Action: &kubeactions.KubeAction_PatchDaemonset{
			PatchDaemonset: &kubeactions.PatchDaemonSetParams{
				Patch: mustNewValue(map[string]interface{}{
					"spec": map[string]interface{}{"minReadySeconds": 5},
				}),
			},
		},
	}

	result := executor.Execute(context.Background(), action)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Message, "failed to get daemonset")
}
