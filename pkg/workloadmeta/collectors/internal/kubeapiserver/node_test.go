// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNodeStore(t *testing.T) {
	mockClient := fake.NewSimpleClientset()
	ctx := context.Background()
	wlm := workloadmeta.NewMockStore()
	_, _, err := newNodeStore(ctx, wlm, mockClient)
	assert.NoError(t, err, "Expected no error while creating new node store")
}

func TestNewNodeReflectorStore(t *testing.T) {
	wlmetaStore := workloadmeta.NewMockStore()

	store := newNodeReflectorStore(wlmetaStore)

	assert.NotNil(t, store)
	assert.NotNil(t, store.seen)
	assert.NotNil(t, store.parser)
}

func TestNodeParser_Parse(t *testing.T) {
	parser := newNodeParser()

	Node := &corev1.Node{
		ObjectMeta: v1.ObjectMeta{
			Name:   "test-node",
			Labels: map[string]string{"test-label": "test-value"},
		},
	}

	entity := parser.Parse(Node)

	storedNode, ok := entity.(*workloadmeta.KubernetesNode)
	require.True(t, ok)
	assert.IsType(t, &workloadmeta.KubernetesNode{}, entity)
	assert.Equal(t, "test-node", storedNode.ID)
	assert.Equal(t, map[string]string{"test-label": "test-value"}, storedNode.Labels)
}

func Test_NodesFakeKubernetesClient(t *testing.T) {
	// Create a fake client to mock API calls.
	client := fake.NewSimpleClientset()
	objectMeta := metav1.ObjectMeta{
		Name:   "test-node",
		Labels: map[string]string{"test-label": "test-value"},
	}

	// Creating a fake node
	_, err := client.CoreV1().Nodes().Create(context.TODO(), &corev1.Node{ObjectMeta: objectMeta}, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Use the fake client in kubeapiserver context.
	wlm := workloadmeta.NewMockStore()
	config.Datadog.SetDefault("language_detection.enabled", true)
	ctx := context.Background()

	nodestore, _, err := newNodeStore(ctx, wlm, client)
	assert.NoError(t, err)
	stopNodestore := make(chan struct{})
	go nodestore.Run(stopNodestore)

	ch := wlm.Subscribe(dummySubscriber, workloadmeta.NormalPriority, nil)
	doneCh := make(chan struct{})

	expected := []workloadmeta.EventBundle{
		{
			Events: []workloadmeta.Event{
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.KubernetesNode{
						EntityID: workloadmeta.EntityID{
							ID:   objectMeta.Name,
							Kind: workloadmeta.KindKubernetesNode,
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:   objectMeta.Name,
							Labels: objectMeta.Labels,
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
	close(stopNodestore)
	wlm.Unsubscribe(ch)
	assert.Equal(t, expected, actual)
}
