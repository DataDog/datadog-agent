// +build kubeapiserver

package topology_collectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/apps/v1"
)

// ReplicaSetCollector implements the ClusterTopologyCollector interface.
type ReplicaSetCollector struct {
	ComponentChan chan<- *topology.Component
	ClusterTopologyCollector
}

// NewReplicaSetCollector
func NewReplicaSetCollector(componentChannel chan<- *topology.Component, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &ReplicaSetCollector{
		ComponentChan: componentChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *ReplicaSetCollector) GetName() string {
	return "ReplicaSet Collector"
}

// Collects and Published the ReplicaSet Components
func (rsc *ReplicaSetCollector) CollectorFunction() error {
	replicaSets, err := rsc.GetAPIClient().GetReplicaSets()
	if err != nil {
		return err
	}

	for _, rs := range replicaSets {
		rsc.ComponentChan <- rsc.replicaSetToStackStateComponent(rs)
	}

	return nil
}

// Creates a StackState component from a Kubernetes / OpenShift Cluster
func (dsc *ReplicaSetCollector) replicaSetToStackStateComponent(replicaSet v1.ReplicaSet) *topology.Component {
	log.Tracef("Mapping ReplicaSet to StackState component: %s", replicaSet.String())

	tags := emptyIfNil(replicaSet.Labels)
	tags = dsc.addClusterNameTag(tags)

	replicaSetExternalID := dsc.buildReplicaSetExternalID(replicaSet.Name)
	component := &topology.Component{
		ExternalID: replicaSetExternalID,
		Type:       topology.Type{Name: "replicaset"},
		Data: map[string]interface{}{
			"name":              replicaSet.Name,
			"kind":              replicaSet.Kind,
			"creationTimestamp": replicaSet.CreationTimestamp,
			"tags":              tags,
			"namespace":         replicaSet.Namespace,
			"desiredReplicas": replicaSet.Spec.Replicas,
			"uid":           replicaSet.UID,
			"generateName":  replicaSet.GenerateName,
		},
	}

	log.Tracef("Created StackState ReplicaSet component %s: %v", replicaSetExternalID, component.JSONString())

	return component
}
