// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package apiserver

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

func TestMapServices(t *testing.T) {
	pod1 := newFakeKubeletPod(
		"foo",
		"pod1_name",
		"1111",
		"1.1.1.1",
	)

	pod2 := newFakeKubeletPod(
		"foo",
		"pod2_name",
		"2222",
		"2.2.2.2",
	)

	pod3 := newFakeKubeletPod(
		"foo",
		"pod3_name",
		"3333",
		"3.3.3.3",
	)

	// These pods have the same name but are in different namespaces.
	defaultPod := newFakeKubeletPod(
		"default",
		"pod_name",
		"1111",
		"1.1.1.1",
	)
	otherPod := newFakeKubeletPod(
		"other",
		"pod_name",
		"2222",
		"2.2.2.2",
	)

	tests := []struct {
		desc            string
		nodeName        string
		kubeletPods     []*kubelet.Pod
		endpoints       []v1.Endpoints
		expectedMapping apiv1.NamespacesPodsStringsSet
	}{
		{
			"1 node, 1 pod, 1 service",
			"myNode",
			[]*kubelet.Pod{pod1},
			[]v1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc1"},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								newFakeKubeletPodEndpointAddress("myNode", pod1),
							},
						},
					},
				},
			},
			apiv1.NamespacesPodsStringsSet{
				"foo": {"pod1_name": sets.New("svc1")},
			},
		},
		{
			"1 node, 2 pods with same name, 2 services",
			"myNode",
			[]*kubelet.Pod{defaultPod, otherPod},
			[]v1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc1"},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								newFakeKubeletPodEndpointAddress("myNode", defaultPod),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc2"},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								newFakeKubeletPodEndpointAddress("myNode", otherPod),
							},
						},
					},
				},
			},
			apiv1.NamespacesPodsStringsSet{
				"default": {"pod_name": sets.New("svc1")},
				"other":   {"pod_name": sets.New("svc2")},
			},
		},
		{
			"endpoint for pod on different node",
			"myNode",
			[]*kubelet.Pod{pod1, pod3},
			[]v1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc1"},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								newFakeKubeletPodEndpointAddress("myNode", pod1),
								// This pod is running on a different node and should not be
								// included in the expected mapping.
								newFakeKubeletPodEndpointAddress("otherNode", pod2),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc2"},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								newFakeKubeletPodEndpointAddress("myNode", pod3),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "svc3"},
					Subsets: []v1.EndpointSubset{
						{
							Addresses: []v1.EndpointAddress{
								newFakeKubeletPodEndpointAddress("myNode", pod1),
								// This pod is running on a different node and should not be
								// included in the expected mapping.
								newFakeKubeletPodEndpointAddress("otherNode", pod2),
							},
						},
					},
				},
			},
			apiv1.NamespacesPodsStringsSet{
				"foo": {
					"pod1_name": sets.New("svc1", "svc3"),
					"pod3_name": sets.New("svc2"),
				},
			},
		},
	}

	// Test the final state after all cases run to make
	// sure mapping does not affect unlisted services
	expectedAggregatedMapping := apiv1.NamespacesPodsStringsSet{
		"foo": {
			"pod1_name": sets.New("svc1", "svc3"),
			"pod3_name": sets.New("svc2"),
		},
		"default": {
			"pod_name": sets.New("svc1"),
		},
		"other": {
			"pod_name": sets.New("svc2"),
		},
	}

	mu := sync.RWMutex{}
	var aggregatedBundle *metadataMapperBundle

	aggregatedBundle = newMetadataMapperBundle()
	for i, tt := range tests {
		endpointsList := v1.EndpointsList{Items: tt.endpoints}

		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			runMapOnIPTest(t, tt.nodeName, tt.kubeletPods, endpointsList, tt.expectedMapping)
			runMapOnRefTest(t, tt.nodeName, tt.kubeletPods, endpointsList, tt.expectedMapping)

			mu.Lock()
			defer mu.Unlock()

			err := aggregatedBundle.mapServices(tt.nodeName, tt.kubeletPods, endpointsList)
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

		endpointsList := v1.EndpointsList{Items: legacyEndpoints}

		t.Run(fmt.Sprintf("#%d %s/legacy", i, tt.desc), func(t *testing.T) {
			runMapOnRefTest(t, tt.nodeName, tt.kubeletPods, endpointsList, tt.expectedMapping)

			mu.Lock()
			defer mu.Unlock()

			err := aggregatedBundle.mapServices(tt.nodeName, tt.kubeletPods, endpointsList)
			require.NoError(t, err)
		})
	}

	mu.RLock()
	assert.Equal(t, expectedAggregatedMapping, aggregatedBundle.Services)
	mu.RUnlock()
}

func runMapOnRefTest(t *testing.T, nodeName string, pods []*kubelet.Pod, endpointsList v1.EndpointsList, expectedMapping apiv1.NamespacesPodsStringsSet) {
	runMapServicesTest(t, nodeName, pods, endpointsList, expectedMapping, false)
}

func runMapOnIPTest(t *testing.T, nodeName string, pods []*kubelet.Pod, endpointsList v1.EndpointsList, expectedMapping apiv1.NamespacesPodsStringsSet) {
	runMapServicesTest(t, nodeName, pods, endpointsList, expectedMapping, true)
}

func runMapServicesTest(t *testing.T, nodeName string, pods []*kubelet.Pod, endpointsList v1.EndpointsList, expectedMapping apiv1.NamespacesPodsStringsSet, mapOnIP bool) {
	testName := "mapOnRef"
	if mapOnIP {
		testName = "mapOnIP"
	}
	t.Run(testName, func(t *testing.T) {
		bundle := newMetadataMapperBundle()
		bundle.mapOnIP = mapOnIP
		err := bundle.mapServices(nodeName, pods, endpointsList)
		require.NoError(t, err)
		assert.Equal(t, expectedMapping, bundle.Services)
	})
}

func newFakeKubeletPod(namespace, name, uid, ip string) *kubelet.Pod {
	return &kubelet.Pod{
		Metadata: kubelet.PodMetadata{
			Name:      name,
			Namespace: namespace,
			UID:       uid,
		},
		Status: kubelet.Status{PodIP: ip},
	}
}

func newFakeKubeletPodEndpointAddress(nodeName string, pod *kubelet.Pod) v1.EndpointAddress {
	return v1.EndpointAddress{
		IP:       pod.Status.PodIP,
		NodeName: &nodeName,
		TargetRef: &v1.ObjectReference{
			Kind:      "Pod",
			Namespace: pod.Metadata.Namespace,
			Name:      pod.Metadata.Name,
			UID:       types.UID(pod.Metadata.UID),
		},
	}
}
