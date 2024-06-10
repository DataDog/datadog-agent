// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestNamespaceParser_Parse(t *testing.T) {
	parser := newNamespaceParser()
	expected := &workloadmeta.KubernetesNamespace{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesNamespace,
			ID:   "test-namespace",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "test-namespace",
			Labels: map[string]string{"test-label": "test-value"},
		},
	}
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   expected.ID,
			Labels: expected.Labels,
		},
	}
	entity := parser.Parse(namespace)
	storedNamespace, ok := entity.(*workloadmeta.KubernetesNamespace)
	require.True(t, ok)
	assert.Equal(t, expected, storedNamespace)
}

func Test_NamespaceFakeKubernetesClient(t *testing.T) {
	objectMeta := metav1.ObjectMeta{
		Name:   "test-namespace",
		Labels: map[string]string{"test-label": "test-value"},
	}

	createResource := func(cl *fake.Clientset) error {
		_, err := cl.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{ObjectMeta: objectMeta}, metav1.CreateOptions{})
		return err
	}
	expected := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.KubernetesNamespace{
					EntityID: workloadmeta.EntityID{
						ID:   objectMeta.Name,
						Kind: workloadmeta.KindKubernetesNamespace,
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:   objectMeta.Name,
						Labels: objectMeta.Labels,
					},
				},
			},
		},
	}
	testCollectEvent(t, createResource, newNamespaceStore, expected)
}
