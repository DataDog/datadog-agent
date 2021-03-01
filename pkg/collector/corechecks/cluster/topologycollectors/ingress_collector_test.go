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
	"k8s.io/api/extensions/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestIngressCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)
	relationChannel := make(chan *topology.Relation)
	defer close(relationChannel)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}

	ic := NewIngressCollector(componentChannel, relationChannel, NewTestCommonClusterCollector(MockIngressAPICollectorClient{}))
	expectedCollectorName := "Ingress Collector"
	RunCollectorTest(t, ic, expectedCollectorName)

	for _, tc := range []struct {
		testCase   string
		assertions []func(*testing.T, chan *topology.Component, chan *topology.Relation)
	}{
		{
			testCase: "Test Service 1 - Minimal",
			assertions: []func(*testing.T, chan *topology.Component, chan *topology.Relation){
				expectComponent(&topology.Component{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-1",
					Type:       topology.Type{Name: "ingress"},
					Data: topology.Data{
						"name":              "test-ingress-1",
						"creationTimestamp": creationTime,
						"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace"},
						"uid":               types.UID("test-ingress-1"),
						"identifiers":       []string{},
					},
				}),
				expectComponent(&topology.Component{
					ExternalID: "urn:endpoint:/test-cluster-name:34.100.200.15",
					Type:       topology.Type{Name: "endpoint"},
					Data: topology.Data{
						"name":              "34.100.200.15",
						"creationTimestamp": creationTime,
						"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace"},
						"identifiers":       []string{},
					},
				}),
				expectRelation(&topology.Relation{
					ExternalID: "urn:endpoint:/test-cluster-name:34.100.200.15->urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-1",
					SourceID:   "urn:endpoint:/test-cluster-name:34.100.200.15",
					TargetID:   "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-1",
					Type:       topology.Type{Name: "routes"},
					Data:       map[string]interface{}{},
				}),
				expectComponent(&topology.Component{
					ExternalID: "urn:endpoint:/test-cluster-name:64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com",
					Type:       topology.Type{Name: "endpoint"},
					Data: topology.Data{
						"name":              "64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com",
						"creationTimestamp": creationTime,
						"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace"},
						"identifiers":       []string{},
					},
				}),
				expectRelation(&topology.Relation{
					ExternalID: "urn:endpoint:/test-cluster-name:64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com->urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-1",
					SourceID:   "urn:endpoint:/test-cluster-name:64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com",
					TargetID:   "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-1",
					Type:       topology.Type{Name: "routes"},
					Data:       map[string]interface{}{},
				}),
			},
		},
		{
			testCase: "Test Service 2 - Default Backend",
			assertions: []func(*testing.T, chan *topology.Component, chan *topology.Relation){
				expectComponent(&topology.Component{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-2",
					Type:       topology.Type{Name: "ingress"},
					Data: topology.Data{
						"name":              "test-ingress-2",
						"creationTimestamp": creationTime,
						"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace"},
						"uid":               types.UID("test-ingress-2"),
						"identifiers":       []string{},
					},
				}),
				expectRelation(&topology.Relation{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-2->" +
						"urn:kubernetes:/test-cluster-name:test-namespace:service/test-service",
					Type:     topology.Type{Name: "routes"},
					SourceID: "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-2",
					TargetID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service",
					Data:     map[string]interface{}{},
				}),
				expectComponent(&topology.Component{
					ExternalID: "urn:endpoint:/test-cluster-name:34.100.200.15",
					Type:       topology.Type{Name: "endpoint"},
					Data: topology.Data{
						"name":              "34.100.200.15",
						"creationTimestamp": creationTime,
						"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace"},
						"identifiers":       []string{},
					},
				}),
				expectRelation(&topology.Relation{
					ExternalID: "urn:endpoint:/test-cluster-name:34.100.200.15->urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-2",
					SourceID:   "urn:endpoint:/test-cluster-name:34.100.200.15",
					TargetID:   "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-2",
					Type:       topology.Type{Name: "routes"},
					Data:       map[string]interface{}{},
				}),
				expectComponent(&topology.Component{
					ExternalID: "urn:endpoint:/test-cluster-name:64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com",
					Type:       topology.Type{Name: "endpoint"},
					Data: topology.Data{
						"name":              "64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com",
						"creationTimestamp": creationTime,
						"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace"},
						"identifiers":       []string{},
					},
				}),
				expectRelation(&topology.Relation{
					ExternalID: "urn:endpoint:/test-cluster-name:64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com->urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-2",
					SourceID:   "urn:endpoint:/test-cluster-name:64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com",
					TargetID:   "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-2",
					Type:       topology.Type{Name: "routes"},
					Data:       map[string]interface{}{},
				}),
			},
		},
		{
			testCase: "Test Service 3 - Ingress Rules",
			assertions: []func(*testing.T, chan *topology.Component, chan *topology.Relation){
				expectComponent(&topology.Component{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-3",
					Type:       topology.Type{Name: "ingress"},
					Data: topology.Data{
						"name":              "test-ingress-3",
						"creationTimestamp": creationTime,
						"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace"},
						"uid":               types.UID("test-ingress-3"),
						"kind":              "some-specified-kind",
						"generateName":      "some-specified-generation",
						"identifiers":       []string{},
					},
				}),
				expectRelation(&topology.Relation{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-3->" +
						"urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-1",
					Type:     topology.Type{Name: "routes"},
					SourceID: "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-3",
					TargetID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-1",
					Data:     map[string]interface{}{},
				}),
				expectRelation(&topology.Relation{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-3->" +
						"urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-2",
					Type:     topology.Type{Name: "routes"},
					SourceID: "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-3",
					TargetID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-2",
					Data:     map[string]interface{}{},
				}),
				expectRelation(&topology.Relation{
					ExternalID: "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-3->" +
						"urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-3",
					Type:     topology.Type{Name: "routes"},
					SourceID: "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-3",
					TargetID: "urn:kubernetes:/test-cluster-name:test-namespace:service/test-service-3",
					Data:     map[string]interface{}{},
				}),
				expectComponent(&topology.Component{
					ExternalID: "urn:endpoint:/test-cluster-name:34.100.200.15",
					Type:       topology.Type{Name: "endpoint"},
					Data: topology.Data{
						"name":              "34.100.200.15",
						"creationTimestamp": creationTime,
						"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace"},
						"identifiers":       []string{},
					},
				}),
				expectRelation(&topology.Relation{
					ExternalID: "urn:endpoint:/test-cluster-name:34.100.200.15->urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-3",
					SourceID:   "urn:endpoint:/test-cluster-name:34.100.200.15",
					TargetID:   "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-3",
					Type:       topology.Type{Name: "routes"},
					Data:       map[string]interface{}{},
				}),
				expectComponent(&topology.Component{
					ExternalID: "urn:endpoint:/test-cluster-name:64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com",
					Type:       topology.Type{Name: "endpoint"},
					Data: topology.Data{
						"name":              "64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com",
						"creationTimestamp": creationTime,
						"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name", "namespace": "test-namespace"},
						"identifiers":       []string{},
					},
				}),
				expectRelation(&topology.Relation{
					ExternalID: "urn:endpoint:/test-cluster-name:64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com->urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-3",
					SourceID:   "urn:endpoint:/test-cluster-name:64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com",
					TargetID:   "urn:kubernetes:/test-cluster-name:test-namespace:ingress/test-ingress-3",
					Type:       topology.Type{Name: "routes"},
					Data:       map[string]interface{}{},
				}),
			},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			for _, a := range tc.assertions {
				a(t, componentChannel, relationChannel)
			}
		})
	}
}

type MockIngressAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockIngressAPICollectorClient) GetIngresses() ([]v1beta1.Ingress, error) {
	ingresses := make([]v1beta1.Ingress, 0)
	for i := 1; i <= 3; i++ {
		ingress := v1beta1.Ingress{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-ingress-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-ingress-%d", i)),
				GenerateName: "",
			},
			Status: v1beta1.IngressStatus{
				LoadBalancer: coreV1.LoadBalancerStatus{
					Ingress: []coreV1.LoadBalancerIngress{
						{IP: "34.100.200.15"},
						{Hostname: "64047e8f24bb48e9a406ac8286ee8b7d.eu-west-1.elb.amazonaws.com"},
					},
				},
			},
		}

		if i == 2 {
			ingress.Spec.Backend = &v1beta1.IngressBackend{ServiceName: "test-service"}
		}

		if i == 3 {
			ingress.TypeMeta.Kind = "some-specified-kind"
			ingress.ObjectMeta.GenerateName = "some-specified-generation"
			ingress.Spec.Rules = []v1beta1.IngressRule{
				{
					Host: "host-1",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{Path: "host-1-path-1", Backend: v1beta1.IngressBackend{ServiceName: "test-service-1"}},
								{Path: "host-1-path-2", Backend: v1beta1.IngressBackend{ServiceName: "test-service-2"}},
							},
						},
					},
				},
				{
					Host: "host-2",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{Path: "host-2-path-1", Backend: v1beta1.IngressBackend{ServiceName: "test-service-3"}},
							},
						},
					},
				},
			}
		}

		ingresses = append(ingresses, ingress)
	}

	return ingresses, nil
}

func expectComponent(expected *topology.Component) func(*testing.T, chan *topology.Component, chan *topology.Relation) {
	return func(t *testing.T, componentChan chan *topology.Component, _ chan *topology.Relation) {
		c := <-componentChan
		assert.EqualValues(t, expected, c)
	}
}

func expectRelation(expected *topology.Relation) func(*testing.T, chan *topology.Component, chan *topology.Relation) {
	return func(t *testing.T, _ chan *topology.Component, relationChan chan *topology.Relation) {
		r := <-relationChan
		assert.EqualValues(t, expected, r)
	}
}
