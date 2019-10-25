// +build kubeapiserver

package topology_collectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/extensions/v1beta1"
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
func (ic *IngressCollector) CollectorFunction() error {
	ingresses, err := ic.GetAPIClient().GetIngresses()
	if err != nil {
		return err
	}

	for _, in := range ingresses {
		ic.ComponentChan <- ic.ingressToStackStateComponent(in)
	}

	return nil
}

// Creates a StackState deployment component from a Kubernetes / OpenShift Cluster
func (dmc *IngressCollector) ingressToStackStateComponent(ingress v1beta1.Ingress) *topology.Component {
	log.Tracef("Mapping Ingress to StackState component: %s", ingress.String())

	tags := emptyIfNil(ingress.Labels)
	tags = dmc.addClusterNameTag(tags)

	identifiers := make([]string, 0)

	for _, ingressPoints := range ingress.Status.LoadBalancer.Ingress {
		if ingressPoints.Hostname != "" {
			identifiers = append(identifiers, dmc.buildEndpointID(ingressPoints.Hostname))
		}
		if ingressPoints.IP != "" {
			identifiers = append(identifiers, dmc.buildEndpointID(ingressPoints.IP))
		}
	}

	ingressExternalID := dmc.buildIngressExternalID(ingress.Name)
	component := &topology.Component{
		ExternalID: ingressExternalID,
		Type:       topology.Type{Name: "ingress"},
		Data: map[string]interface{}{
			"name":              ingress.Name,
			"kind":              ingress.Kind,
			"creationTimestamp": ingress.CreationTimestamp,
			"tags":              tags,
			"namespace":         ingress.Namespace,
			"identifiers": identifiers,
			"uid":           ingress.UID,
			"generateName":  ingress.GenerateName,
		},
	}

	log.Tracef("Created StackState Ingress component %s: %v", ingressExternalID, component.JSONString())

	return component
}
