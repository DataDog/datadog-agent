// +build kubeapiserver

package topologycollectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/extensions/v1beta1"
)

// IngressCollector implements the ClusterTopologyCollector interface.
type IngressCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan  chan<- *topology.Relation
	ClusterTopologyCollector
}

// NewIngressCollector
func NewIngressCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation,
	clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &IngressCollector{
		ComponentChan:            componentChannel,
		RelationChan:             relationChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (ic *IngressCollector) GetName() string {
	return "Ingress Collector"
}

// Collects and Published the Ingress Components
func (ic *IngressCollector) CollectorFunction() error {
	ingresses, err := ic.GetAPIClient().GetIngresses()
	if err != nil {
		return err
	}

	for _, in := range ingresses {
		component := ic.ingressToStackStateComponent(in)
		ic.ComponentChan <- component
		// submit relation to service name for correlation
		if in.Spec.Backend != nil && in.Spec.Backend.ServiceName != "" {
			serviceExternalID := ic.buildServiceExternalID(buildServiceID(in.Namespace, in.Spec.Backend.ServiceName))

			// publish the ingress -> service relation
			relation := ic.ingressToServiceStackStateRelation(component.ExternalID, serviceExternalID)
			ic.RelationChan <- relation
		}

		// submit relation to service name in the ingress rules for correlation
		for _, rules := range in.Spec.Rules {
			for _, path := range rules.HTTP.Paths {
				serviceExternalID := ic.buildServiceExternalID(buildServiceID(in.Namespace, path.Backend.ServiceName))

				// publish the ingress -> service relation
				relation := ic.ingressToServiceStackStateRelation(component.ExternalID, serviceExternalID)
				ic.RelationChan <- relation
			}
		}

	}

	return nil
}

// Creates a StackState deployment component from a Kubernetes / OpenShift Cluster
func (ic *IngressCollector) ingressToStackStateComponent(ingress v1beta1.Ingress) *topology.Component {
	log.Tracef("Mapping Ingress to StackState component: %s", ingress.String())

	tags := emptyIfNil(ingress.Labels)
	tags = ic.addClusterNameTag(tags)

	identifiers := make([]string, 0)

	for _, ingressPoints := range ingress.Status.LoadBalancer.Ingress {
		if ingressPoints.Hostname != "" {
			identifiers = append(identifiers, ic.buildEndpointExternalID(ingressPoints.Hostname))
		}
		if ingressPoints.IP != "" {
			identifiers = append(identifiers, ic.buildEndpointExternalID(ingressPoints.IP))
		}
	}

	ingressExternalID := ic.buildIngressExternalID(buildIngressID(ingress.Namespace, ingress.Name))
	component := &topology.Component{
		ExternalID: ingressExternalID,
		Type:       topology.Type{Name: "ingress"},
		Data: map[string]interface{}{
			"name":              ingress.Name,
			"creationTimestamp": ingress.CreationTimestamp,
			"tags":              tags,
			"namespace":         ingress.Namespace,
			"identifiers":       identifiers,
			"uid":               ingress.UID,
		},
	}

	component.Data.PutNonEmpty("generateName", ingress.GenerateName)
	component.Data.PutNonEmpty("kind", ingress.Kind)

	log.Tracef("Created StackState Ingress component %s: %v", ingressExternalID, component.JSONString())

	return component
}

// Creates a StackState component from a Kubernetes / OpenShift Ingress to Service
func (ic *IngressCollector) ingressToServiceStackStateRelation(ingressExternalID, serviceExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes ingress to service relation: %s -> %s", ingressExternalID, serviceExternalID)

	relation := ic.CreateRelation(ingressExternalID, serviceExternalID, "routes")

	log.Tracef("Created StackState ingress -> service relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// buildIngressID - combination of the ingress namespace and ingress name
func buildIngressID(ingressNamespace, ingressName string) string {
	return fmt.Sprintf("%s:%s", ingressNamespace, ingressName)
}
