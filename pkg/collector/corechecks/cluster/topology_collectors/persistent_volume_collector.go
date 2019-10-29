// +build kubeapiserver

package topology_collectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

// PersistentVolumeCollector implements the ClusterTopologyCollector interface.
type PersistentVolumeCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan chan<- *topology.Relation
	ClusterTopologyCollector
}

// NewPersistentVolumeCollector
func NewPersistentVolumeCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &PersistentVolumeCollector{
		ComponentChan: componentChannel,
		RelationChan: relationChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *PersistentVolumeCollector) GetName() string {
	return "Persistent Volume Collector"
}

// Collects and Published the Persistent Volume Components
func (pvc *PersistentVolumeCollector) CollectorFunction() error {
	volumeClaims, err := pvc.GetAPIClient().GetPersistentVolumeClaims()
	if err != nil {
		return err
	}

	for _, v := range volumeClaims {
		log.Debugf("Received persistent volume claim: %v", v.String())
	}

	volumes, err := pvc.GetAPIClient().GetPersistentVolumes()
	if err != nil {
		return err
	}

	for _, v := range volumes {
		log.Debugf("Received persistent volume: %v", v.String())
	}


	return nil
}
