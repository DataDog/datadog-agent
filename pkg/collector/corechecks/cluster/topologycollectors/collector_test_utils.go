// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package topologycollectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

var creationTime v1.Time
var replicas int32

func NewTestCommonClusterCollector(client apiserver.APICollectorClient) ClusterTopologyCollector {
	instance := topology.Instance{Type: "kubernetes", URL: "test-cluster-name"}

	clusterTopologyCommon := NewClusterTopologyCommon(instance, client)
	return NewClusterTopologyCollector(clusterTopologyCommon)
}

func RunCollectorTest(t *testing.T, collector ClusterTopologyCollector, expectedCollectorName string) {
	actualCollectorName := collector.GetName()
	assert.Equal(t, expectedCollectorName, actualCollectorName)

	// Trigger Collector Function
	go func() {
		log.Debugf("Starting cluster topology collector: %s\n", collector.GetName())
		err := collector.CollectorFunction()
		// assert no error occurred
		assert.Nil(t, err)
		// mark this collector as complete
		log.Debugf("Finished cluster topology collector: %s\n", collector.GetName())
	}()
}
