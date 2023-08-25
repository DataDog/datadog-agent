// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDeploymentStore(t *testing.T) {
	mockClient := fake.NewSimpleClientset()

	config.Datadog.SetDefault("language_detection.enabled", true)

	ctx := context.Background()

	wlm := workloadmeta.NewMockStore()

	_, _, err := newDeploymentStore(ctx, wlm, mockClient)
	assert.NoError(t, err, "Expected no error while creating new deployment store")
}

func TestNewDeploymentReflectorStore(t *testing.T) {
	wlmetaStore := workloadmeta.NewMockStore()

	store := newDeploymentReflectorStore(wlmetaStore)

	assert.NotNil(t, store)
	assert.NotNil(t, store.seen)
	assert.NotNil(t, store.parser)
}

func TestDeploymentParser_Parse(t *testing.T) {
	parser := newdeploymentParser()

	deployment := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
			Labels:    map[string]string{"test-label": "test-value"},
		},
	}

	entity := parser.Parse(deployment)

	storedDeployment, ok := entity.(*workloadmeta.KubernetesDeployment)
	require.True(t, ok)
	assert.IsType(t, &workloadmeta.KubernetesDeployment{}, entity)
	assert.Equal(t, "test-deployment", storedDeployment.ID)
	assert.Equal(t, "test-namespace", storedDeployment.Namespace)
	assert.Equal(t, map[string]string{"test-label": "test-value"}, storedDeployment.Labels)
}

func Test_FakeKubernetesClient(t *testing.T) {
	// Create a fake client to mock API calls.
	client := fake.NewSimpleClientset()

	// Creating a fake deployment
	_, err := client.AppsV1().Deployments("default").Create(context.TODO(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fake-deployment",
		},
	}, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Use the fake client in kubeapiserver context.
	wlm := workloadmeta.NewMockStore()
	config.Datadog.SetDefault("language_detection.enabled", true)
	ctx := context.Background()

	_, _, err = newDeploymentStore(ctx, wlm, client)
	assert.NoError(t, err)

	// Use list to confirm it's added to the fake client.
	deployments, err := client.AppsV1().Deployments("default").List(context.TODO(), metav1.ListOptions{})
	assert.NoError(t, err)

	assert.Equal(t, 1, len(deployments.Items))
	assert.Equal(t, "fake-deployment", deployments.Items[0].Name)
}
