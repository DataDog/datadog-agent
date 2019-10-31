// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package topology_collectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestClusterCollector(t *testing.T) {
	componentChannel := make(chan *topology.Component)
	defer close(componentChannel)

	cc := NewClusterCollector(componentChannel, NewTestCommonClusterCollector(MockClusterAPICollectorClient{}))

	expectedCollectorName := "Cluster Collector"
	actualCollectorName := cc.GetName()
	assert.Equal(t, expectedCollectorName, actualCollectorName)

	// Trigger Collector Function
	go func() {
		log.Debugf("Starting cluster topology collector: %s\n", cc.GetName())
		err := cc.CollectorFunction()
		// assert no error occurred
		assert.Nil(t, err)
		// mark this collector as complete
		log.Debugf("Finished cluster topology collector: %s\n", cc.GetName())
	}()

	type test struct {
		expected *topology.Component
	}

	tests := []test{
		{expected: &topology.Component{
			ExternalID: "urn:cluster:kubernetes/test-cluster-name",
			Type: topology.Type{Name: "cluster"},
			Data: topology.Data{
				"name": "test-cluster-name",
				"tags":map[string]string{"cluster-name":"test-cluster-name"}},
			},
		},
	}

	for _, tc := range tests {
		clusterComponent := <- componentChannel
		assert.EqualValues(t, tc.expected, clusterComponent)
	}
}

type MockClusterAPICollectorClient struct {
	apiserver.APICollectorClient
}
