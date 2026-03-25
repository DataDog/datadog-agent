// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package kubemetadata

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestDCAPerPodProvider(t *testing.T) {
	pod := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: kubelet.Spec{NodeName: "node-1"},
	}

	t.Run("getKubernetesServices returns services", func(t *testing.T) {
		dcaClient := &FakeDCAClient{
			KubernetesMetadataNames: []string{"kube_service:service-1", "kube_service:service-2"},
		}
		provider := newDCAPerPodProvider(dcaClient, false, false)

		services := provider.getKubernetesServices(pod)
		assert.ElementsMatch(t, []string{"service-1", "service-2"}, services)
	})

	t.Run("getKubernetesServices returns empty on error", func(t *testing.T) {
		dcaClient := &FakeDCAClient{
			KubernetesMetadataNamesErr: errors.New("some error"),
		}
		provider := newDCAPerPodProvider(dcaClient, false, false)

		services := provider.getKubernetesServices(pod)
		assert.Empty(t, services)
	})

	t.Run("getNamespaceMetadata returns labels when enabled", func(t *testing.T) {
		dcaClient := &FakeDCAClient{
			NamespaceLabels: map[string]string{"label-1": "a"},
		}
		provider := newDCAPerPodProvider(dcaClient, true, false)

		labels, annotations := provider.getNamespaceMetadata("default")
		assert.Equal(t, map[string]string{"label-1": "a"}, labels)
		assert.Empty(t, annotations) // Not supported in this provider
	})

	t.Run("getNamespaceMetadata returns no labels when their collection is disabled", func(t *testing.T) {
		dcaClient := &FakeDCAClient{}
		provider := newDCAPerPodProvider(dcaClient, false, false)

		labels, annotations := provider.getNamespaceMetadata("default")
		assert.Empty(t, labels)
		assert.Empty(t, annotations) // Not supported in this provider
	})

	t.Run("getNamespaceMetadata returns empty annotations even when their collection is enabled", func(t *testing.T) {
		dcaClient := &FakeDCAClient{}
		provider := newDCAPerPodProvider(dcaClient, false, true)

		labels, annotations := provider.getNamespaceMetadata("default")
		assert.Empty(t, labels)
		assert.Empty(t, annotations) // Not supported in this provider
	})

	t.Run("getCollectedNamespaces returns namespaces seen via getNamespaceMetadata", func(t *testing.T) {
		dcaClient := &FakeDCAClient{
			NamespaceLabels: map[string]string{"label-1": "a"},
		}
		provider := newDCAPerPodProvider(dcaClient, true, false)

		assert.Empty(t, provider.getCollectedNamespaces())

		provider.getNamespaceMetadata("default")
		provider.getNamespaceMetadata("kube-system")

		collectedNamespaces := slices.Collect(maps.Keys(provider.getCollectedNamespaces()))
		assert.ElementsMatch(t, []string{"default", "kube-system"}, collectedNamespaces)
	})
}

func TestDCAPerNodeProvider(t *testing.T) {
	pod := &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: kubelet.Spec{NodeName: "node-1"},
	}

	t.Run("getKubernetesServices returns services", func(t *testing.T) {
		dcaClient := &FakeDCAClient{
			PodMetadataForNode: apiv1.NamespacesPodsStringsSet{
				"default": apiv1.MapStringSet{
					"foo": sets.New("service-1", "service-2"),
				},
			},
		}
		provider := newDCAPerNodeProvider("node-1", dcaClient, false, false)
		require.NoError(t, provider.prepare(nil))

		services := provider.getKubernetesServices(pod)
		assert.ElementsMatch(t, []string{"service-1", "service-2"}, services)
	})

	t.Run("getNamespaceMetadata returns labels when enabled", func(t *testing.T) {
		dcaClient := &FakeDCAClient{
			PodMetadataForNode: apiv1.NamespacesPodsStringsSet{},
			NamespaceLabels:    map[string]string{"label-1": "a"},
		}
		provider := newDCAPerNodeProvider("node-1", dcaClient, true, false)
		require.NoError(t, provider.prepare(nil))

		labels, annotations := provider.getNamespaceMetadata("default")
		assert.Equal(t, map[string]string{"label-1": "a"}, labels)
		assert.Empty(t, annotations) // Not supported in this provider
	})

	t.Run("getNamespaceMetadata returns no labels when their collection is disabled", func(t *testing.T) {
		dcaClient := &FakeDCAClient{
			PodMetadataForNode: apiv1.NamespacesPodsStringsSet{},
		}
		provider := newDCAPerNodeProvider("node-1", dcaClient, false, false)
		require.NoError(t, provider.prepare(nil))

		labels, annotations := provider.getNamespaceMetadata("default")
		assert.Empty(t, labels)
		assert.Empty(t, annotations) // Not supported in this provider
	})

	t.Run("getNamespaceMetadata returns empty annotations even when their collection is enabled", func(t *testing.T) {
		dcaClient := &FakeDCAClient{
			PodMetadataForNode: apiv1.NamespacesPodsStringsSet{},
		}
		provider := newDCAPerNodeProvider("node-1", dcaClient, false, true)
		require.NoError(t, provider.prepare(nil))

		labels, annotations := provider.getNamespaceMetadata("default")
		assert.Empty(t, labels)
		assert.Empty(t, annotations) // Not supported in this provider
	})

	t.Run("getCollectedNamespaces returns namespaces seen via getNamespaceMetadata", func(t *testing.T) {
		dcaClient := &FakeDCAClient{
			PodMetadataForNode: apiv1.NamespacesPodsStringsSet{},
			NamespaceLabels:    map[string]string{"label-1": "a"},
		}
		provider := newDCAPerNodeProvider("node-1", dcaClient, true, false)
		require.NoError(t, provider.prepare(nil))

		assert.Empty(t, provider.getCollectedNamespaces())

		provider.getNamespaceMetadata("default")
		provider.getNamespaceMetadata("kube-system")

		collectedNamespaces := slices.Collect(maps.Keys(provider.getCollectedNamespaces()))
		assert.ElementsMatch(t, []string{"default", "kube-system"}, collectedNamespaces)
	})
}

