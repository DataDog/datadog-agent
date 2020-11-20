// +build kubeapiserver

package topologycollectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/apps/v1"
)

// DaemonSetCollector implements the ClusterTopologyCollector interface.
type DaemonSetCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan  chan<- *topology.Relation
	ClusterTopologyCollector
}

// NewDaemonSetCollector
func NewDaemonSetCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &DaemonSetCollector{
		ComponentChan:            componentChannel,
		RelationChan:             relationChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *DaemonSetCollector) GetName() string {
	return "DaemonSet Collector"
}

// Collects and Published the DaemonSet Components
func (dsc *DaemonSetCollector) CollectorFunction() error {
	daemonSets, err := dsc.GetAPIClient().GetDaemonSets()
	if err != nil {
		return err
	}

	for _, ds := range daemonSets {
		component := dsc.daemonSetToStackStateComponent(ds)
		dsc.ComponentChan <- component
		dsc.RelationChan <- dsc.namespaceToDaemonSetStackStateRelation(dsc.buildNamespaceExternalID(ds.Namespace), component.ExternalID)
	}

	return nil
}

// Creates a StackState daemonset component from a Kubernetes / OpenShift Cluster
func (dsc *DaemonSetCollector) daemonSetToStackStateComponent(daemonSet v1.DaemonSet) *topology.Component {
	log.Tracef("Mapping DaemonSet to StackState component: %s", daemonSet.String())

	tags := dsc.initTags(daemonSet.ObjectMeta)

	daemonSetExternalID := dsc.buildDaemonSetExternalID(daemonSet.Namespace, daemonSet.Name)
	component := &topology.Component{
		ExternalID: daemonSetExternalID,
		Type:       topology.Type{Name: "daemonset"},
		Data: map[string]interface{}{
			"name":              daemonSet.Name,
			"creationTimestamp": daemonSet.CreationTimestamp,
			"tags":              tags,
			"updateStrategy":    daemonSet.Spec.UpdateStrategy.Type,
			"uid":               daemonSet.UID,
		},
	}

	component.Data.PutNonEmpty("generateName", daemonSet.GenerateName)
	component.Data.PutNonEmpty("kind", daemonSet.Kind)

	log.Tracef("Created StackState DaemonSet component %s: %v", daemonSetExternalID, component.JSONString())

	return component
}

// Creates a StackState relation from a Kubernetes / OpenShift Namespace to DaemonSet relation
func (dsc *DaemonSetCollector) namespaceToDaemonSetStackStateRelation(namespaceExternalID, daemonSetExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes namespace to daemon set relation: %s -> %s", namespaceExternalID, daemonSetExternalID)

	relation := dsc.CreateRelation(namespaceExternalID, daemonSetExternalID, "encloses")

	log.Tracef("Created StackState namespace -> daemon set relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}
