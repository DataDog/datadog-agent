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

	"github.com/ericchiang/k8s/api/v1"
	metav1 "github.com/ericchiang/k8s/apis/meta/v1"
	"github.com/stretchr/testify/assert"
)

type podTest struct {
	ip   string
	name string
}

type serviceTest struct {
	svcName string
	podIps  []string
}

func toPtr(str string) *string {
	return &str
}

func createSvcList(nodeName string, svcs []serviceTest) v1.EndpointsList {
	var list v1.EndpointsList
	for _, svc := range svcs {
		var endpoints v1.Endpoints
		var endpointsSubset v1.EndpointSubset
		endpoints.Metadata = &metav1.ObjectMeta{}
		endpoints.Subsets = append(endpoints.Subsets, &endpointsSubset)
		endpoints.Metadata.Name = toPtr(svc.svcName)

		for _, e := range svc.podIps {
			var ep v1.EndpointAddress
			ep.NodeName = &nodeName
			ep.Ip = toPtr(e)
			endpointsSubset.Addresses = append(endpointsSubset.Addresses, &ep)
		}
		list.Items = append(list.Items, &endpoints)
	}
	return list
}

func createPodList(listPodStructs []podTest) v1.PodList {
	var podlist v1.PodList
	for _, ps := range listPodStructs {
		var pod v1.Pod
		pod.Status = &v1.PodStatus{}
		pod.Metadata = &metav1.ObjectMeta{}
		pod.Status.PodIP = toPtr(ps.ip)
		pod.Metadata.Name = toPtr(ps.name)
		podlist.Items = append(podlist.Items, &pod)
	}

	return podlist
}
func createNode(nodeName string) v1.Node {
	var node v1.Node

	node.Metadata = &metav1.ObjectMeta{}
	node.Metadata.Name = &nodeName
	return node
}

func TestMapServices(t *testing.T) {
	testCases := []struct {
		caseName        string
		node            v1.Node
		pods            []podTest
		services        []serviceTest
		expectedMapping map[string][]string
	}{
		{
			caseName: "1 node, 1 pod, 1 service",
			node:     createNode("firstNode"),
			pods: []podTest{
				{
					ip:   "1.1.1.1",
					name: "pod1_name",
				},
			},
			services: []serviceTest{
				{
					svcName: "svc1",
					podIps:  []string{"1.1.1.1"},
				},
			},
			expectedMapping: map[string][]string{
				"pod1_name": {"svc1"},
			},
		},
		{
			caseName: "3 nodes, 4 pods, 3 services",
			node:     createNode("firstNode"),
			pods: []podTest{
				{
					ip:   "2.2.2.2",
					name: "pod2_name",
				},
				{
					ip:   "3.3.3.3",
					name: "pod3_name",
				},
				{
					ip:   "4.4.4.4",
					name: "pod4_name",
				},
				{
					ip:   "5.5.5.5",
					name: "pod5_name",
				},
			},
			services: []serviceTest{
				{
					svcName: "svc2",
					podIps:  []string{"2.2.2.2"},
				},
				{
					svcName: "svc3",
					podIps: []string{
						"2.2.2.2",
						"5.5.5.5",
						"1.1.1.1",
					},
				},
				{
					svcName: "svc4",
					podIps: []string{
						"2.2.2.2",
						"3.3.3.3",
					},
				},
			},
			expectedMapping: map[string][]string{
				"pod2_name": {"svc2", "svc3", "svc4"},
				"pod3_name": {"svc4"},
				"pod5_name": {"svc3"},
			},
		},
	}
	expectedAllPodNameToServices := map[string][]string{
		"pod1_name": {"svc1"},
		"pod2_name": {"svc2", "svc3", "svc4"},
		"pod3_name": {"svc4"},
		"pod5_name": {"svc3"},
	}
	allCasesBundle := newServiceMapperBundle()
	allBundleMu := &sync.RWMutex{}
	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("#%d %s", i, testCase.caseName), func(t *testing.T) {
			testCaseBundle := newServiceMapperBundle()
			podList := createPodList(testCase.pods)
			nodeName := *testCase.node.Metadata.Name
			epList := createSvcList(nodeName, testCase.services)
			testCaseBundle.mapServices(nodeName, podList, epList)
			assert.Equal(t, testCase.expectedMapping, testCaseBundle.PodNameToServices)
			allBundleMu.Lock()
			allCasesBundle.mapServices(nodeName, podList, epList)
			allBundleMu.Unlock()
		})
	}
	allBundleMu.RLock()
	defer allBundleMu.RUnlock()
	assert.Equal(t, expectedAllPodNameToServices, allCasesBundle.PodNameToServices)
}
