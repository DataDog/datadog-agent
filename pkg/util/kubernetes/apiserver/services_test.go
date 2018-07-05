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
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type podTestDef struct {
	uid       string
	ip        string
	name      string
	namespace string
}

type endPointTestDef struct {
	name    string
	subsets [][]addressTestDef
}

type addressTestDef struct {
	ip          string
	nodeName    string
	targetPodId string
}

func mapPodTestDef(defs []podTestDef) map[string]podTestDef {
	mapped := make(map[string]podTestDef)
	for _, d := range defs {
		mapped[d.uid] = d
	}
	return mapped
}

func createEndpointList(nodeName string, defs []endPointTestDef, pods []podTestDef, noNodeName bool) v1.EndpointsList {
	var list v1.EndpointsList
	podsMap := mapPodTestDef(pods)

	for _, epDef := range defs {
		ep := v1.Endpoints{}
		ep.Name = epDef.name
		for _, subsetDef := range epDef.subsets {
			subset := v1.EndpointSubset{}
			for _, addrDef := range subsetDef {
				a := v1.EndpointAddress{}
				a.IP = addrDef.ip
				if !noNodeName { // Kubernetes 1.3.x does not list the nodeName
					a.NodeName = &addrDef.nodeName
				}
				pod, found := podsMap[addrDef.targetPodId]
				if found {
					a.TargetRef = &v1.ObjectReference{
						Kind:      "Pod",
						Namespace: pod.namespace,
						Name:      pod.name,
						UID:       types.UID(pod.uid),
					}
				}
				subset.Addresses = append(subset.Addresses, a)
			}
			ep.Subsets = append(ep.Subsets, subset)
		}
		list.Items = append(list.Items, ep)
	}
	return list
}

func createPodList(listPodStructs []podTestDef) v1.PodList {
	var podlist v1.PodList
	for _, ps := range listPodStructs {
		var pod v1.Pod
		pod.Status = v1.PodStatus{}
		pod.ObjectMeta = metav1.ObjectMeta{Namespace: ps.namespace}
		pod.Status.PodIP = ps.ip
		pod.Name = ps.name
		pod.UID = types.UID(ps.uid)
		podlist.Items = append(podlist.Items, pod)
	}

	return podlist
}

func createNode(nodeName string) v1.Node {
	var node v1.Node

	node.ObjectMeta = metav1.ObjectMeta{}
	node.Name = nodeName
	return node
}

type serviceMapTestCase struct {
	caseName        string
	node            v1.Node
	pods            []podTestDef
	endpoints       []endPointTestDef
	expectedMapping ServicesMapper
}

