// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package topology_collectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"testing"
	"time"
)

var creationTime v1.Time

func TestConfigMapCollector(t *testing.T) {

	componentChannel := make(chan *topology.Component)
	relationChannel := make(chan *topology.Relation)
	defer close(componentChannel)
	defer close(relationChannel)

	creationTime = v1.Time{ Time: time.Now().Add(-1*time.Hour) }

	cmc := NewConfigMapCollector(componentChannel, relationChannel, NewTestCommonClusterCollector(MockConfigMapAPICollectorClient{}))

	expectedCollectorName := "ConfigMap Collector"
	actualCollectorName := cmc.GetName()
	assert.Equal(t, expectedCollectorName, actualCollectorName)

	// Trigger Collector Function
	go func() {
		log.Debugf("Starting cluster topology collector: %s\n", cmc.GetName())
		err := cmc.CollectorFunction()
		// assert no error occurred
		assert.Nil(t, err)
		// mark this collector as complete
		log.Debugf("Finished cluster topology collector: %s\n", cmc.GetName())
	}()

	type test struct {
		expected *topology.Component
	}

	tests := []test{
		{expected: &topology.Component{
			ExternalID: "urn:/kubernetes:test-cluster-name:configmap:test-namespace:test-configmap-1",
			Type: topology.Type{Name:"configmap"},
			Data:topology.Data{
				"name":"test-configmap-1",
				"creationTimestamp": creationTime,
				"tags":map[string]string{"test":"label", "cluster-name":"test-cluster-name"},
				"namespace":"test-namespace",
				"uid": types.UID("test-configmap-1"),
			},
		}},
		{expected: &topology.Component{
			ExternalID: "urn:/kubernetes:test-cluster-name:configmap:test-namespace:test-configmap-2",
			Type: topology.Type{Name:"configmap"},
			Data:topology.Data{
				"name":"test-configmap-2",
				"creationTimestamp": creationTime,
				"tags":map[string]string{"test":"label", "cluster-name":"test-cluster-name"},
				"namespace":"test-namespace",
				"uid": types.UID("test-configmap-2"),
			},
		}},
		{expected: &topology.Component{
			ExternalID: "urn:/kubernetes:test-cluster-name:configmap:test-namespace:test-configmap-3",
			Type: topology.Type{Name:"configmap"},
			Data:topology.Data{
				"name":"test-configmap-3",
				"creationTimestamp": creationTime,
				"tags":map[string]string{"test":"label", "cluster-name":"test-cluster-name"},
				"namespace":"test-namespace",
				"uid": types.UID("test-configmap-3"),
			},
		}},
	}

	for _, tc := range tests {
		configMap := <- componentChannel
		assert.EqualValues(t, tc.expected, configMap)
	}

}

type MockConfigMapAPICollectorClient struct {
	apiserver.APICollectorClient
}

func (m MockConfigMapAPICollectorClient) GetConfigMaps() ([]coreV1.ConfigMap, error) {
	configMaps := make([]coreV1.ConfigMap, 0)
	for i := 1; i <= 3; i++ {
		configMaps = append(configMaps, coreV1.ConfigMap{
			TypeMeta: v1.TypeMeta{
				Kind: "",
			},
			ObjectMeta: v1.ObjectMeta{
				Name:              fmt.Sprintf("test-configmap-%d", i),
				CreationTimestamp: creationTime,
				Namespace:         "test-namespace",
				Labels: map[string]string{
					"test": "label",
				},
				UID:          types.UID(fmt.Sprintf("test-configmap-%d", i)),
				GenerateName: "",
			},
			Data: map[string]string{
				"key": "value",
			},
		})
	}

	return configMaps, nil
}
