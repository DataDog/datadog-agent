// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeploymentParser_Parse(t *testing.T) {
	parser := newdeploymentParser()
	deployment := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
			Labels:    map[string]string{"test-label": "test-value", "tags.datadoghq.com/env": "env", "tags.datadoghq.com/service": "service", "tags.datadoghq.com/version": "version"},
		},
	}
	entity := parser.Parse(deployment)
	storedDeployment, ok := entity.(*workloadmeta.KubernetesDeployment)
	require.True(t, ok)
	assert.IsType(t, &workloadmeta.KubernetesDeployment{}, entity)
	assert.Equal(t, "test-deployment", storedDeployment.ID)
	assert.Equal(t, "env", storedDeployment.Env)
	assert.Equal(t, "service", storedDeployment.Service)
	assert.Equal(t, "version", storedDeployment.Version)
}

func Test_DeploymentsFakeKubernetesClient(t *testing.T) {
	objectMeta := metav1.ObjectMeta{
		Name:      "test-deployment",
		Namespace: "test-namespace",
		Labels:    map[string]string{"test-label": "test-value", "tags.datadoghq.com/env": "env"},
	}
	createResource := func(cl *fake.Clientset) error {
		_, err := cl.AppsV1().Deployments(objectMeta.Namespace).Create(context.TODO(), &appsv1.Deployment{ObjectMeta: objectMeta}, metav1.CreateOptions{})
		return err
	}
	expected := []workloadmeta.EventBundle{
		{
			Events: []workloadmeta.Event{
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.KubernetesDeployment{
						EntityID: workloadmeta.EntityID{
							ID:   objectMeta.Name,
							Kind: workloadmeta.KindKubernetesDeployment,
						},
						Env: "env",
					},
				},
			},
		},
	}
	TestFakeHelper(t, createResource, newDeploymentStore, expected)
}

func Test_Deployment_FilteredOut(t *testing.T) {
	filteredOutObjectMeta := metav1.ObjectMeta{
		Name:      "test-deployment-filtered-out",
		Namespace: "test-namespace",
		Labels:    map[string]string{"test-label": "test-value"},
	}
	objectMeta := metav1.ObjectMeta{
		Name:      "test-deployment",
		Namespace: "test-namespace",
		Labels:    map[string]string{"test-label": "test-value", "tags.datadoghq.com/env": "env"},
	}
	createResource := func(cl *fake.Clientset) error {
		_, err := cl.AppsV1().Deployments(objectMeta.Namespace).Create(context.TODO(), &appsv1.Deployment{ObjectMeta: filteredOutObjectMeta}, metav1.CreateOptions{})
		require.NoError(t, err)
		_, err = cl.AppsV1().Deployments(objectMeta.Namespace).Create(context.TODO(), &appsv1.Deployment{ObjectMeta: objectMeta}, metav1.CreateOptions{})
		return err
	}
	expected := []workloadmeta.EventBundle{
		{
			Events: []workloadmeta.Event{
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.KubernetesDeployment{
						EntityID: workloadmeta.EntityID{
							ID:   objectMeta.Name,
							Kind: workloadmeta.KindKubernetesDeployment,
						},
						Env: "env",
					},
				},
			},
		},
	}
	TestFakeHelper(t, createResource, newDeploymentStore, expected)
}
