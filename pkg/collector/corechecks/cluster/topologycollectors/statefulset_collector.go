// +build kubeapiserver

package topologycollectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/apps/v1"
)

// StatefulSetCollector implements the ClusterTopologyCollector interface.
type StatefulSetCollector struct {
	ComponentChan chan<- *topology.Component
	ClusterTopologyCollector
}

// NewStatefulSetCollector
func NewStatefulSetCollector(componentChannel chan<- *topology.Component, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &StatefulSetCollector{
		ComponentChan:            componentChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *StatefulSetCollector) GetName() string {
	return "StatefulSet Collector"
}

// Collects and Published the StatefulSet Components
func (ssc *StatefulSetCollector) CollectorFunction() error {
	statefulSets, err := ssc.GetAPIClient().GetStatefulSets()
	if err != nil {
		return err
	}

	for _, ss := range statefulSets {
		ssc.ComponentChan <- ssc.statefulSetToStackStateComponent(ss)
	}

	return nil
}

// Creates a StackState component from a Kubernetes / OpenShift Cluster
func (ssc *StatefulSetCollector) statefulSetToStackStateComponent(statefulSet v1.StatefulSet) *topology.Component {
	log.Tracef("Mapping StatefulSet to StackState component: %s", statefulSet.String())

	tags := emptyIfNil(statefulSet.Labels)
	tags = ssc.addClusterNameTag(tags)

	statefulSetExternalID := ssc.buildStatefulSetExternalID(statefulSet.Name)
	component := &topology.Component{
		ExternalID: statefulSetExternalID,
		Type:       topology.Type{Name: "statefulset"},
		Data: map[string]interface{}{
			"name":                statefulSet.Name,
			"creationTimestamp":   statefulSet.CreationTimestamp,
			"tags":                tags,
			"namespace":           statefulSet.Namespace,
			"updateStrategy":      statefulSet.Spec.UpdateStrategy.Type,
			"desiredReplicas":     statefulSet.Spec.Replicas,
			"podManagementPolicy": statefulSet.Spec.PodManagementPolicy,
			"serviceName":         statefulSet.Spec.ServiceName,
			"uid":                 statefulSet.UID,
		},
	}

	component.Data.PutNonEmpty("generateName", statefulSet.GenerateName)
	component.Data.PutNonEmpty("kind", statefulSet.Kind)

	log.Tracef("Created StackState StatefulSet component %s: %v", statefulSetExternalID, component.JSONString())

	return component
}
