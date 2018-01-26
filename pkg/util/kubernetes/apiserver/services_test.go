// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver
package apiserver

import (
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

func createSvcList(nodeName string, svc svcStruct) v1.EndpointsList {
	var list v1.EndpointsList
	var endpoints v1.Endpoints
	var endpointsSubset v1.EndpointSubset
	var edpt v1.EndpointAddress

	endpoints.Metadata = &metav1.ObjectMeta{}

	endpoints.Subsets = append(endpoints.Subsets, &endpointsSubset)

	endpoints.Metadata.Name = &svc.svcName
	edpt.NodeName = &nodeName

	for _, e := range svc.podIps {
		edpt.Ip = &e
		endpointsSubset.Addresses = append(endpointsSubset.Addresses, &edpt)
	}

	list.Items = append(list.Items, &endpoints)
	return list
}

func createPodList(listPodStructs []podStruct) v1.PodList {
	var podlist v1.PodList
	var pod v1.Pod
	pod.Status = &v1.PodStatus{}
	pod.Metadata = &metav1.ObjectMeta{}
	for _, ps := range listPodStructs {
		pod.Status.PodIP = &ps.ip
		pod.Metadata.Name = &ps.name
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
		ip:   "1.2.3.4",
		name: "foo",
	}
	svc := svcStruct{
		svcName: "svc1",
		podIps: []string{
			"1.2.3.4",
		},
	}
	k8PodList1 := createPodList([]podStruct{pod1})
	k8EpList := createSvcList(*node1.Metadata.Name, svc)

	smb.mapServices(*node1.Metadata.Name, k8PodList1, k8EpList)
	res := make(map[string][]string)
	res[pod1.name] = []string{svc.svcName}
	require.Equal(t, res, smb.PodNameToServices)

	// Test 3 nodes, 5 pods, 4 services
	// smb := newServiceMapperBundle()

	// node1 := createNode("firstNode")
	// node2 := createNode("firstNode")
	// node2 := createNode("firstNode")
	// pod1 := podStruct{
	// 	ip:   "1.2.3.4",
	// 	name: "foo",
	// }
	// svc := svcStruct{
	// 	svcName: "svc1",
	// 	podIps: []string{
	// 		"1.2.3.4",
	// 	},
	// }
	// k8PodList1 := createPodList([]podStruct{pod1})
	// k8EpList := createSvcList(*node1.Metadata.Name, svc)

	// node2 := createNode("secondNode")
	// pod2 := podStruct{
	// 	ip: '0.0.0.0',
	// 	name: 'bar'
	// }
	// pod3 := podStruct{
	// 	ip: '0.1.0.1',
	// 	name: 'baz'
	// }
	// k8PodList2 := createPodList([]podStruct{pod2,pod3})
}
