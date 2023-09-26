// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeParser_Parse(t *testing.T) {
	parser := newNodeParser()
	expected := &workloadmeta.KubernetesNode{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesNode,
			ID:   "test-node",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "test-node",
			Labels: map[string]string{"test-label": "test-value"},
		},
	}
	node := &corev1.Node{
		ObjectMeta: v1.ObjectMeta{
			Name:   expected.ID,
			Labels: expected.Labels,
		},
	}
	entity := parser.Parse(node)
	storedNode, ok := entity.(*workloadmeta.KubernetesNode)
	require.True(t, ok)
	assert.Equal(t, expected, storedNode)
}

func Test_NodesFakeKubernetesClient(t *testing.T) {
	objectMeta := metav1.ObjectMeta{
		Name:   "test-node",
		Labels: map[string]string{"test-label": "test-value"},
	}

	createResource := func(cl *fake.Clientset) error {
		_, err := cl.CoreV1().Nodes().Create(context.TODO(), &corev1.Node{ObjectMeta: objectMeta}, metav1.CreateOptions{})
		return err
	}
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
	testCollectEvent(t, createResource, newNodeStore, expected)
}
