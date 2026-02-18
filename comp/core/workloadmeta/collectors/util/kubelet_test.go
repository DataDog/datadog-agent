// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// TestParseKubeletPods_ImageFromContainerSpec tests that the image from the container spec is used
func TestParseKubeletPods_ImageFromContainerSpec(t *testing.T) {
	// The container spec image should be used because it preserves the tag even when a digest is specified
	pod := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid-123",
		},
		Spec: kubelet.Spec{
			Containers: []kubelet.ContainerSpec{
				{
					Name: "test-container",
					// Spec has tag + digest
					Image: "nginx:1.23.0@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0",
				},
			},
		},
		Status: kubelet.Status{
			Containers: []kubelet.ContainerStatus{
				{
					Name: "test-container",
					ID:   "containerd://abc123",
					// Status may not have the tag (only digest)
					Image:   "nginx@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0",
					ImageID: "docker-pullable://nginx@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0",
				},
			},
		},
	}

	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	events := ParseKubeletPods([]*kubelet.Pod{pod}, false, mockStore)

	// Find the container event
	var containerEvent *workloadmeta.CollectorEvent
	for i := range events {
		if events[i].Entity.GetID().Kind == workloadmeta.KindContainer {
			containerEvent = &events[i]
			break
		}
	}

	require.NotNil(t, containerEvent)
	container := containerEvent.Entity.(*workloadmeta.Container)

	// The image should come from the spec, which has the tag
	assert.Equal(t, "nginx:1.23.0@sha256:5bef08742407efd622d243692b79ba0055383bbce12900324f75e56f589aedb0", container.Image.RawName)
	assert.Equal(t, "nginx", container.Image.Name)
	assert.Equal(t, "1.23.0", container.Image.Tag, "tag should be preserved from spec")
	assert.Equal(t, "nginx", container.Image.ShortName)
}

// TestParseKubeletPods_NamespaceMetadataFromCache tests that namespace labels and annotations
// are applied to pods from cached namespace metadata entities
func TestParseKubeletPods_NamespaceMetadataFromCache(t *testing.T) {
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	// Create namespace metadata entity with labels and annotations
	namespaceLabels := map[string]string{
		"team":        "container-platform",
		"environment": "production",
	}
	namespaceAnnotations := map[string]string{
		"owner":       "team-container-platform@example.com",
		"cost-center": "engineering",
	}

	nsEntityID := GenerateKubeMetadataEntityID("", "namespaces", "", "test-namespace")
	nsEntity := &workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   string(nsEntityID),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        "test-namespace",
			Namespace:   "test-namespace",
			Labels:      namespaceLabels,
			Annotations: namespaceAnnotations,
		},
		GVR: &schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "namespaces",
		},
	}
	mockStore.Set(nsEntity)

	// Create a pod in the namespace
	podLabels := map[string]string{
		"app": "web-server",
	}
	podAnnotations := map[string]string{
		"version": "1.0.0",
	}
	pod := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:        "test-pod",
			Namespace:   "test-namespace",
			UID:         "test-pod-uid-456",
			Labels:      podLabels,
			Annotations: podAnnotations,
		},
		Spec: kubelet.Spec{
			Containers: []kubelet.ContainerSpec{
				{
					Name:  "test-container",
					Image: "nginx:1.23.0",
				},
			},
		},
		Status: kubelet.Status{
			Containers: []kubelet.ContainerStatus{
				{
					Name:    "test-container",
					ID:      "containerd://xyz789",
					Image:   "nginx:1.23.0",
					ImageID: "nginx@sha256:abc123",
				},
			},
		},
	}

	// Parse pods with the mock store
	events := ParseKubeletPods([]*kubelet.Pod{pod}, false, mockStore)
	var podEvent *workloadmeta.CollectorEvent
	for i := range events {
		if events[i].Entity.GetID().Kind == workloadmeta.KindKubernetesPod {
			podEvent = &events[i]
			break
		}
	}

	require.NotNil(t, podEvent, "pod event should be created")
	kubePod := podEvent.Entity.(*workloadmeta.KubernetesPod)

	// Assert that namespace labels/annotations were applied from cache
	assert.Equal(t, namespaceLabels, kubePod.NamespaceLabels, "namespace labels should be applied from cached metadata")
	assert.Equal(t, namespaceAnnotations, kubePod.NamespaceAnnotations, "namespace annotations should be applied from cached metadata")

	// Assert that pod's own labels and annotations are still present
	assert.Equal(t, podLabels, kubePod.Labels)
	assert.Equal(t, podAnnotations, kubePod.Annotations)
}
