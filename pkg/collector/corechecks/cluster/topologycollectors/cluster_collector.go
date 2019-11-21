// +build kubeapiserver

package topologycollectors

import (
	"errors"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

// ClusterCollector implements the ClusterTopologyCollector interface.
type ClusterCollector struct {
	ComponentChan chan<- *topology.Component
	ClusterTopologyCollector
}

// NewClusterTopologyCollector
func NewClusterCollector(componentChannel chan<- *topology.Component, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &ClusterCollector{
		ComponentChan:            componentChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *ClusterCollector) GetName() string {
	return "Cluster Collector"
}

// Collects and Published the Cluster Component
func (cc *ClusterCollector) CollectorFunction() error {
	if cc.GetInstance().Type == "" || cc.GetInstance().URL == "" {
		return errors.New("cluster name or cluster instance type could not be detected, " +
			"therefore we are unable to create the cluster component")
	}

	cc.ComponentChan <- cc.clusterToStackStateComponent()
	return nil
}

// Creates a StackState component from a Kubernetes / OpenShift Cluster
func (cc *ClusterCollector) clusterToStackStateComponent() *topology.Component {
	clusterExternalID := cc.buildClusterExternalID()

	tags := make(map[string]string, 0)
	tags = cc.addClusterNameTag(tags)

	component := &topology.Component{
		ExternalID: clusterExternalID,
		Type:       topology.Type{Name: "cluster"},
		Data: map[string]interface{}{
			"name": cc.GetInstance().URL,
			"tags": tags,
		},
	}

	log.Tracef("Created StackState cluster component %s: %v", clusterExternalID, component.JSONString())

	return component
}
