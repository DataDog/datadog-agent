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
	ClusterTopologyCollector
}

// NewDaemonSetCollector
func NewDaemonSetCollector(componentChannel chan<- *topology.Component, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &DaemonSetCollector{
		ComponentChan:            componentChannel,
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
		dsc.ComponentChan <- dsc.daemonSetToStackStateComponent(ds)
	}

	return nil
}

// Creates a StackState daemonset component from a Kubernetes / OpenShift Cluster
func (dsc *DaemonSetCollector) daemonSetToStackStateComponent(daemonSet v1.DaemonSet) *topology.Component {
	log.Tracef("Mapping DaemonSet to StackState component: %s", daemonSet.String())

	tags := emptyIfNil(daemonSet.Labels)
	tags = dsc.addClusterNameTag(tags)

	daemonSetExternalID := dsc.buildDaemonSetExternalID(daemonSet.Name)
	component := &topology.Component{
		ExternalID: daemonSetExternalID,
		Type:       topology.Type{Name: "daemonset"},
		Data: map[string]interface{}{
			"name":              daemonSet.Name,
			"creationTimestamp": daemonSet.CreationTimestamp,
			"tags":              tags,
			"namespace":         daemonSet.Namespace,
			"updateStrategy":    daemonSet.Spec.UpdateStrategy.Type,
			"uid":               daemonSet.UID,
		},
	}

	component.Data.PutNonEmpty("generateName", daemonSet.GenerateName)
	component.Data.PutNonEmpty("kind", daemonSet.Kind)

	log.Tracef("Created StackState DaemonSet component %s: %v", daemonSetExternalID, component.JSONString())

	return component
}
