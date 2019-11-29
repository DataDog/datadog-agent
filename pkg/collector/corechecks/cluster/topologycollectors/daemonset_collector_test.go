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

func TestDaemonSetCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}

	cmc := NewDaemonSetCollector(componentChannel, NewTestCommonClusterCollector(MockDaemonSetAPICollectorClient{}))
	expectedCollectorName := "DaemonSet Collector"
	RunCollectorTest(t, cmc, expectedCollectorName)

	for _, tc := range []struct {
		testCase string
		expected *topology.Component
	}{
		{
			testCase: "Test DaemonSet 1",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:daemonset:test-daemonset-1",
				Type:       topology.Type{Name: "daemonset"},
				Data: topology.Data{
					"name":              "test-daemonset-1",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-daemonset-1"),
					"updateStrategy":    appsV1.RollingUpdateDaemonSetStrategyType,
				},
			},
		},
		{
			testCase: "Test DaemonSet 2",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:daemonset:test-daemonset-2",
				Type:       topology.Type{Name: "daemonset"},
				Data: topology.Data{
					"name":              "test-daemonset-2",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-daemonset-2"),
					"updateStrategy":    appsV1.RollingUpdateDaemonSetStrategyType,
				},
			},
		},
		{
			testCase: "Test DaemonSet 3 - Kind + Generate Name",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:daemonset:test-daemonset-3",
				Type:       topology.Type{Name: "daemonset"},
				Data: topology.Data{
					"name":              "test-daemonset-3",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-daemonset-3"),
					"updateStrategy":    appsV1.RollingUpdateDaemonSetStrategyType,
					"kind":              "some-specified-kind",
					"generateName":      "some-specified-generation",
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

type MockDaemonSetAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockDaemonSetAPICollectorClient) GetDaemonSets() ([]appsV1.DaemonSet, error) {
	daemonSets := make([]appsV1.DaemonSet, 0)
	for i := 1; i <= 3; i++ {
		daemonSet := appsV1.DaemonSet{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-daemonset-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-daemonset-%d", i)),
				GenerateName: "",
			},
			Spec: appsV1.DaemonSetSpec{
				UpdateStrategy: appsV1.DaemonSetUpdateStrategy{
					Type: appsV1.RollingUpdateDaemonSetStrategyType,
				},
			},
		}

		if i == 3 {
			daemonSet.TypeMeta.Kind = "some-specified-kind"
			daemonSet.ObjectMeta.GenerateName = "some-specified-generation"
		}

		daemonSets = append(daemonSets, daemonSet)
	}

	return daemonSets, nil
}
