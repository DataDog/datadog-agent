// +build kubeapiserver

package topologycollectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/apps/v1"
)

// ReplicaSetCollector implements the ClusterTopologyCollector interface.
type ReplicaSetCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan  chan<- *topology.Relation
	ClusterTopologyCollector
}

// GetName returns the name of the Collector
func (rsc *ReplicaSetCollector) GetName() string {
	return "ReplicaSet Collector"
}

// NewReplicaSetCollector
func NewReplicaSetCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &ReplicaSetCollector{
		ComponentChan:            componentChannel,
		RelationChan:             relationChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// Collects and Published the ReplicaSet Components
func (rsc *ReplicaSetCollector) CollectorFunction() error {
	replicaSets, err := rsc.GetAPIClient().GetReplicaSets()
	if err != nil {
		return err
	}

	for _, rs := range replicaSets {
		component := rsc.replicaSetToStackStateComponent(rs)
		rsc.ComponentChan <- component

		// check to see if this pod is "controlled" by a deployment
		for _, ref := range rs.OwnerReferences {
			switch kind := ref.Kind; kind {
			case Deployment:
				dmExternalID := rsc.buildDeploymentExternalID(rs.Namespace, ref.Name)
				rsc.RelationChan <- rsc.deploymentToReplicaSetStackStateRelation(dmExternalID, component.ExternalID)
			}
		}

	}

	return nil
}

// Creates a StackState component from a Kubernetes / OpenShift Cluster
func (rsc *ReplicaSetCollector) replicaSetToStackStateComponent(replicaSet v1.ReplicaSet) *topology.Component {
	log.Tracef("Mapping ReplicaSet to StackState component: %s", replicaSet.String())

	tags := emptyIfNil(replicaSet.Labels)
	tags = rsc.addClusterNameTag(tags)

	replicaSetExternalID := rsc.buildReplicaSetExternalID(replicaSet.Name)
	component := &topology.Component{
		ExternalID: replicaSetExternalID,
		Type:       topology.Type{Name: "replicaset"},
		Data: map[string]interface{}{
			"name":              replicaSet.Name,
			"creationTimestamp": replicaSet.CreationTimestamp,
			"tags":              tags,
			"namespace":         replicaSet.Namespace,
			"desiredReplicas":   replicaSet.Spec.Replicas,
			"uid":               replicaSet.UID,
		},
	}

	component.Data.PutNonEmpty("kind", replicaSet.Kind)
	component.Data.PutNonEmpty("generateName", replicaSet.GenerateName)

	log.Tracef("Created StackState ReplicaSet component %s: %v", replicaSetExternalID, component.JSONString())

	return component
}

// Creates a StackState relation from a Kubernetes / OpenShift Controller Workload to Pod relation
func (rsc *ReplicaSetCollector) deploymentToReplicaSetStackStateRelation(deploymentExternalID, replicaSetExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes deployment to replica set relation: %s -> %s", deploymentExternalID, replicaSetExternalID)

	relation := rsc.CreateRelation(deploymentExternalID, replicaSetExternalID, "controls")

	log.Tracef("Created StackState deployment -> replica set relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}
