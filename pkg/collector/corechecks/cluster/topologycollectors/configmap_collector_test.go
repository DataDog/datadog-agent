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

func TestConfigMapCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)

	creationTime = v1.Time{Time: time.Now().Add(-1 * time.Hour)}

	cmc := NewConfigMapCollector(componentChannel, NewTestCommonClusterCollector(MockConfigMapAPICollectorClient{}))
	expectedCollectorName := "ConfigMap Collector"
	RunCollectorTest(t, cmc, expectedCollectorName)

	type test struct {
		testCase string
		expected *topology.Component
	}

	for _, tc := range []struct {
		testCase string
		expected *topology.Component
	}{
		{
			testCase: "Test ConfigMap 1 - Complete",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:configmap:test-namespace:test-configmap-1",
				Type:       topology.Type{Name: "configmap"},
				Data: topology.Data{
					"name":              "test-configmap-1",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-configmap-1"),
					"data":              map[string]string{"key1": "value1", "key2": "longersecretvalue2"},
					"identifiers":       []string{"urn:/kubernetes:test-cluster-name:configmap:test-namespace:test-configmap-1"},
				},
			},
		},
		{
			testCase: "Test ConfigMap 2 - Without Data",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:configmap:test-namespace:test-configmap-2",
				Type:       topology.Type{Name: "configmap"},
				Data: topology.Data{
					"name":              "test-configmap-2",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"test": "label", "cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-configmap-2"),
					"identifiers":       []string{"urn:/kubernetes:test-cluster-name:configmap:test-namespace:test-configmap-2"},
				},
			},
		},
		{
			testCase: "Test ConfigMap 3 - Minimal",
			expected: &topology.Component{
				ExternalID: "urn:/kubernetes:test-cluster-name:configmap:test-namespace:test-configmap-3",
				Type:       topology.Type{Name: "configmap"},
				Data: topology.Data{
					"name":              "test-configmap-3",
					"creationTimestamp": creationTime,
					"tags":              map[string]string{"cluster-name": "test-cluster-name"},
					"namespace":         "test-namespace",
					"uid":               types.UID("test-configmap-3"),
					"identifiers":       []string{"urn:/kubernetes:test-cluster-name:configmap:test-namespace:test-configmap-3"},
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

type MockConfigMapAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockConfigMapAPICollectorClient) GetConfigMaps() ([]coreV1.ConfigMap, error) {
	configMaps := make([]coreV1.ConfigMap, 0)
	for i := 1; i <= 3; i++ {

		configMap := coreV1.ConfigMap{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-configmap-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				UID:               types.UID(fmt.Sprintf("test-configmap-%d", i)),
				GenerateName:      "",
			},
		}

		if i == 1 {
			configMap.Data = map[string]string{
				"key1": "value1",
				"key2": "longersecretvalue2",
			}
		}

		if i != 3 {
			configMap.Labels = map[string]string{
				"test": "label",
			}
		}

		configMaps = append(configMaps, configMap)
	}

	return configMaps, nil
}
