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
	appsV1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"
)

func TestStatefulSetCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}

	replicas = int32(1)

	cmc := NewStatefulSetCollector(componentChannel, NewTestCommonClusterCollector(MockStatefulSetAPICollectorClient{}))
	expectedCollectorName := "StatefulSet Collector"
	RunCollectorTest(t, cmc, expectedCollectorName)

	for _, tc := range []struct {
		testCase string
		expected *topology.Component
	}{
		{
			testCase: "Test StatefulSet 1",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:statefulset:test-statefulset-1",
				Type:       topology.Type{Name: "statefulset"},
				Data: topology.Data{
					"name":                "test-statefulset-1",
					"creationTimestamp":   creationTime,
					"tags":                map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":           "test-namespace",
					"uid":                 types.UID("test-statefulset-1"),
					"updateStrategy":      appsV1.StatefulSetUpdateStrategyType(appsV1.RollingUpdateStatefulSetStrategyType),
					"desiredReplicas":     &replicas,
					"podManagementPolicy": appsV1.OrderedReadyPodManagement,
					"serviceName":         "statefulset-service-name",
				},
			},
		},
		{
			testCase: "Test StatefulSet 2",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:statefulset:test-statefulset-2",
				Type:       topology.Type{Name: "statefulset"},
				Data: topology.Data{
					"name":                "test-statefulset-2",
					"creationTimestamp":   creationTime,
					"tags":                map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":           "test-namespace",
					"uid":                 types.UID("test-statefulset-2"),
					"updateStrategy":      appsV1.StatefulSetUpdateStrategyType(appsV1.RollingUpdateStatefulSetStrategyType),
					"desiredReplicas":     &replicas,
					"podManagementPolicy": appsV1.OrderedReadyPodManagement,
					"serviceName":         "statefulset-service-name",
				},
			},
		},
		{
			testCase: "Test StatefulSet 3 - Kind + Generate Name",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:statefulset:test-statefulset-3",
				Type:       topology.Type{Name: "statefulset"},
				Data: topology.Data{
					"name":                "test-statefulset-3",
					"creationTimestamp":   creationTime,
					"tags":                map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":           "test-namespace",
					"uid":                 types.UID("test-statefulset-3"),
					"kind":                "some-specified-kind",
					"generateName":        "some-specified-generation",
					"updateStrategy":      appsV1.StatefulSetUpdateStrategyType(appsV1.RollingUpdateStatefulSetStrategyType),
					"desiredReplicas":     &replicas,
					"podManagementPolicy": appsV1.OrderedReadyPodManagement,
					"serviceName":         "statefulset-service-name",
				},
			},
		},
	} {
		t.Run(tc.testCase, func(t *testing.T) {
			component := <-componentChannel
			assert.EqualValues(t, tc.expected, component)
		})
	}
}

type MockStatefulSetAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockStatefulSetAPICollectorClient) GetStatefulSets() ([]appsV1.StatefulSet, error) {
	statefulSets := make([]appsV1.StatefulSet, 0)
	for i := 1; i <= 3; i++ {
		statefulSet := appsV1.StatefulSet{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-statefulset-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-statefulset-%d", i)),
				GenerateName: "",
			},
			Spec: appsV1.StatefulSetSpec{
				UpdateStrategy: appsV1.StatefulSetUpdateStrategy{
					Type: appsV1.RollingUpdateStatefulSetStrategyType,
				},
				Replicas:            &replicas,
				PodManagementPolicy: appsV1.OrderedReadyPodManagement,
				ServiceName:         "statefulset-service-name",
			},
		}

		if i == 3 {
			statefulSet.TypeMeta.Kind = "some-specified-kind"
			statefulSet.ObjectMeta.GenerateName = "some-specified-generation"
		}

		statefulSets = append(statefulSets, statefulSet)
	}

	return statefulSets, nil
}
