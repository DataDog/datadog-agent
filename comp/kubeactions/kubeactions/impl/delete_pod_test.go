// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeactionsimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
)

func makePod(name, namespace string, uid k8stypes.UID) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       uid,
		},
	}
}

func deletePodInputs(namespace, name, resourceID string, grace *int64) kubeactions.DeletePodInputs {
	return kubeactions.DeletePodInputs{
		ResourceRef: kubeactions.ResourceRef{
			Kind:       "Pod",
			Namespace:  namespace,
			Name:       name,
			ResourceID: resourceID,
		},
		GracePeriodSeconds: grace,
	}
}

func TestDeletePodExecutor_Success(t *testing.T) {
	clientset := fake.NewSimpleClientset(makePod("my-pod", "default", "pod-uid"))
	executor := NewDeletePodExecutor(clientset)

	result := executor.Execute(context.Background(), deletePodInputs("default", "my-pod", "pod-uid", nil))
	assert.Equal(t, kubeactions.StatusSuccess, result.Status)
	assert.Contains(t, result.Message, "deleted")

	_, err := clientset.CoreV1().Pods("default").Get(context.Background(), "my-pod", metav1.GetOptions{})
	assert.Error(t, err, "pod should no longer exist")
}

func TestDeletePodExecutor_GracePeriodForwarded(t *testing.T) {
	clientset := fake.NewSimpleClientset(makePod("my-pod", "default", "pod-uid"))
	executor := NewDeletePodExecutor(clientset)

	grace := int64(30)
	result := executor.Execute(context.Background(), deletePodInputs("default", "my-pod", "pod-uid", &grace))
	assert.Equal(t, kubeactions.StatusSuccess, result.Status)

	var deleteSeen bool
	for _, action := range clientset.Actions() {
		if action.GetVerb() == "delete" && action.GetResource().Resource == "pods" {
			deleteSeen = true
		}
	}
	require.True(t, deleteSeen, "expected a delete action on pods")
}

func TestDeletePodExecutor_EmptyResourceIDSkipsGuard(t *testing.T) {
	clientset := fake.NewSimpleClientset(makePod("my-pod", "default", "any-uid"))
	executor := NewDeletePodExecutor(clientset)

	// No resource_id supplied: the UID guard is skipped and the pod is deleted by name.
	result := executor.Execute(context.Background(), deletePodInputs("default", "my-pod", "", nil))
	assert.Equal(t, kubeactions.StatusSuccess, result.Status)

	_, err := clientset.CoreV1().Pods("default").Get(context.Background(), "my-pod", metav1.GetOptions{})
	assert.Error(t, err, "pod should have been deleted")
}

func TestDeletePodExecutor_UIDMismatch(t *testing.T) {
	clientset := fake.NewSimpleClientset(makePod("my-pod", "default", "real-uid"))
	executor := NewDeletePodExecutor(clientset)

	result := executor.Execute(context.Background(), deletePodInputs("default", "my-pod", "wrong-uid", nil))
	assert.Equal(t, kubeactions.StatusFailed, result.Status)
	assert.Contains(t, result.Message, "UID mismatch")

	// Pod must still exist — a UID mismatch must never delete.
	_, err := clientset.CoreV1().Pods("default").Get(context.Background(), "my-pod", metav1.GetOptions{})
	assert.NoError(t, err)
}

func TestDeletePodExecutor_NotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	executor := NewDeletePodExecutor(clientset)

	result := executor.Execute(context.Background(), deletePodInputs("default", "nonexistent", "pod-uid", nil))
	assert.Equal(t, kubeactions.StatusFailed, result.Status)
	assert.Contains(t, result.Message, "failed to get pod")
}
