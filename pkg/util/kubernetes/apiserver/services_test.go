// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestServicesMapper(t *testing.T) {
	mapper := ServicesMapper{}

	mapper.Set("default", "pod1", "svc1")
	mapper.Set("default", "pod2", "svc1")
	mapper.Set("default", "pod3", "svc2")

	require.Equal(t, 3, len(mapper["default"]))
	assert.Equal(t, sets.NewString("svc1"), mapper["default"]["pod1"])

	mapper.Delete("default", "svc1")
	require.Equal(t, 1, len(mapper["default"]))
	assert.Equal(t, sets.NewString("svc2"), mapper["default"]["pod3"])

	mapper.Delete("default", "svc2")

	// No more pods in default namespace.
	_, ok := mapper["default"]
	require.False(t, ok, "default namespace still exists")
}

func TestMapServices(t *testing.T) {
	pod1 := newFakePod(
		"foo",
		"pod1_name",
		"1111",
		"1.1.1.1",
	)

	pod2 := newFakePod(
		"foo",
		"pod2_name",
		"2222",
		"2.2.2.2",
	)

	pod3 := newFakePod(
		"foo",
		"pod3_name",
		"3333",
		"3.3.3.3",
	)

	// These pods have the same name but are in different namespaces.
	defaultPod := newFakePod(
		"default",
		"pod_name",
		"1111",
		"1.1.1.1",
	)
	otherPod := newFakePod(
		"other",
		"pod_name",
		"2222",
		"2.2.2.2",
	)

	tests := []struct {
		desc            string
		nodeName        string
		pods            []v1.Pod
		endpoints       []v1.Endpoints
		expectedMapping ServicesMapper
	}{
		{
			"1 node, 1 pod, 1 service",
			"myNode",
			[]v1.Pod{pod1},
			[]v1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc1"},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								newFakeEndpointAddress("myNode", pod1),
							},
						},
					},
				},
			},
			ServicesMapper{
				"foo": {"pod1_name": sets.NewString("svc1")},
			},
		},
		{
			"1 node, 2 pods with same name, 2 services",
			"myNode",
			[]v1.Pod{defaultPod, otherPod},
			[]v1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc1"},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								newFakeEndpointAddress("myNode", defaultPod),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc2"},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								newFakeEndpointAddress("myNode", otherPod),
							},
						},
					},
				},
			},
			ServicesMapper{
				"default": {"pod_name": sets.NewString("svc1")},
				"other":   {"pod_name": sets.NewString("svc2")},
			},
		},
		{
			"endpoint for pod on different node",
			"myNode",
			[]v1.Pod{pod1, pod3},
			[]v1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc1"},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								newFakeEndpointAddress("myNode", pod1),
								// This pod is running on a different node and should not be
								// included in the expected mapping.
								newFakeEndpointAddress("otherNode", pod2),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc2"},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								newFakeEndpointAddress("myNode", pod3),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc3"},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								newFakeEndpointAddress("myNode", pod1),
								// This pod is running on a different node and should not be
								// included in the expected mapping.
								newFakeEndpointAddress("otherNode", pod2),
							},
						},
					},
				},
			},
			ServicesMapper{
				"foo": {
					"pod1_name": sets.NewString("svc1", "svc3"),
					"pod3_name": sets.NewString("svc2"),
				},
			},
		},
	}

	// Test the final state after all cases run to make
	// sure mapping does not affect unlisted services
	expectedAggregatedMapping := ServicesMapper{
		"foo": {
			"pod1_name": sets.NewString("svc1", "svc3"),
			"pod3_name": sets.NewString("svc2"),
		},
		"default": {
			"pod_name": sets.NewString("svc1"),
		},
		"other": {
			"pod_name": sets.NewString("svc2"),
		},
	}

	mu := sync.RWMutex{}
	var aggregatedBundle *MetadataMapperBundle

	aggregatedBundle = newMetadataMapperBundle()
	for i, tt := range tests {
		podList := v1.PodList{Items: tt.pods}
		endpointsList := v1.EndpointsList{Items: tt.endpoints}

		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			runMapOnIPTest(t, tt.nodeName, podList, endpointsList, tt.expectedMapping)
			runMapOnRefTest(t, tt.nodeName, podList, endpointsList, tt.expectedMapping)

			mu.Lock()
			defer mu.Unlock()

			err := aggregatedBundle.mapServices(tt.nodeName, podList, endpointsList)
			require.NoError(t, err)
		})
	}

	mu.RLock()
	assert.Equal(t, expectedAggregatedMapping, aggregatedBundle.Services)
	mu.RUnlock()

	// Run the tests again for legacy versions of Kubernetes
	aggregatedBundle = newMetadataMapperBundle()
	for i, tt := range tests {
		// Kubernetes 1.3.x does not include `NodeName`
		var legacyEndpoints []v1.Endpoints
		for _, endpoint := range tt.endpoints {
			for i, subset := range endpoint.Subsets {
				for i, address := range subset.Addresses {
					address.NodeName = nil
					subset.Addresses[i] = address
				}
				endpoint.Subsets[i] = subset
			}
			legacyEndpoints = append(legacyEndpoints, endpoint)
		}

		podList := v1.PodList{Items: tt.pods}
		endpointsList := v1.EndpointsList{Items: legacyEndpoints}

		t.Run(fmt.Sprintf("#%d %s/legacy", i, tt.desc), func(t *testing.T) {
			runMapOnRefTest(t, tt.nodeName, podList, endpointsList, tt.expectedMapping)

			mu.Lock()
			defer mu.Unlock()

			err := aggregatedBundle.mapServices(tt.nodeName, podList, endpointsList)
			require.NoError(t, err)
		})
	}

	mu.RLock()
	assert.Equal(t, expectedAggregatedMapping, aggregatedBundle.Services)
	mu.RUnlock()
}

func runMapOnRefTest(t *testing.T, nodeName string, podList v1.PodList, endpointsList v1.EndpointsList, expectedMapping ServicesMapper) {
	runMapServicesTest(t, nodeName, podList, endpointsList, expectedMapping, false)
}

func runMapOnIPTest(t *testing.T, nodeName string, podList v1.PodList, endpointsList v1.EndpointsList, expectedMapping ServicesMapper) {
	runMapServicesTest(t, nodeName, podList, endpointsList, expectedMapping, true)
}

func runMapServicesTest(t *testing.T, nodeName string, podList v1.PodList, endpointsList v1.EndpointsList, expectedMapping ServicesMapper, mapOnIP bool) {
	testName := "mapOnRef"
	if mapOnIP {
		testName = "mapOnIP"
	}
	t.Run(testName, func(t *testing.T) {
		bundle := newMetadataMapperBundle()
		bundle.mapOnIP = mapOnIP
		err := bundle.mapServices(nodeName, podList, endpointsList)
		require.NoError(t, err)
		assert.Equal(t, expectedMapping, bundle.Services)
	})
}

func newFakePod(namespace, name, uid, ip string) v1.Pod {
	return v1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
		Status: v1.PodStatus{PodIP: ip},
	}
}

func newFakeEndpointAddress(nodeName string, pod v1.Pod) v1.EndpointAddress {
	return v1.EndpointAddress{
		IP:       pod.Status.PodIP,
		NodeName: &nodeName,
		TargetRef: &v1.ObjectReference{
			Kind:      pod.Kind,
			Namespace: pod.Namespace,
			Name:      pod.Name,
			UID:       pod.UID,
		},
	}
}
