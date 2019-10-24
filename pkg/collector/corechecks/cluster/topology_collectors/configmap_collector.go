// +build kubeapiserver

package topology_collectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
)

// ConfigMapCollector implements the ClusterTopologyCollector interface.
type ConfigMapCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan chan<- *topology.Relation
	ClusterTopologyCollector
}

// NewConfigMapCollector
func NewConfigMapCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &ConfigMapCollector{
		ComponentChan: componentChannel,
		RelationChan: relationChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *ConfigMapCollector) GetName() string {
	return "ConfigMap Collector"
}

// Collects and Published the ConfigMap Components
func (cmc *ConfigMapCollector) CollectorFunction() error {

	return nil
}
