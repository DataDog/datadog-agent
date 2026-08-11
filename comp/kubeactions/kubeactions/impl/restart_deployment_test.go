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
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
)

func restartInputs(namespace, name, resourceID string) kubeactions.RestartDeploymentInputs {
	return kubeactions.RestartDeploymentInputs{
		ResourceRef: kubeactions.ResourceRef{
			Kind:       "Deployment",
			Namespace:  namespace,
			Name:       name,
			ResourceID: resourceID,
		},
	}
}

func TestRestartDeploymentExecutor_Success(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deploy",
			Namespace: "default",
			UID:       k8stypes.UID("deploy-uid"),
		},
	}
	clientset := fake.NewSimpleClientset(deployment)
	executor := NewRestartDeploymentExecutor(clientset)

	result := executor.Execute(context.Background(), restartInputs("default", "my-deploy", "deploy-uid"))
	assert.Equal(t, kubeactions.StatusSuccess, result.Status)
	assert.Contains(t, result.Message, "restarted")

	updated, err := clientset.AppsV1().Deployments("default").Get(context.Background(), "my-deploy", metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotEmpty(t, updated.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"],
		"restart annotation should be set")
}

func TestRestartDeploymentExecutor_UIDMismatch(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-deploy",
			Namespace: "default",
			UID:       k8stypes.UID("real-uid"),
		},
	}
	clientset := fake.NewSimpleClientset(deployment)
	executor := NewRestartDeploymentExecutor(clientset)

	result := executor.Execute(context.Background(), restartInputs("default", "my-deploy", "wrong-uid"))
	assert.Equal(t, kubeactions.StatusFailed, result.Status)
	assert.Contains(t, result.Message, "UID mismatch")
}

func TestRestartDeploymentExecutor_NotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	executor := NewRestartDeploymentExecutor(clientset)

	result := executor.Execute(context.Background(), restartInputs("default", "nonexistent", "deploy-uid"))
	assert.Equal(t, kubeactions.StatusFailed, result.Status)
	assert.Contains(t, result.Message, "failed to get deployment")
}
