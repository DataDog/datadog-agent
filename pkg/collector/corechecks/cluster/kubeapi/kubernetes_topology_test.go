// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package kubeapi

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	collectors "github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/topologycollectors"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"strconv"
	"sync"
	"testing"
)

var componentID int
var relationID int

func TestRunClusterCollectors(t *testing.T) {
	// set the initial id values
	componentID = 1
	relationID = 1

	kubernetesTopologyCheck := KubernetesApiTopologyFactory().(*TopologyCheck)
	instance := topology.Instance{Type: "kubernetes", URL: "test-cluster-name"}
	// set up the batcher for this instance
	kubernetesTopologyCheck.instance.CollectTimeout = 5
	kubernetesTopologyCheck.submitter = NewTestTopologySubmitter(t, "kubernetes_api_topology", instance)

	var waitGroup sync.WaitGroup
	componentChannel := make(chan *topology.Component)
	relationChannel := make(chan *topology.Relation)
	errChannel := make(chan error)
	waitGroupChannel := make(chan bool)

	clusterTopologyCommon := collectors.NewClusterTopologyCommon(instance, nil)
	commonClusterCollector := collectors.NewClusterTopologyCollector(clusterTopologyCommon)

	clusterCollectors := []collectors.ClusterTopologyCollector{
		NewTestCollector(componentChannel, relationChannel, commonClusterCollector),
		NewErrorTestCollector(componentChannel, relationChannel, commonClusterCollector),
	}
	clusterCorrelators := make([]collectors.ClusterTopologyCorrelator, 0)

	// starts all the cluster collectors
	kubernetesTopologyCheck.RunClusterCollectors(clusterCollectors, clusterCorrelators, &waitGroup, errChannel)

	// receive all the components, will return once the wait group notifies
	kubernetesTopologyCheck.WaitForTopology(componentChannel, relationChannel, errChannel, &waitGroup, waitGroupChannel)

	close(componentChannel)
	close(relationChannel)
	close(errChannel)
	close(waitGroupChannel)
}

// NewTestTopologySubmitter creates a new instance of TestTopologySubmitter
func NewTestTopologySubmitter(t *testing.T, checkID check.ID, instance topology.Instance) TopologySubmitter {
	return &TestTopologySubmitter{
		t:        t,
		CheckID:  checkID,
		Instance: instance,
	}
}

// TestTopologySubmitter provides functionality to submit topology data with the Batcher.
type TestTopologySubmitter struct {
	t        *testing.T
	CheckID  check.ID
	Instance topology.Instance
}

func (b *TestTopologySubmitter) SubmitStartSnapshot() {}
func (b *TestTopologySubmitter) SubmitStopSnapshot()  {}
func (b *TestTopologySubmitter) SubmitComplete()      {}

// SubmitRelation takes a component and submits it with the Batcher
func (b *TestTopologySubmitter) SubmitComponent(component *topology.Component) {
	// match the component with the count number that represents the ExternalID
	assert.Equal(b.t, strconv.Itoa(componentID), component.ExternalID)
	componentID = componentID + 1
}

// SubmitRelation takes a relation and submits it with the Batcher
func (b *TestTopologySubmitter) SubmitRelation(relation *topology.Relation) {
	// match the relation with the count number -> +1 that represents the ExternalID
	assert.Equal(b.t, fmt.Sprintf("%s->%s", strconv.Itoa(relationID), strconv.Itoa(relationID+1)), relation.ExternalID)
	relationID = relationID + 2
}

// HandleError handles any errors during topology gathering
func (b *TestTopologySubmitter) HandleError(err error) {
	// match the error message
	assert.Equal(b.t, "ErrorTestCollector", err.Error())
}

// TestCollector implements the ClusterTopologyCollector interface.
type TestCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan  chan<- *topology.Relation
	collectors.ClusterTopologyCollector
}

// NewTestCollector
func NewTestCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation, clusterTopologyCollector collectors.ClusterTopologyCollector) collectors.ClusterTopologyCollector {
	return &TestCollector{
		ComponentChan:            componentChannel,
		RelationChan:             relationChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the TestCollector
func (_ *TestCollector) GetName() string {
	return "Test Collector"
}

// Collects and Publishes dummy Components and Relations
func (tc *TestCollector) CollectorFunction() error {
	tc.ComponentChan <- &topology.Component{ExternalID: "1", Type: topology.Type{Name: "component-type"}}
	tc.ComponentChan <- &topology.Component{ExternalID: "2", Type: topology.Type{Name: "component-type"}}
	tc.ComponentChan <- &topology.Component{ExternalID: "3", Type: topology.Type{Name: "component-type"}}
	tc.ComponentChan <- &topology.Component{ExternalID: "4", Type: topology.Type{Name: "component-type"}}

	tc.RelationChan <- &topology.Relation{ExternalID: "1->2"}
	tc.RelationChan <- &topology.Relation{ExternalID: "3->4"}

	return nil
}

// ErrorTestCollector implements the ClusterTopologyCollector interface.
type ErrorTestCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan  chan<- *topology.Relation
	collectors.ClusterTopologyCollector
}

// NewErrorTestCollector
func NewErrorTestCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation, clusterTopologyCollector collectors.ClusterTopologyCollector) collectors.ClusterTopologyCollector {
	return &ErrorTestCollector{
		ComponentChan:            componentChannel,
		RelationChan:             relationChannel,
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
