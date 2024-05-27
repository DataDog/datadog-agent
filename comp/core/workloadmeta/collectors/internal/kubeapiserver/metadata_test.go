// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
)

func TestParse_ParsePartialObjectMetadata(t *testing.T) {

	testcases := []struct {
		name                  string
		gvr                   schema.GroupVersionResource
		partialObjectMetadata *metav1.PartialObjectMetadata
		expected              *workloadmeta.KubernetesMetadata
	}{
		{
			name: "deployments [namespace scoped]",
			gvr: schema.GroupVersionResource{
				Group:    "apps",
				Version:  "v1",
				Resource: "deployments",
			},
			partialObjectMetadata: &metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-app",
					Namespace:   "default",
					Labels:      map[string]string{"l1": "v1", "l2": "v2", "l3": "v3"},
					Annotations: map[string]string{"k1": "v1", "k2": "v2", "k3": "v3"},
				},
			},
			expected: &workloadmeta.KubernetesMetadata{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesMetadata,
					ID:   "deployments/default/test-app",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "test-app",
					Namespace:   "default",
					Labels:      map[string]string{"l1": "v1", "l2": "v2", "l3": "v3"},
					Annotations: map[string]string{"k1": "v1", "k2": "v2", "k3": "v3"},
				},
				GVR: schema.GroupVersionResource{
					Group:    "apps",
					Version:  "v1",
					Resource: "deployments",
				},
			},
		},
		{
			name: "namespaces [cluster scoped]",
			gvr: schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "namespaces",
			},
			partialObjectMetadata: &metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-namespace",
					Namespace:   "",
					Labels:      map[string]string{"l1": "v1", "l2": "v2", "l3": "v3"},
					Annotations: map[string]string{"k1": "v1", "k2": "v2", "k3": "v3"},
				},
			},
			expected: &workloadmeta.KubernetesMetadata{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesMetadata,
					ID:   "namespaces//test-namespace",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "test-namespace",
					Namespace:   "",
					Labels:      map[string]string{"l1": "v1", "l2": "v2", "l3": "v3"},
					Annotations: map[string]string{"k1": "v1", "k2": "v2", "k3": "v3"},
				},
				GVR: schema.GroupVersionResource{
					Group:    "",
					Version:  "v1",
					Resource: "namespaces",
				},
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(tt *testing.T) {
			parser := newMetadataParser(test.gvr)
			entity := parser.Parse(test.partialObjectMetadata)
			storedMetadata, ok := entity.(*workloadmeta.KubernetesMetadata)
			require.True(t, ok)
			assert.Equal(t, test.expected, storedMetadata)
		})
	}
}

func Test_MetadataFakeClient(t *testing.T) {
	ns := "default"
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	objectMeta := metav1.ObjectMeta{
		Name:        "test-app",
		Namespace:   ns,
		Labels:      map[string]string{"test-label": "test-value"},
		Annotations: map[string]string{"k": "v"},
	}

	createObjects := func() []runtime.Object {
		return []runtime.Object{
			&metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       kubernetes.DeploymentKind,
				},
				ObjectMeta: objectMeta,
			},
		}
	}

	expected := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type: workloadmeta.EventTypeSet,
				Entity: &workloadmeta.KubernetesMetadata{
					EntityID: workloadmeta.EntityID{
						ID:   "deployments/default/test-app",
						Kind: workloadmeta.KindKubernetesMetadata,
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:        objectMeta.Name,
						Namespace:   "default",
						Labels:      objectMeta.Labels,
						Annotations: objectMeta.Annotations,
					},
					GVR: gvr,
				},
			},
		},
	}

	testCollectMetadataEvent(t, createObjects, gvr, expected)
}
