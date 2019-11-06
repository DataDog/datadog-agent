// +build kubeapiserver

package topology_collectors

import (
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/extensions/v1beta1"
)

// IngressCollector implements the ClusterTopologyCollector interface.
type IngressCollector struct {
	ComponentChan             chan<- *topology.Component
	RelationChan              chan<- *topology.Relation
	ServiceCorrelationChannel chan<- *IngressToServiceCorrelation
	ClusterTopologyCollector
}

// NewIngressCollector
func NewIngressCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation,
	serviceCorrelationChannel chan<- *IngressToServiceCorrelation, clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &IngressCollector{
		ComponentChan:             componentChannel,
		RelationChan:              relationChannel,
		ServiceCorrelationChannel: serviceCorrelationChannel,
		ClusterTopologyCollector:  clusterTopologyCollector,
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
		if in.Spec.Backend.ServiceName != "" {
			serviceID := buildServiceID(in.Namespace, in.Spec.Backend.ServiceName)

			ic.ServiceCorrelationChannel <- &IngressToServiceCorrelation{
				ServiceID:         serviceID,
				IngressExternalID: component.ExternalID,
			}
		}

		// submit relation to service name in the ingress rules for correlation
		for _, rules := range in.Spec.Rules {
			for _, path := range rules.HTTP.Paths {
				serviceID := buildServiceID(in.Namespace, path.Backend.ServiceName)

				ic.ServiceCorrelationChannel <- &IngressToServiceCorrelation{
					ServiceID:         serviceID,
					IngressExternalID: component.ExternalID,
				}
			}
		}

	}

	// close the service correlation channel
	close(ic.ServiceCorrelationChannel)

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

	ingressExternalID := ic.buildIngressExternalID(ingress.Name)
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
