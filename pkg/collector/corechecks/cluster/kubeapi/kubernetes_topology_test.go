// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package kubeapi

import (
	"fmt"
	collectors "github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/topology_collectors"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestRunClusterCollectors(t *testing.T) {

	kubernetesTopologyCheck := KubernetesApiTopologyFactory().(*TopologyCheck)
	instance := topology.Instance{Type: "kubernetes", URL: "test-cluster-name"}

	var waitGroup sync.WaitGroup
	componentChannel := make(chan *topology.Component)
	relationChannel := make(chan *topology.Relation)
	errChannel := make(chan error)
	waitGroupChannel := make(chan int)
	defer close(componentChannel)
	defer close(relationChannel)
	defer close(errChannel)
	defer close(waitGroupChannel)

	var clusterCollectors []collectors.ClusterTopologyCollector
	commonClusterCollector := collectors.NewClusterTopologyCollector(instance, nil)

	clusterCollectors = append(clusterCollectors,
		NewTestCollector(componentChannel, relationChannel, commonClusterCollector),
		NewErrorTestCollector(componentChannel, relationChannel, commonClusterCollector),
	)
	kubernetesTopologyCheck.runClusterCollectors(clusterCollectors, &waitGroup, errChannel)

	assertTopology(t, componentChannel, relationChannel, errChannel, &waitGroup, waitGroupChannel)

}

// sets up the receiver that handles the component, relation and error channel and asserts the response.
func assertTopology(t *testing.T , componentChannel <-chan *topology.Component, relationChannel <-chan *topology.Relation,
	errorChannel <-chan error, waitGroup *sync.WaitGroup, waitGroupChannel chan int) {
	timeout := 1 * time.Minute
	go func() {
		waitGroup.Wait()
		waitGroupChannel <- 1
	}()

	componentCount := 1
	relationCount := 1

	for {
		select {
		case component := <-componentChannel:
			// match the component with the count number that represents the ExternalID
			assert.Equal(t, strconv.Itoa(componentCount), component.ExternalID)
			componentCount = componentCount + 1
		case relation := <-relationChannel:
			// match the relation with the count number -> +1 that represents the ExternalID
			assert.Equal(t, fmt.Sprintf("%s->%s", strconv.Itoa(relationCount), strconv.Itoa(relationCount+1)), relation.ExternalID)
			relationCount = relationCount + 2
		case err := <-errorChannel:
			// match the error message
			assert.Equal(t, "ErrorTestCollector", err.Error())
		case cnt := <-waitGroupChannel:
			// match the wait group channel closing message
			assert.Equal(t, 1, cnt)
			return
		case <-time.After(timeout):
			assert.Fail(t, "Topology Collectors timed out")
		default:
			// no message received, continue looping
		}
	}

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
