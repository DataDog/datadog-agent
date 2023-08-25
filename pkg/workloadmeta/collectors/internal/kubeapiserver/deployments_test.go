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

func Test_DeploymentsFakeKubernetesClient(t *testing.T) {
	// Create a fake client to mock API calls.
	client := fake.NewSimpleClientset()
	objectMeta := metav1.ObjectMeta{
		Name:      "test-deployment",
		Namespace: "test-namespace",
		Labels:    map[string]string{"test-label": "test-value"},
	}

	// Creating a fake deployment
	_, err := client.AppsV1().Deployments(objectMeta.Namespace).Create(context.TODO(), &appsv1.Deployment{ObjectMeta: objectMeta}, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Use the fake client in kubeapiserver context.
	wlm := workloadmeta.NewMockStore()
	config.Datadog.SetDefault("language_detection.enabled", true)
	ctx := context.Background()

	deploymentStore, _, err := newDeploymentStore(ctx, wlm, client)
	assert.NoError(t, err)
	stopDeploymentStore := make(chan struct{})
	go deploymentStore.Run(stopDeploymentStore)

	ch := wlm.Subscribe(dummySubscriber, workloadmeta.NormalPriority, nil)
	doneCh := make(chan struct{})

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

	actual := []workloadmeta.EventBundle{}
	go func() {
		<-ch
		bundle := <-ch
		close(bundle.Ch)

		// nil the bundle's Ch so we can
		// deep-equal just the events later
		bundle.Ch = nil

		actual = append(actual, bundle)

		close(doneCh)
	}()

	<-doneCh
	close(stopDeploymentStore)
	wlm.Unsubscribe(ch)
	assert.Equal(t, expected, actual)
}
