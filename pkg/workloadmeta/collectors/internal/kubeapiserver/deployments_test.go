// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	"strings"
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
	tests := []struct {
		name       string
		configFunc func() config.Config
		expectErr  bool
	}{
		{
			name: "New Deployment Store Test",
			configFunc: func() config.Config {
				cfg := config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
				cfg.SetDefault("language_detection.enabled", true)
				return cfg
			},
			expectErr: false,
		},
		{
			name: "Fail new deployment with error",
			configFunc: func() config.Config {
				return config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
			},
			expectErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := TestNewResourceStore(t, newDeploymentStore, tt.configFunc())
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
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

func Test_DeploymentsFakeKubernetesClient(t *testing.T) {
	cfg := config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	cfg.SetDefault("language_detection.enabled", true)

	objectMeta := metav1.ObjectMeta{
		Name:      "test-deployment",
		Namespace: "test-namespace",
		Labels:    map[string]string{"test-label": "test-value"},
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
						EntityMeta: workloadmeta.EntityMeta{
							Name:      objectMeta.Name,
							Namespace: objectMeta.Namespace,
							Labels:    objectMeta.Labels,
						},
					},
				},
			},
		},
	}
	TestFakeHelper(t, cfg, createResource, newDeploymentStore, expected)
}
