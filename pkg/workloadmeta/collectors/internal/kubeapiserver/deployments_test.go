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

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeploymentParser_Parse(t *testing.T) {
	tests := []struct {
		name       string
		expected   *workloadmeta.KubernetesDeployment
		deployment *appsv1.Deployment
	}{
		{
			name: "everything",
			expected: &workloadmeta.KubernetesDeployment{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesDeployment,
					ID:   "test-deployment",
				},
				Env:     "env",
				Service: "service",
				Version: "version",
				InitContainerLanguages: map[string][]languagemodels.Language{
					"nginx-cont": []languagemodels.Language{
						{Name: languagemodels.Go},
						{Name: languagemodels.Java},
						{Name: languagemodels.Python},
					},
				},
				ContainerLanguages: map[string][]languagemodels.Language{
					"nginx-cont": []languagemodels.Language{
						{Name: languagemodels.Go},
						{Name: languagemodels.Java},
						{Name: languagemodels.Python},
					},
				},
			},
			deployment: &appsv1.Deployment{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"test-label":                 "test-value",
						"tags.datadoghq.com/env":     "env",
						"tags.datadoghq.com/service": "service",
						"tags.datadoghq.com/version": "version",
					},
					Annotations: map[string]string{
						"apm.datadoghq.com/nginx-cont.languages":      "go,java,  python  ",
						"apm.datadoghq.com/init.nginx-cont.languages": "go,java,  python  ",
					},
				},
			},
		},
		{
			name: "only usm",
			expected: &workloadmeta.KubernetesDeployment{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesDeployment,
					ID:   "test-deployment",
				},
				Env:                    "env",
				Service:                "service",
				Version:                "version",
				InitContainerLanguages: map[string][]languagemodels.Language{},
				ContainerLanguages:     map[string][]languagemodels.Language{},
			},
			deployment: &appsv1.Deployment{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"test-label":                 "test-value",
						"tags.datadoghq.com/env":     "env",
						"tags.datadoghq.com/service": "service",
						"tags.datadoghq.com/version": "version",
					},
				},
			},
		},
		{
			name: "only languages",
			expected: &workloadmeta.KubernetesDeployment{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesDeployment,
					ID:   "test-deployment",
				},
				InitContainerLanguages: map[string][]languagemodels.Language{
					"nginx-cont": []languagemodels.Language{
						{Name: languagemodels.Go},
						{Name: languagemodels.Java},
						{Name: languagemodels.Python},
					},
				},
				ContainerLanguages: map[string][]languagemodels.Language{
					"nginx-cont": []languagemodels.Language{
						{Name: languagemodels.Go},
						{Name: languagemodels.Java},
						{Name: languagemodels.Python},
					},
				},
			},
			deployment: &appsv1.Deployment{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"test-label": "test-value",
					},
					Annotations: map[string]string{
						"apm.datadoghq.com/nginx-cont.languages":      "go,java,  python  ",
						"apm.datadoghq.com/init.nginx-cont.languages": "go,java,  python  ",
					},
				},
			},
		},
	}

	// Run test for each testcase
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := newdeploymentParser()
			entity := parser.Parse(tt.deployment)
			storedDeployment, ok := entity.(*workloadmeta.KubernetesDeployment)
			require.True(t, ok)
			assert.Equal(t, tt.expected, storedDeployment)
		})
	}
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
	testFakeHelper(t, createResource, newDeploymentStore, expected)
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
	testFakeHelper(t, createResource, newDeploymentStore, expected)
}
