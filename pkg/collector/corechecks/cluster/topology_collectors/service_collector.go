// +build kubeapiserver

package topology_collectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/core/v1"
)

// ServiceCollector implements the ClusterTopologyCollector interface.
type ServiceCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan chan<- *topology.Relation
	ClusterTopologyCollector
}

// EndpointID contains the definition of a cluster ip
type EndpointID struct {
	URL           string
	RefExternalID string
}

// NewServiceCollector
func NewServiceCollector(componentChannel chan<- *topology.Component, relationChannel chan<- *topology.Relation,
	clusterTopologyCollector ClusterTopologyCollector) ClusterTopologyCollector {
	return &ServiceCollector{
		ComponentChan: componentChannel,
		RelationChan: relationChannel,
		ClusterTopologyCollector: clusterTopologyCollector,
	}
}

// GetName returns the name of the Collector
func (_ *ServiceCollector) GetName() string {
	return "Service Collector"
}

// Collects and Published the Service Components
func (sc *ServiceCollector) CollectorFunction() error {
	services, err := sc.GetAPIClient().GetServices()
	if err != nil {
		return err
	}

	endpoints, err := sc.GetAPIClient().GetEndpoints()
	if err != nil {
		return err
	}

	serviceEndpointIdentifiers := make(map[string][]EndpointID, 0)

	// Get all the endpoints for the Service
	for _, endpoint := range endpoints {
		serviceID := buildServiceID(endpoint.Namespace, endpoint.Name)
		for _, subset := range endpoint.Subsets {
			for _, address := range subset.Addresses {
				for _, port := range subset.Ports {
					endpointID := EndpointID{
						URL: fmt.Sprintf("%s:%d", address.IP, port.Port),
					}

					// check if the target reference is populated, so we can create relations
					if address.TargetRef != nil {
						switch kind := address.TargetRef.Kind; kind {
						// add endpoint url as identifier and create service -> pod relation
						case "Pod":
							endpointID.RefExternalID = sc.buildPodExternalID(sc.GetInstance().URL, address.TargetRef.Name)
						// ignore different Kind's for now, create no relation
						default:
						}
					}

					serviceEndpointIdentifiers[serviceID] = append(serviceEndpointIdentifiers[serviceID], endpointID)
				}
			}
		}
	}

	for _, service := range services {
		// creates and publishes StackState service component with relations
		serviceID := buildServiceID(service.Namespace, service.Name)
		serviceEndpoints := serviceEndpointIdentifiers[serviceID]
		component := sc.serviceToStackStateComponent(service, serviceEndpoints)

		sc.ComponentChan <- component

		for _, endpoint := range serviceEndpoints {
			// create the relation between the service and pod on the endpoint
			if endpoint.RefExternalID != "" {
				relation := sc.podToServiceStackStateRelation(component.ExternalID, endpoint.RefExternalID)

				sc.RelationChan <- relation
			}
		}

	}

	return nil
}

// Creates a StackState component from a Kubernetes / OpenShift Pod Service
func (sc *ServiceCollector) serviceToStackStateComponent(service v1.Service, endpoints []EndpointID) *topology.Component {
	log.Tracef("Mapping kubernetes pod service to StackState component: %s", service.String())
	// create identifier list to merge with StackState components
	var identifiers []string
	serviceID := buildServiceID(service.Namespace, service.Name)

	// all external ip's which are associated with this service, but are not managed by kubernetes
	for _, ip := range service.Spec.ExternalIPs {
		for _, port := range service.Spec.Ports {
			identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%d", ip, port.Port))
		}
	}

	// all endpoints for this service
	for _, endpoint := range endpoints {
		identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%s", sc.GetInstance().URL, endpoint.URL))
	}

	switch service.Spec.Type {
	case v1.ServiceTypeClusterIP:
		identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%s", sc.GetInstance().URL, service.Spec.ClusterIP))
	case v1.ServiceTypeLoadBalancer:
		identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%s", sc.GetInstance().URL, service.Spec.LoadBalancerIP))
	default:
	}

	log.Tracef("Created identifiers for %s: %v", service.Name, identifiers)

	serviceExternalID := sc.buildServiceExternalID(sc.GetInstance().URL, serviceID)

	tags := emptyIfNil(service.Labels)
	tags = sc.addClusterNameTag(tags)

	data := map[string]interface{}{
		"name":              service.Name,
		"namespace":         service.Namespace,
		"creationTimestamp": service.CreationTimestamp,
		"tags":              tags,
		"identifiers":       identifiers,
	}

	if service.Status.LoadBalancer.Ingress != nil {
		data["ingressPoints"] = service.Status.LoadBalancer.Ingress
	}

	component := &topology.Component{
		ExternalID: serviceExternalID,
		Type:       topology.Type{Name: "service"},
		Data:       data,
	}

	log.Tracef("Created StackState service component %s: %v", serviceExternalID, component.JSONString())

	return component
}

// Creates a StackState component from a Kubernetes / OpenShift Pod Service
func (sc *ServiceCollector) podToServiceStackStateRelation(refExternalID, serviceExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes reference to service relation: %s -> %s", refExternalID, serviceExternalID)

	relation := &topology.Relation{
		ExternalID: fmt.Sprintf("%s->%s", refExternalID, serviceExternalID),
		SourceID:   refExternalID,
		TargetID:   serviceExternalID,
		Type:       topology.Type{Name: "exposes"},
		Data:       map[string]interface{}{},
	}

	log.Tracef("Created StackState reference -> service relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// buildServiceID - combination of the service namespace and service name
func buildServiceID(serviceNamespace, serviceName string) string {
	return fmt.Sprintf("%s:%s", serviceNamespace, serviceName)
}
