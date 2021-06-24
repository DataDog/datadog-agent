// +build kubeapiserver

package topologycollectors

import (
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
func (*IngressCollector) GetName() string {
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
			serviceExternalID := ic.buildServiceExternalID(in.Namespace, in.Spec.Backend.ServiceName)

			// publish the ingress -> service relation
			relation := ic.ingressToServiceStackStateRelation(component.ExternalID, serviceExternalID)
			ic.RelationChan <- relation
		}

		// submit relation to service name in the ingress rules for correlation
		for _, rules := range in.Spec.Rules {
			for _, path := range rules.HTTP.Paths {
				serviceExternalID := ic.buildServiceExternalID(in.Namespace, path.Backend.ServiceName)

				// publish the ingress -> service relation
				relation := ic.ingressToServiceStackStateRelation(component.ExternalID, serviceExternalID)
				ic.RelationChan <- relation
			}
		}

		// submit relation to loadbalancer
		for _, ingressPoints := range in.Status.LoadBalancer.Ingress {
			if ingressPoints.Hostname != "" {
				endpoint := ic.endpointStackStateComponentFromIngress(in, ingressPoints.Hostname)

				ic.ComponentChan <- endpoint
				ic.RelationChan <- ic.endpointToIngressStackStateRelation(endpoint.ExternalID, component.ExternalID)
			}

			if ingressPoints.IP != "" {
				endpoint := ic.endpointStackStateComponentFromIngress(in, ingressPoints.IP)

				ic.ComponentChan <- endpoint
				ic.RelationChan <- ic.endpointToIngressStackStateRelation(endpoint.ExternalID, component.ExternalID)
			}
		}
	}

	return nil
}

// Creates a StackState ingress component from a Kubernetes / OpenShift Ingress
func (ic *IngressCollector) ingressToStackStateComponent(ingress v1beta1.Ingress) *topology.Component {
	log.Tracef("Mapping Ingress to StackState component: %s", ingress.String())

	tags := ic.initTags(ingress.ObjectMeta)

	identifiers := make([]string, 0)

	ingressExternalID := ic.buildIngressExternalID(ingress.Namespace, ingress.Name)
	component := &topology.Component{
		ExternalID: ingressExternalID,
		Type:       topology.Type{Name: "ingress"},
		Data: map[string]interface{}{
			"name":              ingress.Name,
			"creationTimestamp": ingress.CreationTimestamp,
			"tags":              tags,
			"identifiers":       identifiers,
			"uid":               ingress.UID,
		},
	}

	component.Data.PutNonEmpty("generateName", ingress.GenerateName)
	component.Data.PutNonEmpty("kind", ingress.Kind)

	log.Tracef("Created StackState Ingress component %s: %v", ingressExternalID, component.JSONString())

	return component
}

// Creates a StackState loadbalancer component from a Kubernetes / OpenShift Ingress
func (ic *IngressCollector) endpointStackStateComponentFromIngress(ingress v1beta1.Ingress, ingressPoint string) *topology.Component {
	log.Tracef("Mapping Ingress to StackState endpoint component: %s", ingressPoint)

	tags := ic.initTags(ingress.ObjectMeta)
	identifiers := make([]string, 0)
	endpointExternalID := ic.buildEndpointExternalID(ingressPoint)

	component := &topology.Component{
		ExternalID: endpointExternalID,
		Type:       topology.Type{Name: "endpoint"},
		Data: map[string]interface{}{
			"name":              ingressPoint,
			"creationTimestamp": ingress.CreationTimestamp,
			"tags":              tags,
			"identifiers":       identifiers,
		},
	}

	log.Tracef("Created StackState endpoint component %s: %v", endpointExternalID, component.JSONString())

	return component
}

// Creates a StackState relation from a Kubernetes / OpenShift Ingress to Service
func (ic *IngressCollector) ingressToServiceStackStateRelation(ingressExternalID, serviceExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes ingress to service relation: %s -> %s", ingressExternalID, serviceExternalID)

	relation := ic.CreateRelation(ingressExternalID, serviceExternalID, "routes")

	log.Tracef("Created StackState ingress -> service relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState relation from an Endpoint to a Kubernetes / OpenShift Ingress
func (ic *IngressCollector) endpointToIngressStackStateRelation(endpointExternalID, ingressExternalID string) *topology.Relation {
	log.Tracef("Mapping endpoint to kubernetes ingress relation: %s -> %s", endpointExternalID, ingressExternalID)

	relation := ic.CreateRelation(endpointExternalID, ingressExternalID, "routes")

	log.Tracef("Created endpoint -> StackState ingress relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}
