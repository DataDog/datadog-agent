// +build kubeapiserver

package topologycollectors

import (
	"fmt"

	"github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/dns"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	v1 "k8s.io/api/core/v1"
)

// ServiceCollector implements the ClusterTopologyCollector interface.
type ServiceCollector struct {
	ComponentChan chan<- *topology.Component
	RelationChan  chan<- *topology.Relation
	ClusterTopologyCollector
	DNS dns.Resolver
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
		DNS:                      dns.StandardResolver,
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
						// add endpoint url as identifier, will be used for service -> pod relation
						case "Pod":
							if address.TargetRef.Namespace != "" {
								endpointID.RefExternalID = sc.buildPodExternalID(address.TargetRef.Namespace, address.TargetRef.Name)
							} else {
								endpointID.RefExternalID = sc.buildPodExternalID(endpoint.Namespace, address.TargetRef.Name)
							}
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
		component := sc.serviceToStackStateComponent(service)

		sc.ComponentChan <- component

		// Check whether we have an ExternalName service, which will result in an extra component+relation
		if service.Spec.Type == v1.ServiceTypeExternalName {
			externalService := sc.serviceToExternalServiceComponent(service)

			sc.ComponentChan <- externalService
			sc.RelationChan <- sc.serviceToExternalServiceStackStateRelation(component.ExternalID, externalService.ExternalID)
		}

		// First ensure we publish all components, else the test becomes complex
		sc.RelationChan <- sc.namespaceToServiceStackStateRelation(sc.buildNamespaceExternalID(service.Namespace), component.ExternalID)

		publishedPodRelations := make(map[string]string, 0)
		for _, endpoint := range serviceEndpointIdentifiers[serviceID] {
			// create the relation between the service and the pod
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
func (sc *ServiceCollector) serviceToStackStateComponent(service v1.Service) *topology.Component {
	log.Tracef("Mapping kubernetes pod service to StackState component: %s", service.String())
	// create identifier list to merge with StackState components
	identifiers := make([]string, 0)

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

	switch service.Spec.Type {
	// identifier for
	case v1.ServiceTypeClusterIP:
		// verify that the cluster ip is not empty
		if service.Spec.ClusterIP != "None" && service.Spec.ClusterIP != "" {
			identifiers = append(identifiers, sc.buildEndpointExternalID(service.Spec.ClusterIP))
		}
	case v1.ServiceTypeNodePort:
		// verify that the node port is not empty
		if service.Spec.ClusterIP != "None" && service.Spec.ClusterIP != "" {
			identifiers = append(identifiers, sc.buildEndpointExternalID(service.Spec.ClusterIP))
			// map all of the node ports for the ip
			for _, port := range service.Spec.Ports {
				// map all the node ports
				if port.NodePort != 0 {
					identifiers = append(identifiers, sc.buildEndpointExternalID(fmt.Sprintf("%s:%d", service.Spec.ClusterIP, port.NodePort)))
				}
			}
		}
	case v1.ServiceTypeLoadBalancer:
		// verify that the load balance ip is not empty
		if service.Spec.LoadBalancerIP != "" {
			identifiers = append(identifiers, sc.buildEndpointExternalID(service.Spec.LoadBalancerIP))
		}
		// verify that the cluster ip is not empty
		if service.Spec.ClusterIP != "None" && service.Spec.ClusterIP != "" {
			identifiers = append(identifiers, sc.buildEndpointExternalID(service.Spec.ClusterIP))
		}
	case v1.ServiceTypeExternalName:
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

	// add identifier for this service name
	serviceID := buildServiceID(service.Namespace, service.Name)
	identifiers = append(identifiers, fmt.Sprintf("urn:service:/%s:%s", sc.GetInstance().URL, serviceID))

	log.Tracef("Created identifiers for %s: %v", service.Name, identifiers)

	serviceExternalID := sc.buildServiceExternalID(service.Namespace, service.Name)

	tags := sc.initTags(service.ObjectMeta)
	tags["service-type"] = string(service.Spec.Type)

	if service.Spec.ClusterIP == "None" {
		tags["service"] = "headless"
	}

	component := &topology.Component{
		ExternalID: serviceExternalID,
		Type:       topology.Type{Name: "service"},
		Data: map[string]interface{}{
			"name":              service.Name,
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

func (sc *ServiceCollector) serviceToExternalServiceComponent(service v1.Service) *topology.Component {
	log.Tracef("Mapping kubernetes pod ExternalName service to extra StackState component: %s", service.String())
	// create identifier list to merge with StackState components
	identifiers := make([]string, 0)

	if service.Spec.ExternalName != "None" && service.Spec.ExternalName != "" {
		identifiers = append(identifiers, fmt.Sprintf("urn:endpoint:/%s", service.Spec.ExternalName))
		// If targetPorts are specified, use those
		for _, port := range service.Spec.Ports {
			// map all the node ports
			if port.Port != 0 {
				identifiers = append(identifiers, sc.buildEndpointExternalID(fmt.Sprintf("%s:%d", service.Spec.ExternalName, port.Port)))
			}
		}

		addrs, err := sc.DNS(service.Spec.ExternalName)
		if err != nil {
			log.Warnf("Could not lookup IP addresses for host '%s' (Error: %s)", service.Spec.ExternalName, err.Error())
		} else {
			for _, addr := range addrs {
				identifiers = append(identifiers, sc.buildEndpointExternalID(addr))
				// If targetPorts are specified, use those
				for _, port := range service.Spec.Ports {
					// map all the node ports
					if port.Port != 0 {
						identifiers = append(identifiers, sc.buildEndpointExternalID(fmt.Sprintf("%s:%d", addr, port.Port)))
					}
				}
			}
		}
	}

	// add identifier for this service name
	serviceID := buildServiceID(service.Namespace, service.Name)
	identifiers = append(identifiers, fmt.Sprintf("urn:external-service:/%s:%s", sc.GetInstance().URL, serviceID))

	log.Tracef("Created identifiers for %s: %v", service.Name, identifiers)

	externalID := sc.GetURNBuilder().BuildComponentExternalID("external-service", service.Namespace, service.Name)

	tags := sc.initTags(service.ObjectMeta)

	component := &topology.Component{
		ExternalID: externalID,
		Type:       topology.Type{Name: "external-service"},
		Data: map[string]interface{}{
			"name":              service.Name,
			"creationTimestamp": service.CreationTimestamp,
			"tags":              tags,
			"identifiers":       identifiers,
			"uid":               service.UID,
		},
	}

	log.Tracef("Created StackState external-service component %s: %v", externalID, component.JSONString())

	return component
}

// Creates a StackState relation from a Kubernetes / OpenShift Service to Pod
func (sc *ServiceCollector) serviceToPodStackStateRelation(serviceExternalID, podExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes pod to service relation: %s -> %s", podExternalID, serviceExternalID)

	relation := sc.CreateRelation(serviceExternalID, podExternalID, "exposes")

	log.Tracef("Created StackState service -> pod relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState relation from a Kubernetes / OpenShift Service to Namespace relation
func (sc *ServiceCollector) namespaceToServiceStackStateRelation(namespaceExternalID, serviceExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes namespace to service relation: %s -> %s", namespaceExternalID, serviceExternalID)

	relation := sc.CreateRelation(namespaceExternalID, serviceExternalID, "encloses")

	log.Tracef("Created StackState namespace -> service relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// Creates a StackState relation from a Kubernetes / OpenShift Service to 'ExternalService' relation
func (sc *ServiceCollector) serviceToExternalServiceStackStateRelation(serviceExternalID, externalServiceExternalID string) *topology.Relation {
	log.Tracef("Mapping kubernetes service to external service relation: %s -> %s", serviceExternalID, externalServiceExternalID)

	relation := sc.CreateRelation(serviceExternalID, externalServiceExternalID, "uses")

	log.Tracef("Created StackState service -> external service relation %s->%s", relation.SourceID, relation.TargetID)

	return relation
}

// buildServiceID - combination of the service namespace and service name
func buildServiceID(serviceNamespace, serviceName string) string {
	return fmt.Sprintf("%s:%s", serviceNamespace, serviceName)
}
