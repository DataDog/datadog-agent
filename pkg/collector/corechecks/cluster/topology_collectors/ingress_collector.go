// +build kubeapiserver

package topology_collectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
)

// IngressCollector implements the ClusterTopologyCollector interface.
type IngressCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan chan<- *topology.Relation
	ClusterTopologyCollector
}

// NewIngressCollector
func NewIngressCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &IngressCollector{
		ComponentChan: componentChannel,
		RelationChan: relationChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *IngressCollector) GetName() string {
	return "Ingress Collector"
}

// Collects and Published the Ingress Components
func (cc *IngressCollector) CollectorFunction() error {

	return nil
}

