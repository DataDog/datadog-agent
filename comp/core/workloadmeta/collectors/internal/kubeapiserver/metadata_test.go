// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func Test_MetadataFakeClient(t *testing.T) {
	t.Parallel()
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
						ID:   string(util.GenerateKubeMetadataEntityID("apps", "deployments", "default", "test-app")),
						Kind: workloadmeta.KindKubernetesMetadata,
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:        objectMeta.Name,
						Namespace:   "default",
						Labels:      objectMeta.Labels,
						Annotations: objectMeta.Annotations,
					},
					GVR: &gvr,
				},
			},
		},
	}

	testCollectMetadataEvent(t, createObjects, gvr, expected)
}

func Test_metadataFilter_filteredOut(t *testing.T) {
	tests := []struct {
		name           string
		labels         map[string]struct{}
		annotations    map[string]struct{}
		entity         workloadmeta.Entity
		expectFiltered bool
	}{
		{
			name:        "no filter keys",
			labels:      map[string]struct{}{},
			annotations: map[string]struct{}{},
			entity: &workloadmeta.KubernetesMetadata{
				EntityMeta: workloadmeta.EntityMeta{
					Labels:      map[string]string{"some-label": "1"},
					Annotations: map[string]string{"some-annotation": "2"},
				},
			},
			expectFiltered: false,
		},
		{
			name:        "entity has matching label",
			labels:      map[string]struct{}{"some-label": {}},
			annotations: map[string]struct{}{},
			entity: &workloadmeta.KubernetesMetadata{
				EntityMeta: workloadmeta.EntityMeta{
					Labels: map[string]string{"some-label": "1", "other-label": "2"},
				},
			},
			expectFiltered: false,
		},
		{
			name:        "entity has matching annotation",
			labels:      map[string]struct{}{},
			annotations: map[string]struct{}{"some-annotation": {}},
			entity: &workloadmeta.KubernetesMetadata{
				EntityMeta: workloadmeta.EntityMeta{
					Annotations: map[string]string{"some-annotation": "1", "other-annotation": "2"},
				},
			},
			expectFiltered: false,
		},
		{
			name:        "entity has no matching label or annotation",
			labels:      map[string]struct{}{"some-label": {}},
			annotations: map[string]struct{}{"some-annotation": {}},
			entity: &workloadmeta.KubernetesMetadata{
				EntityMeta: workloadmeta.EntityMeta{
					Labels:      map[string]string{"other-label": "1"},
					Annotations: map[string]string{"other-annotation": "1"},
				},
			},
			expectFiltered: true,
		},
		{
			name:        "entity has empty labels and annotations",
			labels:      map[string]struct{}{"some-label": {}},
			annotations: map[string]struct{}{},
			entity: &workloadmeta.KubernetesMetadata{
				EntityMeta: workloadmeta.EntityMeta{
					Labels:      map[string]string{},
					Annotations: map[string]string{},
				},
			},
			expectFiltered: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter := newMetadataFilter(test.labels, test.annotations)
			result := filter.filteredOut(test.entity)
			assert.Equal(t, test.expectFiltered, result)
		})
	}
}
