// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"
	"testing"

	"github.com/ericchiang/k8s/api/v1"
	metav1 "github.com/ericchiang/k8s/apis/meta/v1"
	"github.com/stretchr/testify/require"
)

type podStruct struct {
	ip   string
	name string
}

type svcStruct struct {
	svcName string
	podIps  []string
}

func toPtr(str string) *string {
	strToptr := str
	return &strToptr
}

func createSvcList(nodeName string, svcs []svcStruct) v1.EndpointsList {
	var list v1.EndpointsList
	for _, svc := range svcs {
		var endpoints v1.Endpoints
		var endpointsSubset v1.EndpointSubset
		endpoints.Metadata = &metav1.ObjectMeta{}
		endpoints.Subsets = append(endpoints.Subsets, &endpointsSubset)
		endpoints.Metadata.Name = toPtr(svc.svcName)

		for _, e := range svc.podIps {
			var edpt v1.EndpointAddress
			edpt.NodeName = &nodeName
			edpt.Ip = toPtr(e)
			endpointsSubset.Addresses = append(endpointsSubset.Addresses, &edpt)
		}
		list.Items = append(list.Items, &endpoints)
	}
	return list
}

func createPodList(listPodStructs []podStruct) v1.PodList {
	var podlist v1.PodList
	for _, ps := range listPodStructs {
		var pod v1.Pod
		pod.Status = &v1.PodStatus{}
		pod.Metadata = &metav1.ObjectMeta{}
		pod.Status.PodIP = toPtr(ps.ip)
		pod.Metadata.Name = toPtr(ps.name)
		fmt.Printf("\n ps.name is %s, toPtr ps.name is %q and &ps.name is %q \n\n", ps.name, toPtr(ps.name), &ps.name)
		//fmt.Printf("current pod added is %s and pod list is %q \n", ps.name, podlist.Items)
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
	// Test 1 node 1 pod 1 service.
	smb := newServiceMapperBundle()

	node1 := createNode("firstNode")
	pod1 := podStruct{
		ip:   "1.1.1.1",
		name: "pod1_name",
	}
	svc1 := svcStruct{
		svcName: "svc1",
		podIps:  []string{"1.1.1.1"},
	}
	k8PodList1 := createPodList([]podStruct{pod1})
	k8EpList := createSvcList(*node1.Metadata.Name, []svcStruct{svc1})

	smb.mapServices(*node1.Metadata.Name, k8PodList1, k8EpList)

	res := make(map[string][]string)
	res[pod1.name] = []string{svc1.svcName}

	require.Equal(t, res, smb.PodNameToServices)

	// Test 3 nodes, 5 pods, 4 services
	smb1 := newServiceMapperBundle()

	node2 := createNode("secondNode")
	pod2 := podStruct{
		ip:   "2.2.2.2",
		name: "pod2_name",
	}
	pod3 := podStruct{
		ip:   "3.3.3.3",
		name: "pod3_name",
	}
	pod4 := podStruct{
		ip:   "4.4.4.4",
		name: "pod4_name",
	}
	pod5 := podStruct{
		ip:   "5.5.5.5",
		name: "pod5_name",
	}
	svc2 := svcStruct{
		svcName: "svc2",
		podIps: []string{
			"2.2.2.2",
		},
	}
	svc3 := svcStruct{
		svcName: "svc3",
		podIps: []string{
			"2.2.2.2",
			"5.5.5.5",
			"1.1.1.1",
		},
	}
	svc4 := svcStruct{
		svcName: "svc4",
		podIps: []string{
			"2.2.2.2",
			"3.3.3.3",
		},
	}
	k8PodList2 := createPodList([]podStruct{pod1, pod2, pod3, pod4})
	k8EpList2 := createSvcList(*node2.Metadata.Name, []svcStruct{svc1, svc3, svc4})

	fmt.Printf("input is pod list %q and ep list %q", k8PodList2, k8EpList2)
	smb1.mapServices(*node2.Metadata.Name, k8PodList2, k8EpList2) // Simulate that we evaluate the node2

	res2 := make(map[string][]string)
	res2[pod1.name] = []string{svc1.svcName, svc3.svcName}
	res2[pod2.name] = []string{svc3.svcName, svc4.svcName} // No SV2 as it's on pod2
	res2[pod3.name] = []string{svc4.svcName}
	fmt.Printf("smb is %q", smb1.PodNameToServices)
	require.Equal(t, res2, smb1.PodNameToServices) // Though pod4 and sv4 are being evaluated, as none of the svcs point to pod4, it won't be in the list.

	// We check that evaluating new services/pods on a given SMB (that would be gotten from the cache) maps as expected.
	k8PodList3 := createPodList([]podStruct{pod1, pod2, pod3, pod4, pod5})
	k8EpLis3 := createSvcList(*node2.Metadata.Name, []svcStruct{svc1, svc2, svc3, svc4})

	res2[pod5.name] = []string{svc3.svcName}
	res2[pod2.name] = []string{svc2.svcName, svc3.svcName, svc4.svcName} // SV2 as it's on pod2
	smb1.mapServices(*node2.Metadata.Name, k8PodList3, k8EpLis3)
	fmt.Printf("smb is %q", smb1.PodNameToServices)
	require.Equal(t, res2, smb1.PodNameToServices) // We can imagine a case
}