func TestDCAFullProvider(t *testing.T) {
	// First version that supports this provider
	dcaVersion := version.Version{Major: 7, Minor: 55}

	t.Run("getKubernetesServices returns services", func(t *testing.T) {
		dcaClient := &FakeDCAClient{
			LocalVersion: dcaVersion,
			PodMetadataForNode: apiv1.NamespacesPodsStringsSet{
				"default": apiv1.MapStringSet{
					"foo": sets.New("service-1"),
				},
			},
		}
		provider := newDCAFullProvider("node-1", dcaClient, false, false)
		require.NoError(t, provider.prepare(nil))

		pod := &kubelet.Pod{
			Metadata: kubelet.PodMetadata{Name: "foo", Namespace: "default"},
		}
		services := provider.getKubernetesServices(pod)
		assert.ElementsMatch(t, []string{"service-1"}, services)
	})

	t.Run("getNamespaceMetadata returns labels and annotations", func(t *testing.T) {
		dcaClient := &FakeDCAClient{
			LocalVersion: dcaVersion,
			NamespaceMetadata: clusteragent.Metadata{
				Labels:      map[string]string{"label-1": "a"},
				Annotations: map[string]string{"annotation-1": "a"},
			},
		}
		provider := newDCAFullProvider("node-1", dcaClient, true, true)
		require.NoError(t, provider.prepare(nil))

		labels, annotations := provider.getNamespaceMetadata("default")
		assert.Equal(t, map[string]string{"label-1": "a"}, labels)
		assert.Equal(t, map[string]string{"annotation-1": "a"}, annotations)
	})

	t.Run("getNamespaceMetadata returns empty labels/annotations when collection is disabled", func(t *testing.T) {
		dcaClient := &FakeDCAClient{
			LocalVersion: dcaVersion,
		}
		provider := newDCAFullProvider("node-1", dcaClient, false, false)
		require.NoError(t, provider.prepare(nil))

		labels, annotations := provider.getNamespaceMetadata("default")
		assert.Empty(t, labels)
		assert.Empty(t, annotations)
	})

	t.Run("getCollectedNamespaces returns namespaces seen via getNamespaceMetadata", func(t *testing.T) {
		dcaClient := &FakeDCAClient{
			LocalVersion: dcaVersion,
			NamespaceMetadata: clusteragent.Metadata{
				Labels:      map[string]string{"label-1": "a"},
				Annotations: map[string]string{"annotation-1": "a"},
			},
		}
		provider := newDCAFullProvider("node-1", dcaClient, true, true)
		require.NoError(t, provider.prepare(nil))

		assert.Empty(t, provider.getCollectedNamespaces())

		provider.getNamespaceMetadata("default")
		provider.getNamespaceMetadata("kube-system")

		collectedNamespaces := slices.Collect(maps.Keys(provider.getCollectedNamespaces()))
		assert.ElementsMatch(t, []string{"default", "kube-system"}, collectedNamespaces)
	})
}
