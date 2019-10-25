// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package kubeapi

import (
	collectors "github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/topology_collectors"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/pkg/errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunClusterCollectors(t *testing.T) {

	kubernetesTopologyCheck := KubernetesApiTopologyFactory().(*TopologyCheck)
	instance := topology.Instance{Type: "kubernetes", URL: "test-cluster-name"}

	var wg sync.WaitGroup
	componentChannel := make(chan *topology.Component)
	relationChannel := make(chan *topology.Relation)
	errChannel := make(chan error)
	defer close(componentChannel)
	defer close(relationChannel)
	defer close(errChannel)

	var clusterCollectors []collectors.ClusterTopologyCollector
	commonClusterCollector := collectors.NewClusterTopologyCollector(instance, nil)

	clusterCollectors = append(clusterCollectors,
		NewTestCollector(componentChannel, relationChannel, commonClusterCollector),
		NewErrorTestCollector(componentChannel, relationChannel, commonClusterCollector),
	)

	wg.Add(len(clusterCollectors))

	kubernetesTopologyCheck.runClusterCollectors(clusterCollectors, wg, errChannel)

	component1 := <- componentChannel
	assert.Equal(t, component1.ExternalID, "1")

	component2 := <- componentChannel
	assert.Equal(t, component2.ExternalID, "2")

	component3 := <- componentChannel
	assert.Equal(t, component3.ExternalID, "3")

	component4 := <- componentChannel
	assert.Equal(t, component4.ExternalID, "4")

	relation1 := <- relationChannel
	assert.Equal(t, relation1.ExternalID, "1->2")

	relation2 := <- relationChannel
	assert.Equal(t, relation2.ExternalID, "3->4")

	error := <- errChannel
	assert.Equal(t, "ErrorTestCollector", error.Error())
}

// TestCollector implements the ClusterTopologyCollector interface.
type TestCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan chan<- *topology.Relation
	collectors.ClusterTopologyCollector
}

// NewTestCollector
func NewTestCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation, clusterTopologyCollector collectors.ClusterTopologyCollector) collectors.ClusterTopologyCollector {
	return &TestCollector{
		ComponentChan: componentChannel,
		RelationChan: relationChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the TestCollector
func (_ *TestCollector) GetName() string {
	return "Test Collector"
}

// Collects and Publishes dummy Components and Relations
func (tc *TestCollector) CollectorFunction() error {
	tc.ComponentChan <- &topology.Component{ExternalID: "1", Type: topology.Type{Name: "component-type"} }
	tc.ComponentChan <- &topology.Component{ExternalID: "2", Type: topology.Type{Name: "component-type"} }
	tc.ComponentChan <- &topology.Component{ExternalID: "3", Type: topology.Type{Name: "component-type"} }
	tc.ComponentChan <- &topology.Component{ExternalID: "4", Type: topology.Type{Name: "component-type"} }

	tc.RelationChan <- &topology.Relation{ ExternalID: "1->2"}
	tc.RelationChan <- &topology.Relation{ ExternalID: "3->4"}

	return nil
}

// ErrorTestCollector implements the ClusterTopologyCollector interface.
type ErrorTestCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan chan<- *topology.Relation
	collectors.ClusterTopologyCollector
}

// NewErrorTestCollector
func NewErrorTestCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation, clusterTopologyCollector collectors.ClusterTopologyCollector) collectors.ClusterTopologyCollector {
	return &ErrorTestCollector{
		ComponentChan: componentChannel,
		RelationChan: relationChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the ErrorTestCollector
func (_ *ErrorTestCollector) GetName() string {
	return "Error Test Collector"
}

// Returns a error
func (etc *ErrorTestCollector) CollectorFunction() error {
	return errors.New("ErrorTestCollector")
}