func TestMapServices(t *testing.T) {
	testCases := []serviceMapTestCase{
		{
			caseName: "1 node, 1 pod, 1 service",
			node:     createNode("firstNode"),
			pods: []podTestDef{
				{
					uid:       "1111",
					ip:        "1.1.1.1",
					name:      "pod1_name",
					namespace: "foo",
				},
			},
			endpoints: []endPointTestDef{
				{
					name: "svc1",
					subsets: [][]addressTestDef{
						{
							{
								ip:          "1.1.1.1",
								nodeName:    "firstNode",
								targetPodId: "1111",
							},
						},
					},
				},
			},
			expectedMapping: ServicesMapper{
				"foo": {"pod1_name": {"svc1"}},
			},
		},
		{
			caseName: "1 node, 2 pods with same name, 2 services",
			node:     createNode("firstNode"),
			pods: []podTestDef{
				{
					uid:       "1111",
					ip:        "1.1.1.1",
					name:      "pod_name",
					namespace: "foo",
				},
				{
					uid:       "2222",
					ip:        "2.2.2.2",
					name:      "pod_name",
					namespace: "bar",
				},
			},
			endpoints: []endPointTestDef{
				{
					name: "svc1",
					subsets: [][]addressTestDef{
						{
							{
								ip:          "1.1.1.1",
								nodeName:    "firstNode",
								targetPodId: "1111",
							},
						},
					},
				},
				{
					name: "svc2",
					subsets: [][]addressTestDef{
						{
							{
								ip:          "2.2.2.2",
								nodeName:    "firstNode",
								targetPodId: "2222",
							},
						},
					},
				},
			},
			expectedMapping: ServicesMapper{
				"foo": {"pod_name": {"svc1"}},
				"bar": {"pod_name": {"svc2"}},
			},
		},
		{
			caseName: "3 nodes, 4 pods, 3 services",
			node:     createNode("firstNode"),
			pods: []podTestDef{
				{
					uid:       "2222",
					ip:        "2.2.2.2",
					name:      "pod2_name",
					namespace: "foo",
				},
				{
					uid:       "3333",
					ip:        "3.3.3.3",
					name:      "pod3_name",
					namespace: "foo",
				},
				{
					uid:       "4444",
					ip:        "4.4.4.4",
					name:      "pod4_name",
					namespace: "foo",
				},
				{
					uid:       "5555",
					ip:        "5.5.5.5",
					name:      "pod5_name",
					namespace: "foo",
				},
			},
			endpoints: []endPointTestDef{
				{
					name: "svc2",
					subsets: [][]addressTestDef{
						{
							{
								ip:          "2.2.2.2",
								nodeName:    "firstNode",
								targetPodId: "2222",
							},
						},
					},
				},
				{
					name: "svc3",
					subsets: [][]addressTestDef{
						{
							{
								ip:          "2.2.2.2",
								nodeName:    "firstNode",
								targetPodId: "2222",
							},
							{
								ip:          "5.5.5.5",
								nodeName:    "firstNode",
								targetPodId: "5555",
							},
							{
								ip:          "1.1.1.1",
								nodeName:    "firstNode",
								targetPodId: "1111",
							},
						},
					},
				},
				{
					name: "svc4",
					subsets: [][]addressTestDef{
						{
							{
								ip:          "2.2.2.2",
								nodeName:    "firstNode",
								targetPodId: "2222",
							},
							{
								ip:          "3.3.3.3",
								nodeName:    "firstNode",
								targetPodId: "3333",
							},
						},
					},
				},
			},
			expectedMapping: ServicesMapper{
				"foo": {
					"pod2_name": {"svc2", "svc3", "svc4"},
					"pod3_name": {"svc4"},
					"pod5_name": {"svc3"},
				},
			},
		},
	}

	// Test the final state after all cases run to make
	// sure mapping does not affect unlisted services
	expectedAllPodNameToService := ServicesMapper{
		"foo": {
			"pod_name":  {"svc1"},
			"pod1_name": {"svc1"},
			"pod2_name": {"svc2", "svc3", "svc4"},
			"pod3_name": {"svc4"},
			"pod5_name": {"svc3"},
		},
		"bar": {
			"pod_name": {"svc2"},
		},
	}
	allCasesBundle := newMetadataMapperBundle()
	allBundleMu := &sync.RWMutex{}

	runCase := func(t *testing.T, tc serviceMapTestCase, noNodeName bool) {
		podList := createPodList(tc.pods)
		nodeName := tc.node.Name
		epList := createEndpointList(nodeName, tc.endpoints, tc.pods, noNodeName)

		if !noNodeName { // byIP would fail without the nodeName field, skipping
			byIPBundle := newMetadataMapperBundle()
			byIPBundle.mapOnIP = true
			byIPBundle.mapServices(nodeName, podList, epList)
			assert.Equal(t, tc.expectedMapping, byIPBundle.Services)
		}

		byRefBundle := newMetadataMapperBundle()
		byRefBundle.mapOnIP = false
		byRefBundle.mapServices(nodeName, podList, epList)
		assert.Equal(t, tc.expectedMapping, byRefBundle.Services)

		allBundleMu.Lock()
		allCasesBundle.mapServices(nodeName, podList, epList)
		allBundleMu.Unlock()
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("#%d %s - mapOnIp", i, tc.caseName), func(t *testing.T) {
			runCase(t, tc, false)
		})
		t.Run(fmt.Sprintf("#%d %s - mapOnRef", i, tc.caseName), func(t *testing.T) {
			runCase(t, tc, true)
		})
	}
	t.Run("Final state", func(t *testing.T) {
		allBundleMu.RLock()
		defer allBundleMu.RUnlock()
		assert.Equal(t, expectedAllPodNameToService, allCasesBundle.Services)
	})
}
