// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package topologycollectors

import (
	"fmt"
	"testing"
	"time"

	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestServiceCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)
	relationChannel := make(chan *topology.Relation)
	defer close(relationChannel)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}

	cjc := NewServiceCollector(componentChannel, relationChannel, NewTestCommonClusterCollector(MockServiceAPICollectorClient{}))
	// Mock out DNS resolution function for test
	cjc.(*ServiceCollector).DNS = func(name string) ([]string, error) {
		return []string{"10.10.42.42", "10.10.42.43"}, nil
	}
	expectedCollectorName := "Service Collector"
	RunCollectorTest(t, cjc, expectedCollectorName)

	for _, tc := range []struct {
		testCase           string
		expectedComponents []*topology.Component
		expectedRelations  []*topology.Relation
	}{
		{
			testCase: "Test Service 1 - Service + Pod Relation",
			expectedComponents: []*topology.Component{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-1",
					Type:       topology.Type{Name: "service"},
					Data: topology.Data{
						"name":              "test-service-1",
						"creationTimestamp": creationTime,
						"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace", "service-type": "ClusterIP"},
						"uid":               types.UID("test-service-1"),
						"identifiers":       []string{"urn:service:/test-cluster-name:test-namespace:test-service-1"},
					},
				},
			},
			expectedRelations: []*topology.Relation{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace->" +
						"urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-1",
					Type:     topology.Type{Name: "encloses"},
					SourceID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace",
					TargetID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-1",
					Data:     map[string]interface{}{},
				},
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-1->" +
						"urn:kubernetes:/test-cluster-name:pod-namespace:pod/some-pod-name",
					Type:     topology.Type{Name: "exposes"},
					SourceID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-1",
					TargetID: "urn:kubernetes:/test-cluster-name:pod-namespace:pod/some-pod-name",
					Data:     map[string]interface{}{},
				},
			},
		},
		{
			testCase: "Test Service 2 - Minimal - NodePort",
			expectedComponents: []*topology.Component{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-2",
					Type:       topology.Type{Name: "service"},
					Data: topology.Data{
						"name":              "test-service-2",
						"creationTimestamp": creationTime,
						"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace", "service-type": "NodePort"},
						"uid":               types.UID("test-service-2"),
						"identifiers": []string{
							"urn:endpoint:/test-cluster-name:10.100.200.20",
							"urn:endpoint:/test-cluster-name:10.100.200.20:10202",
							"urn:service:/test-cluster-name:test-namespace:test-service-2",
						},
					},
				},
			},
			expectedRelations: []*topology.Relation{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace->" +
						"urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-2",
					Type:     topology.Type{Name: "encloses"},
					SourceID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace",
					TargetID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-2",
					Data:     map[string]interface{}{},
				},
			},
		},
		{
			testCase: "Test Service 3 - Minimal - Cluster IP + External IPs",
			expectedComponents: []*topology.Component{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-3",
					Type:       topology.Type{Name: "service"},
					Data: topology.Data{
						"name":              "test-service-3",
						"creationTimestamp": creationTime,
						"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace", "service-type": "ClusterIP"},
						"uid":               types.UID("test-service-3"),
						"identifiers": []string{
							"urn:endpoint:/34.100.200.12:83", "urn:endpoint:/34.100.200.13:83",
							"urn:endpoint:/test-cluster-name:10.100.200.21",
							"urn:service:/test-cluster-name:test-namespace:test-service-3",
						},
					},
				},
			},
			expectedRelations: []*topology.Relation{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace->" +
						"urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-3",
					Type:     topology.Type{Name: "encloses"},
					SourceID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace",
					TargetID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-3",
					Data:     map[string]interface{}{},
				},
			},
		},
		{
			testCase: "Test Service 4 - Minimal - Cluster IP",
			expectedComponents: []*topology.Component{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-4",
					Type:       topology.Type{Name: "service"},
					Data: topology.Data{
						"name":              "test-service-4",
						"creationTimestamp": creationTime,
						"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace", "service-type": "ClusterIP"},
						"uid":               types.UID("test-service-4"),
						"identifiers": []string{
							"urn:endpoint:/test-cluster-name:10.100.200.22",
							"urn:service:/test-cluster-name:test-namespace:test-service-4",
						},
					},
				},
			},
			expectedRelations: []*topology.Relation{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace->" +
						"urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-4",
					Type:     topology.Type{Name: "encloses"},
					SourceID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace",
					TargetID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-4",
					Data:     map[string]interface{}{},
				},
			},
		},
		{
			testCase: "Test Service 5 - Minimal - Cluster IP - None",
			expectedComponents: []*topology.Component{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-5",
					Type:       topology.Type{Name: "service"},
					Data: topology.Data{
						"name":              "test-service-5",
						"creationTimestamp": creationTime,
						"tags": map[string]string{
							"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace", "service": "headless", "service-type": "ClusterIP",
						},
						"uid":         types.UID("test-service-5"),
						"identifiers": []string{"urn:service:/test-cluster-name:test-namespace:test-service-5"},
					},
				},
			},
			expectedRelations: []*topology.Relation{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace->" +
						"urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-5",
					Type:     topology.Type{Name: "encloses"},
					SourceID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace",
					TargetID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-5",
					Data:     map[string]interface{}{},
				},
			},
		},
		{
			testCase: "Test Service 6 - LoadBalancer + Ingress Points + Ingress Correlation",
			expectedComponents: []*topology.Component{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-6",
					Type:       topology.Type{Name: "service"},
					Data: topology.Data{
						"name":              "test-service-6",
						"creationTimestamp": creationTime,
						"tags": map[string]string{
							"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace", "service-type": "LoadBalancer",
						},
						"uid": types.UID("test-service-6"),
						"identifiers": []string{
							"urn:endpoint:/test-cluster-name:10.100.200.23", "urn:ingress-point:/34.100.200.15",
							"urn:ingress-point:/64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com",
							"urn:service:/test-cluster-name:test-namespace:test-service-6"},
					},
				},
			},
			expectedRelations: []*topology.Relation{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace->" +
						"urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-6",
					Type:     topology.Type{Name: "encloses"},
					SourceID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace",
					TargetID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-6",
					Data:     map[string]interface{}{},
				},
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-6->" +
						"urn:kubernetes:/test-cluster-name:pod-namespace:pod/some-pod-name",
					Type:     topology.Type{Name: "exposes"},
					SourceID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-6",
					TargetID: "urn:kubernetes:/test-cluster-name:pod-namespace:pod/some-pod-name",
					Data:     map[string]interface{}{},
				},
			},
		},
		{
			testCase: "Test Service 7 - ExternalName Service",
			expectedComponents: []*topology.Component{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-7",
					Type:       topology.Type{Name: "service"},
					Data: topology.Data{
						"name":              "test-service-7",
						"creationTimestamp": creationTime,
						"tags": map[string]string{
							"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace", "service-type": "ExternalName",
						},
						"uid":         types.UID("test-service-7"),
						"identifiers": []string{"urn:service:/test-cluster-name:test-namespace:test-service-7"},
					},
				},
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:external-service/test-service-7",
					Type:       topology.Type{Name: "external-service"},
					Data: topology.Data{
						"name":              "test-service-7",
						"creationTimestamp": creationTime,
						"tags": map[string]string{
							"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace",
						},
						"uid": types.UID("test-service-7"),
						"identifiers": []string{
							"urn:endpoint:/mysql-db.host.example.com",
							"urn:endpoint:/test-cluster-name:mysql-db.host.example.com:87",
							"urn:endpoint:/test-cluster-name:10.10.42.42",
							"urn:endpoint:/test-cluster-name:10.10.42.42:87",
							"urn:endpoint:/test-cluster-name:10.10.42.43",
							"urn:endpoint:/test-cluster-name:10.10.42.43:87",
							"urn:external-service:/test-cluster-name:test-namespace:test-service-7",
						},
					},
				},
			},
			expectedRelations: []*topology.Relation{
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-7->" +
						"urn:kubernetes:/test-cluster-name:test-namespace:external-service/test-service-7",
					Type:     topology.Type{Name: "uses"},
					SourceID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-7",
					TargetID: "urn:kubernetes:/test-cluster-name:test-namespace:external-service/test-service-7",
					Data:     map[string]interface{}{},
				},
				{
					ExternalID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace->" +
						"urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-7",
					Type:     topology.Type{Name: "encloses"},
					SourceID: "urn:kubernetes:/test-cluster-name:namespace/test-namespace",
					TargetID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-7",
					Data:     map[string]interface{}{},
				},
			},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			for _, expectedComponent := range tc.expectedComponents {
				component := <-componentChannel
				assert.EqualValues(t, expectedComponent, component)
			}

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
	for i := 1; i <= 7; i++ {

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
			service.Spec.Type = coreV1.ServiceTypeClusterIP
			service.Spec.ClusterIP = "None"
		}

		if i == 6 {
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

		if i == 7 {
			service.Spec.Type = coreV1.ServiceTypeExternalName
			service.Spec.Ports = []coreV1.ServicePort{
				{
					Name:       fmt.Sprintf("test-service-port-%d", i),
					Port:       int32(80 + i),
					TargetPort: intstr.FromInt(8080 + i),
				},
			}
			service.Spec.ExternalName = "mysql-db.host.example.com"
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
					{IP: "10.100.200.1", TargetRef: &coreV1.ObjectReference{Kind: "Pod", Name: "some-pod-name", Namespace: "pod-namespace"}},
				},
				Ports: []coreV1.EndpointPort{
					{Name: "", Port: int32(81)},
				},
			},
		},
	})

	// endpoints for test case 6
	endpoints = append(endpoints, coreV1.Endpoints{
		TypeMeta: v1.TypeMeta{
			Kind: "",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:              "test-service-6",
			CreationTimestamp: creationTime,
			Namespace:         "test-namespace",
			Labels: map[string]string{
				"test": "label",
			},
			UID:          "test-service-6",
			GenerateName: "",
		},
		Subsets: []coreV1.EndpointSubset{
			{
				Addresses: []coreV1.EndpointAddress{
					{IP: "10.100.200.2", TargetRef: &coreV1.ObjectReference{Kind: "Pod", Name: "some-pod-name", Namespace: "pod-namespace"}},
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
