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
	"k8s.io/apimachinery/pkg/util/intstr"
	"testing"
	"time"
)

func TestServiceCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)
	relationChannel := make(chan *topology.Relation)
	defer close(relationChannel)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}

	cjc := NewServiceCollector(componentChannel, relationChannel, NewTestCommonClusterCollector(MockServiceAPICollectorClient{}))
	expectedCollectorName := "Service Collector"
	RunCollectorTest(t, cjc, expectedCollectorName)

	for _, tc := range []struct {
		testCase          string
		expectedComponent *topology.Component
		expectedRelations []*topology.Relation
	}{
		{
			testCase: "Test Service 1 - Service + Pod Relation",
			expectedComponent: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:service:test-namespace:test-service-1",
				Type:       topology.Type{Name: "service"},
				Data: topology.Data{
					"name":              "test-service-1",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-service-1"),
					"identifiers":       []string{"urn:endpoint:/test-cluster-name:10.100.200.1:81"},
				},
			},
			expectedRelations: []*topology.Relation{
				{
					ExternalID: "urn:/kubernetes:test-cluster-name:service:test-namespace:test-service-1->urn:/kubernetes:test-cluster-name:pod:some-pod-name",
					Type:       topology.Type{Name: "exposes"},
					SourceID:   "urn:/kubernetes:test-cluster-name:service:test-namespace:test-service-1",
					TargetID:   "urn:/kubernetes:test-cluster-name:pod:some-pod-name",
					Data:       map[string]interface{}{},
				},
			},
		},
		{
			testCase: "Test Service 2 - Minimal - NodePort",
			expectedComponent: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:service:test-namespace:test-service-2",
				Type:       topology.Type{Name: "service"},
				Data: topology.Data{
					"name":              "test-service-2",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-service-2"),
					"identifiers":       []string{"urn:endpoint:/test-cluster-name:10.100.200.20", "urn:endpoint:/test-cluster-name:10.100.200.20:10202"},
				},
			},
			expectedRelations: []*topology.Relation{},
		},
		{
			testCase: "Test Service 3 - Minimal - Cluster IP + External IPs",
			expectedComponent: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:service:test-namespace:test-service-3",
				Type:       topology.Type{Name: "service"},
				Data: topology.Data{
					"name":              "test-service-3",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-service-3"),
					"identifiers":       []string{"urn:endpoint:/34.100.200.12:83", "urn:endpoint:/34.100.200.13:83", "urn:endpoint:/test-cluster-name:10.100.200.21"},
				},
			},
			expectedRelations: []*topology.Relation{},
		},
		{
			testCase: "Test Service 4 - Minimal - Cluster IP",
			expectedComponent: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:service:test-namespace:test-service-4",
				Type:       topology.Type{Name: "service"},
				Data: topology.Data{
					"name":              "test-service-4",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-service-4"),
					"identifiers":       []string{"urn:endpoint:/test-cluster-name:10.100.200.22"},
				},
			},
			expectedRelations: []*topology.Relation{},
		},
		{
			testCase: "Test Service 5 - LoadBalancer + Ingress Points + Ingress Correlation",
			expectedComponent: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:service:test-namespace:test-service-5",
				Type:       topology.Type{Name: "service"},
				Data: topology.Data{
					"name":              "test-service-5",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-service-5"),
					"identifiers": []string{"urn:endpoint:/test-cluster-name:10.100.200.2:85", "urn:endpoint:/test-cluster-name:10.100.200.2:10205",
						"urn:endpoint:/test-cluster-name:10.100.200.23", "urn:ingress-point:/34.100.200.15",
						"urn:ingress-point:/64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com"},
				},
			},
			expectedRelations: []*topology.Relation{
				{
					ExternalID: "urn:/kubernetes:test-cluster-name:service:test-namespace:test-service-5->urn:/kubernetes:test-cluster-name:pod:some-pod-name",
					Type:       topology.Type{Name: "exposes"},
					SourceID:   "urn:/kubernetes:test-cluster-name:service:test-namespace:test-service-5",
					TargetID:   "urn:/kubernetes:test-cluster-name:pod:some-pod-name",
					Data:       map[string]interface{}{},
				},
			},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			service := <-componentChannel
			assert.EqualValues(t, tc.expectedComponent, service)

			for _, expectedRelation := range tc.expectedRelations {
				serviceRelation := <-relationChannel
				assert.EqualValues(t, expectedRelation, serviceRelation)
			}
		})
	}
}

type MockServiceAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockServiceAPICollectorClient) GetServices() ([]coreV1.Service, error) {
	services := make([]coreV1.Service, 0)
	for i := 1; i <= 5; i++ {

		service := coreV1.Service{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-service-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-service-%d", i)),
				GenerateName: "",
			},
			Spec: coreV1.ServiceSpec{
				Ports: []coreV1.ServicePort{
					{Name: fmt.Sprintf("test-service-port-%d", i), Port: int32(80 + i), TargetPort: intstr.FromInt(8080 + i)},
				},
				Type: coreV1.ServiceTypeClusterIP,
			},
		}

		if i == 2 {
			service.Spec.Type = coreV1.ServiceTypeNodePort
			service.Spec.Ports = []coreV1.ServicePort{
				{
					Name:       fmt.Sprintf("test-service-node-port-%d", i),
					Port:       int32(80 + i),
					TargetPort: intstr.FromInt(8080 + i),
					NodePort:   int32(10200 + i),
				},
			}
			service.Spec.ClusterIP = "10.100.200.20"
		}
		if i == 3 {
			service.Spec.Type = coreV1.ServiceTypeClusterIP
			service.Spec.ExternalIPs = []string{"34.100.200.12", "34.100.200.13"}
			service.Spec.ClusterIP = "10.100.200.21"
		}

		if i == 4 {
			service.Spec.Type = coreV1.ServiceTypeClusterIP
			service.Spec.ClusterIP = "10.100.200.22"
		}

		if i == 5 {
			service.Spec.Type = coreV1.ServiceTypeLoadBalancer
			service.Spec.Ports = []coreV1.ServicePort{
				{
					Name:       fmt.Sprintf("test-service-port-%d", i),
					Port:       int32(80 + i),
					TargetPort: intstr.FromInt(8080 + i),
				},
				{
					Name:       fmt.Sprintf("test-service-node-port-%d", i),
					Port:       int32(80 + i),
					TargetPort: intstr.FromInt(8080 + i),
					NodePort:   int32(10200 + i),
				},
			}
			service.Status.LoadBalancer = coreV1.LoadBalancerStatus{
				Ingress: []coreV1.LoadBalancerIngress{
					{IP: "34.100.200.15"},
					{Hostname: "64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com"},
				},
			}
			service.Spec.LoadBalancerIP = "10.100.200.23"
		}

		services = append(services, service)
	}

	return services, nil
}

func (m MockServiceAPICollectorClient) GetEndpoints() ([]coreV1.Endpoints, error) {
	endpoints := make([]coreV1.Endpoints, 0)
	// endpoints for test case 1
	endpoints = append(endpoints, coreV1.Endpoints{
		TypeMeta: v1.TypeMeta{
			Kind: "",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:              "test-service-1",
			CreationTimestamp: creationTime,
			Namespace:         "test-namespace",
			Labels: map[string]string{
				"test": "label",
			},
			UID:          types.UID("test-service-1"),
			GenerateName: "",
		},
		Subsets: []coreV1.EndpointSubset{
			{
				Addresses: []coreV1.EndpointAddress{
					{IP: "10.100.200.1", TargetRef: &coreV1.ObjectReference{Kind: "Pod", Name: "some-pod-name"}},
				},
				Ports: []coreV1.EndpointPort{
					{Name: "", Port: int32(81)},
				},
			},
		},
	})

	// endpoints for test case 5
	endpoints = append(endpoints, coreV1.Endpoints{
		TypeMeta: v1.TypeMeta{
			Kind: "",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:              "test-service-5",
			CreationTimestamp: creationTime,
			Namespace:         "test-namespace",
			Labels: map[string]string{
				"test": "label",
			},
			UID:          types.UID("test-service-5"),
			GenerateName: "",
		},
		Subsets: []coreV1.EndpointSubset{
			{
				Addresses: []coreV1.EndpointAddress{
					{IP: "10.100.200.2", TargetRef: &coreV1.ObjectReference{Kind: "Pod", Name: "some-pod-name"}},
				},
				Ports: []coreV1.EndpointPort{
					{Name: "Endpoint Port", Port: int32(85)},
					{Name: "Endpoint NodePort", Port: int32(10205)},
				},
			},
		},
	})

	return endpoints, nil
}
