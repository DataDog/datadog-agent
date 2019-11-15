// +build kubeapiserver

package topologycollectors

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"k8s.io/api/core/v1"
)

// ServiceCollector implements the ClusterTopologyCollector interface.
type ServiceCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan  chan<- *topology.Relation
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
		ComponentChan:            componentChannel,
		RelationChan:             relationChannel,
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
							endpointID.RefExternalID = sc.buildPodExternalID(address.TargetRef.Name)
						// ignore different Kind's for now, create no relation
						default:
						}
					}

					serviceEndpointIdentifiers[serviceID] = append(serviceEndpointIdentifiers[serviceID], endpointID)
				}
			}
		}
	}

	serviceMap := make(map[string][]string)

	for _, service := range services {
		// creates and publishes StackState service component with relations
		serviceID := buildServiceID(service.Namespace, service.Name)
		serviceEndpoints := serviceEndpointIdentifiers[serviceID]
		component := sc.serviceToStackStateComponent(service, serviceEndpoints)

		sc.ComponentChan <- component

		publishedPodRelations := make(map[string]string, 0)
		for _, endpoint := range serviceEndpoints {
			// create the relation between the pod and the service on the endpoint
			podExternalID := endpoint.RefExternalID
			relationExternalID := fmt.Sprintf("%s->%s", component.ExternalID, podExternalID)

			_, ok := publishedPodRelations[relationExternalID]
			if !ok && podExternalID != "" {
				relation := sc.serviceToPodStackStateRelation(component.ExternalID, podExternalID)

				sc.RelationChan <- relation

				publishedPodRelations[relationExternalID] = relationExternalID
			}
		}

		serviceMap[serviceID] = append(serviceMap[serviceID], component.ExternalID)
	}

	return nil
}

// Creates a StackState component from a Kubernetes / OpenShift Service
func (sc *ServiceCollector) serviceToStackStateComponent(service v1.Service, endpoints []EndpointID) *topology.Component {
	log.Tracef("Mapping kubernetes pod service to StackState component: %s", service.String())
	// create identifier list to merge with StackState components
	var identifiers []string
	serviceID := buildServiceID(service.Namespace, service.Name)

	// all external ip's which are associated with this service, but are not managed by kubernetes
	for _, ip := range service.Spec.ExternalIPs {
		// verify that the ip is not empty
		if ip == "" {
			continue
		}
		// map all of the ports for the ip
		for _, port := range service.Spec.Ports {
			identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%d", ip, port.Port))

			if service.Spec.Type == v1.ServiceTypeNodePort && port.NodePort != 0 {
				identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%d", ip, port.NodePort))
			}
		}
	}

	// all endpoints for this service
	for _, endpoint := range endpoints {
		// verify that the endpoint url is not empty
		if endpoint.URL == "" {
			continue
		}
		identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%s", sc.GetInstance().URL, endpoint.URL))
	}

	switch service.Spec.Type {
	// identifier for
	case v1.ServiceTypeClusterIP:
		// verify that the cluster ip is not empty
		if service.Spec.ClusterIP != "" {
			identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%s", sc.GetInstance().URL, service.Spec.ClusterIP))
		}
	case v1.ServiceTypeNodePort:
		// verify that the node port is not empty
		if service.Spec.ClusterIP != "" {
			identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%s", sc.GetInstance().URL, service.Spec.ClusterIP))
			// map all of the node ports for the ip
			for _, port := range service.Spec.Ports {
				// map all the node ports
				if port.NodePort != 0 {
					identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%s:%d", sc.GetInstance().URL, service.Spec.ClusterIP, port.NodePort))
				}
			}
		}
	case v1.ServiceTypeLoadBalancer:
		// verify that the load balance ip is not empty
		if service.Spec.LoadBalancerIP != "" {
			identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%s", sc.GetInstance().URL, service.Spec.LoadBalancerIP))
		}
		// verify that the cluster ip is not empty
		if service.Spec.ClusterIP != "" {
			identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s:%s", sc.GetInstance().URL, service.Spec.ClusterIP))
		}
	default:
	}

	for _, inpoint := range service.Status.LoadBalancer.Ingress {
		if inpoint.IP != "" {
			identifiers = append(identifiers, fmt.Sprintf("urn:ingress-point:/%s", inpoint.IP))
		}

		if inpoint.Hostname != "" {
			identifiers = append(identifiers, fmt.Sprintf("urn:ingress-point:/%s", inpoint.Hostname))

		}
	}

	log.Tracef("Created identifiers for %s: %v", service.Name, identifiers)

	serviceExternalID := sc.buildServiceExternalID(serviceID)

	tags := emptyIfNil(service.Labels)
	tags = sc.addClusterNameTag(tags)

	component := &topology.Component{
		ExternalID: serviceExternalID,
		Type:       topology.Type{Name: "service"},
		Data: map[string]interface{}{
			"name":              service.Name,
			"namespace":         service.Namespace,
			"creationTimestamp": service.CreationTimestamp,
			"tags":              tags,
			"identifiers":       identifiers,
			"uid":               service.UID,
		},
	}

	component.Data.PutNonEmpty("kind", service.Kind)
	component.Data.PutNonEmpty("generateName", service.GenerateName)

	log.Tracef("Created StackState service component %s: %v", serviceExternalID, component.JSONString())

	return component
}

// Creates a StackState relation from a Kubernetes / OpenShift Service to Pod
func (sc *ServiceCollector) serviceToPodStackStateRelation(serviceExternalID, podExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes pod to service relation: %s -> %s", podExternalID, serviceExternalID)

	relation := sc.CreateRelation(serviceExternalID, podExternalID, "exposes")

	log.Tracef("Created StackState service -> pod relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// buildServiceID - combination of the service namespace and service name
func buildServiceID(serviceNamespace, serviceName string) string {
	return fmt.Sprintf("%s:%s", serviceNamespace, serviceName)
}
