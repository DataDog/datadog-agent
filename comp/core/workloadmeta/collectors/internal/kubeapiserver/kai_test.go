// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

func TestKAIPodGroupGVR(t *testing.T) {
	// KAI serves its Queues on v2, the group's preferred version, and its
	// PodGroups only on v2alpha2.
	discoveryClient := fakeclientset.NewSimpleClientset().Discovery().(*fakediscovery.FakeDiscovery)
	discoveryClient.Resources = []*metav1.APIResourceList{
		{GroupVersion: "scheduling.run.ai/v2", APIResources: []metav1.APIResource{{Name: "queues"}}},
		{GroupVersion: "scheduling.run.ai/v2alpha2", APIResources: []metav1.APIResource{{Name: "podgroups"}, {Name: "bindrequests"}}},
	}

	gvrs, err := getGVRsForRequestedResources(discoveryClient, kaiPodGroupGVRStrings())
	require.NoError(t, err)
	assert.Equal(t, []schema.GroupVersionResource{{Group: "scheduling.run.ai", Version: "v2alpha2", Resource: "podgroups"}}, gvrs)
}

func TestKAIPodGroupStore(t *testing.T) {
	wmeta := mockedWorkloadmeta(t)
	_, store := newKAIPodGroupStore(wmeta, nil, schema.GroupVersionResource{})

	newPodGroup := func(namespace, queue string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "pg",
				"namespace": namespace,
				"uid":       "uid-" + namespace,
			},
			"spec": map[string]interface{}{"queue": queue},
		}}
	}
	assertQueue := func(id, queue string) {
		t.Helper()
		podGroup, err := wmeta.GetKubernetesKAIPodGroup(id)
		require.NoError(t, err)
		assert.Equal(t, queue, podGroup.QueueName)
	}

	require.NoError(t, store.Add(newPodGroup("team-a", "queue-a")))
	require.NoError(t, store.Add(newPodGroup("team-b", "queue-b")))
	assertQueue("team-a/pg", "queue-a")
	assertQueue("team-b/pg", "queue-b")

	require.NoError(t, store.Update(newPodGroup("team-a", "queue-c")))
	assertQueue("team-a/pg", "queue-c")

	require.NoError(t, store.Delete(newPodGroup("team-a", "queue-c")))
	_, err := wmeta.GetKubernetesKAIPodGroup("team-a/pg")
	assert.Error(t, err)
	assertQueue("team-b/pg", "queue-b")

	// A relist that no longer contains the PodGroup removes it.
	require.NoError(t, store.Replace(nil, ""))
	_, err = wmeta.GetKubernetesKAIPodGroup("team-b/pg")
	assert.Error(t, err)
}
