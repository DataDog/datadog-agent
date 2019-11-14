// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package topologycollectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"
)

func TestInstanceIdExtractor(t *testing.T) {
	nodeSpecProviderId := "aws:///us-east-1b/i-024b28584ed2e6321"

	instanceId := extractInstanceIdFromProviderId(coreV1.NodeSpec{ProviderID: nodeSpecProviderId})
	assert.Equal(t, "i-024b28584ed2e6321", instanceId)
}

func TestNodeCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)
	relationChannel := make(chan *topology.Relation)
	defer close(relationChannel)
	nodeIdentifierCorrelationChannel := make(chan *NodeIdentifierCorrelation)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}

	ic := NewNodeCollector(componentChannel, relationChannel, nodeIdentifierCorrelationChannel, NewTestCommonClusterCollector(MockNodeAPICollectorClient{}))
	expectedCollectorName := "Node Collector"
	RunCollectorTest(t, ic, expectedCollectorName)

	for _, tc := range []struct {
		testCase   string
		assertions []func()
	}{
		{
			testCase: "Test Node 1 - NodeInternalIP",
			assertions: []func(){
				func() {
					component := <-componentChannel
					expectedComponent := &topology.Component{
						ExternalID: "urn:/kubernetes:test-cluster-name:node:test-node-1",
						Type:       topology.Type{Name: "node"},
						Data: topology.Data{
							"name":              "test-node-1",
							"creationTimestamp": creationTime,
							"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
							"namespace":         "test-namespace",
							"uid":               types.UID("test-node-1"),
							"status": NodeStatus{
								Phase: coreV1.NodeRunning,
								NodeInfo: coreV1.NodeSystemInfo{
									MachineID:               "test-machine-id-1",
									SystemUUID:              "",
									BootID:                  "",
									KernelVersion:           "4.19.0",
									OSImage:                 "",
									ContainerRuntimeVersion: "",
									KubeletVersion:          "",
									KubeProxyVersion:        "",
									OperatingSystem:         "",
									Architecture:            "x86_64",
								},
								KubeletEndpoint: coreV1.DaemonEndpoint{Port: 5000},
							},
							"identifiers": []string{"urn:ip:/test-cluster-name:test-node-1:10.20.01.01"},
						},
					}
					assert.EqualValues(t, expectedComponent, component)
				},
				func() {
					relation := <-relationChannel
					expectedRelation := &topology.Relation{
						ExternalID: "urn:/kubernetes:test-cluster-name:node:test-node-1->urn:cluster:/kubernetes:test-cluster-name",
						Type:       topology.Type{Name: "belongs_to"},
						SourceID:   "urn:/kubernetes:test-cluster-name:node:test-node-1",
						TargetID:   "urn:cluster:/kubernetes:test-cluster-name",
						Data:       map[string]interface{}{},
					}
					assert.EqualValues(t, expectedRelation, relation)
				},
			},
		},
		{
			testCase: "Test Node 2 - NodeInternalIP + NodeExternalIP + Kind + Generate Name",
			assertions: []func(){
				func() {
					component := <-componentChannel
					expectedComponent := &topology.Component{

						ExternalID: "urn:/kubernetes:test-cluster-name:node:test-node-2",
						Type:       topology.Type{Name: "node"},
						Data: topology.Data{
							"name":              "test-node-2",
							"creationTimestamp": creationTime,
							"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
							"namespace":         "test-namespace",
							"uid":               types.UID("test-node-2"),
							"status": NodeStatus{
								Phase: coreV1.NodeRunning,
								NodeInfo: coreV1.NodeSystemInfo{
									MachineID:     "test-machine-id-2",
									KernelVersion: "4.19.0",
									Architecture:  "x86_64",
								},
								KubeletEndpoint: coreV1.DaemonEndpoint{Port: 5000},
							},
							"identifiers":  []string{"urn:ip:/test-cluster-name:test-node-2:10.20.01.01", "urn:ip:/test-cluster-name:10.20.01.02"},
							"kind":         "some-specified-kind",
							"generateName": "some-specified-generation",
						},
					}
					assert.EqualValues(t, expectedComponent, component)
				},
				func() {
					relation := <-relationChannel
					expectedRelation := &topology.Relation{
						ExternalID: "urn:/kubernetes:test-cluster-name:node:test-node-2->urn:cluster:/kubernetes:test-cluster-name",
						Type:       topology.Type{Name: "belongs_to"},
						SourceID:   "urn:/kubernetes:test-cluster-name:node:test-node-2",
						TargetID:   "urn:cluster:/kubernetes:test-cluster-name",
						Data:       map[string]interface{}{},
					}
					assert.EqualValues(t, expectedRelation, relation)
				},
			},
		},
		{
			testCase: "Test Node 3 - Complete",
			assertions: []func(){
				func() {
					component := <-componentChannel
					expectedComponent := &topology.Component{
						ExternalID: "urn:/kubernetes:test-cluster-name:node:test-node-3",
						Type:       topology.Type{Name: "node"},
						Data: topology.Data{
							"name":              "test-node-3",
							"creationTimestamp": creationTime,
							"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
							"namespace":         "test-namespace",
							"uid":               types.UID("test-node-3"),
							"status": NodeStatus{
								Phase: coreV1.NodeRunning,
								NodeInfo: coreV1.NodeSystemInfo{
									MachineID:     "test-machine-id-3",
									KernelVersion: "4.19.0",
									Architecture:  "x86_64",
								},
								KubeletEndpoint: coreV1.DaemonEndpoint{Port: 5000},
							},
							"identifiers": []string{"urn:ip:/test-cluster-name:test-node-3:10.20.01.01", "urn:ip:/test-cluster-name:10.20.01.02",
								"urn:host:/test-cluster-name:cluster.internal.dns.test-node-3", "urn:host:/my-organization.test-node-3",
								"urn:host:/i-024b28584ed2e6321"},
							"kind":         "some-specified-kind",
							"generateName": "some-specified-generation",
							"instanceId":   "i-024b28584ed2e6321",
						},
					}
					assert.EqualValues(t, expectedComponent, component)
				},
				func() {

					relation := <-relationChannel
					expectedRelation := &topology.Relation{
						ExternalID: "urn:/kubernetes:test-cluster-name:node:test-node-3->urn:cluster:/kubernetes:test-cluster-name",
						Type:       topology.Type{Name: "belongs_to"},
						SourceID:   "urn:/kubernetes:test-cluster-name:node:test-node-3",
						TargetID:   "urn:cluster:/kubernetes:test-cluster-name",
						Data:       map[string]interface{}{},
					}
					assert.EqualValues(t, expectedRelation, relation)
				},
				func() {
					nodeIdentifier := <-nodeIdentifierCorrelationChannel
					expectedNodeIdentifier := &NodeIdentifierCorrelation{
						NodeName:       "test-node-3",
						NodeIdentifier: "i-024b28584ed2e6321",
					}
					assert.EqualValues(t, expectedNodeIdentifier, nodeIdentifier)
				},
			},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			for _, assertion := range tc.assertions {
				assertion()
			}
		})
	}
}

type MockNodeAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockNodeAPICollectorClient) GetNodes() ([]coreV1.Node, error) {
	nodes := make([]coreV1.Node, 0)
	for i := 1; i <= 3; i++ {
		node := coreV1.Node{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-node-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-node-%d", i)),
				GenerateName: "",
			},
			Status: coreV1.NodeStatus{
				Phase: coreV1.NodeRunning,
				NodeInfo: coreV1.NodeSystemInfo{
					MachineID:     fmt.Sprintf("test-machine-id-%d", i),
					KernelVersion: "4.19.0",
					Architecture:  "x86_64",
				},
				DaemonEndpoints: coreV1.NodeDaemonEndpoints{KubeletEndpoint: coreV1.DaemonEndpoint{Port: 5000}},
			},
		}

		if i == 1 {
			node.Status.Addresses = []coreV1.NodeAddress{
				{Type: coreV1.NodeInternalIP, Address: "10.20.01.01"},
			}
		}

		if i == 2 {
			node.TypeMeta.Kind = "some-specified-kind"
			node.ObjectMeta.GenerateName = "some-specified-generation"

			node.Status.Addresses = []coreV1.NodeAddress{
				{Type: coreV1.NodeInternalIP, Address: "10.20.01.01"},
				{Type: coreV1.NodeExternalIP, Address: "10.20.01.02"},
			}
		}

		if i == 3 {
			node.TypeMeta.Kind = "some-specified-kind"
			node.ObjectMeta.GenerateName = "some-specified-generation"
			node.Spec.ProviderID = "aws:///us-east-1b/i-024b28584ed2e6321"
			node.Status.Addresses = []coreV1.NodeAddress{
				{Type: coreV1.NodeInternalIP, Address: "10.20.01.01"},
				{Type: coreV1.NodeExternalIP, Address: "10.20.01.02"},
				{Type: coreV1.NodeInternalDNS, Address: "cluster.internal.dns.test-node-3"},
				{Type: coreV1.NodeExternalDNS, Address: "my-organization.test-node-3"},
			}
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}
