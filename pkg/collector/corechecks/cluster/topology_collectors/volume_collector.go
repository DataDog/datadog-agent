// +build kubeapiserver

package topology_collectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

// VolumeCollector implements the ClusterTopologyCollector interface.
type VolumeCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan chan<- *topology.Relation
	ClusterTopologyCollector
}

// NewVolumeCollector
func NewVolumeCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &VolumeCollector{
		ComponentChan: componentChannel,
		RelationChan: relationChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *VolumeCollector) GetName() string {
	return "Volume Collector"
}

// Collects and Published the Volume Components
func (vc *VolumeCollector) CollectorFunction() error {
	volumes, err := vc.GetAPIClient().GetVolumeAttachments()
	if err != nil {
		return err
	}

	for _, v := range volumes {
		log.Debugf("Received volume attachment: %v", v.String())
	}

	return nil
}
