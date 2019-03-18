// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package listeners

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	infov1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	kubeServiceAnnotationFormat  = "ad.datadoghq.com/service.instances"
	kubeEndpointAnnotationFormat = "ad.datadoghq.com/endpoints.instances"
	kubeEndpointIDPrefix         = "kube_endpoint://"
)

// KubeServiceListener listens to kubernetes service creation
type KubeServiceListener struct {
	servicesInformer   infov1.ServiceInformer
	endpointsInformer  infov1.EndpointsInformer
	services           map[types.UID]Service
	endpoints          map[string][]*KubeEndpointService
	endpointsAnnotated map[string]bool
	newService         chan<- Service
	delService         chan<- Service
	m                  sync.RWMutex
}

// KubeServiceService represents a Kubernetes Service
type KubeServiceService struct {
	entity       string
	tags         []string
	hosts        map[string]string
	ports        []ContainerPort
	creationTime integration.CreationTime
}

// KubeEndpointService represents a Kubernetes Endpoint
type KubeEndpointService struct {
	entity       string
	tags         []string
	hosts        map[string]string
	ports        []ContainerPort
	creationTime integration.CreationTime
	adID         string
}

func init() {
	Register("kube_services", NewKubeServiceListener)
}

func NewKubeServiceListener() (ServiceListener, error) {
	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}
	servicesInformer := ac.InformerFactory.Core().V1().Services()
	if servicesInformer == nil {
		return nil, fmt.Errorf("cannot get service informer: %s", err)
	}
	endpointsInformer := ac.InformerFactory.Core().V1().Endpoints()
	if endpointsInformer == nil {
		return nil, fmt.Errorf("cannot get endpoint informer: %s", err)
	}
	return &KubeServiceListener{
		services:           make(map[types.UID]Service),
		endpoints:          make(map[string][]*KubeEndpointService),
		endpointsAnnotated: make(map[string]bool),
		servicesInformer:   servicesInformer,
		endpointsInformer:  endpointsInformer,
	}, nil
}

func (l *KubeServiceListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	l.servicesInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    l.addedSvc,
		UpdateFunc: l.updatedSvc,
		DeleteFunc: l.deletedSvc,
	})

	l.endpointsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    l.addedEndpt,
		UpdateFunc: l.updatedEndpt,
		DeleteFunc: l.deletedEndpt,
	})

	// Initial fill
	services, err := l.servicesInformer.Lister().List(labels.Everything())
	if err != nil {
		log.Errorf("Cannot list Kubernetes services: %s", err)
	}
	for _, s := range services {
		l.createService(s, true)
	}
	endpoints, err := l.endpointsInformer.Lister().List(labels.Everything())
	if err != nil {
		log.Errorf("Cannot list Kubernetes endpoints: %s", err)
	}
	for _, e := range endpoints {
		l.createEndpoint(e, true)
	}
}

// Stop is a stub
func (l *KubeServiceListener) Stop() {
	// We cannot deregister from the informer
}

func (l *KubeServiceListener) addedSvc(obj interface{}) {
	castedObj, ok := obj.(*v1.Service)
	if !ok {
		log.Errorf("Expected a Service type, got: %v", obj)
		return
	}
	l.createService(castedObj, false)
}

func (l *KubeServiceListener) deletedSvc(obj interface{}) {
	castedObj, ok := obj.(*v1.Service)
	if !ok {
		log.Errorf("Expected a Service type, got: %v", obj)
		return
	}
	l.removeService(castedObj)
}

func (l *KubeServiceListener) updatedSvc(old, obj interface{}) {
	// Cast the updated object or return on failure
	castedObj, ok := obj.(*v1.Service)
	if !ok {
		log.Errorf("Expected a Service type, got: %v", obj)
		return
	}
	// Cast the old object, consider it an add on cast failure
	castedOld, ok := old.(*v1.Service)
	if !ok {
		log.Errorf("Expected a Service type, got: %v", old)
		l.createService(castedObj, false)
		return
	}
	if servicesDiffer(castedObj, castedOld) {
		l.removeService(castedObj)
		l.createService(castedObj, false)
	}
}

// servicesDiffer compares two services to only go forward
// when relevant fields are changed. This logic must be
// updated if more fields are used.
func servicesDiffer(first, second *v1.Service) bool {
	// Quick exit if resversion did not change
	if first.ResourceVersion == second.ResourceVersion {
		return false
	}
	// AD annotations
	if isServiceAnnotated(first) != isServiceAnnotated(second) {
		return true
	}
	if isEndpointAnnotated(first) != isEndpointAnnotated(second) {
		return true
	}
	// Cluster IP
	if first.Spec.ClusterIP != second.Spec.ClusterIP {
		return true
	}
	// Ports
	if len(first.Spec.Ports) != len(second.Spec.Ports) {
		return true
	}
	for i := range first.Spec.Ports {
		if first.Spec.Ports[i].Name != second.Spec.Ports[i].Name {
			return true
		}
		if first.Spec.Ports[i].Port != second.Spec.Ports[i].Port {
			return true
		}
	}
	// No relevant change
	return false
}

func (l *KubeServiceListener) createService(ksvc *v1.Service, firstRun bool) {
	if ksvc == nil {
		return
	}
	if !isServiceAnnotated(ksvc) {
		// Ignore services with no AD annotation
		return
	}

	svc := processService(ksvc, firstRun)

	l.m.Lock()
	l.services[ksvc.UID] = svc
	l.endpointsAnnotated[fmt.Sprintf("%s/%s", ksvc.Namespace, ksvc.Name)] = isEndpointAnnotated(ksvc)
	l.m.Unlock()

	l.newService <- svc
}

func processService(ksvc *v1.Service, firstRun bool) *KubeServiceService {
	svc := &KubeServiceService{
		entity:       apiserver.EntityForService(ksvc),
		creationTime: integration.After,
	}
	if firstRun {
		svc.creationTime = integration.Before
	}

	// Tags, static for now
	svc.tags = []string{
		fmt.Sprintf("kube_service:%s", ksvc.Name),
		fmt.Sprintf("kube_namespace:%s", ksvc.Namespace),
	}

	// Hosts, only use internal ClusterIP for now
	svc.hosts = map[string]string{"cluster": ksvc.Spec.ClusterIP}

	// Ports
	var ports []ContainerPort
	for _, port := range ksvc.Spec.Ports {
		ports = append(ports, ContainerPort{int(port.Port), port.Name})
	}
	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})
	svc.ports = ports
	if len(svc.ports) == 0 {
		// Port might not be specified in pod spec
		log.Debugf("No ports found for service %s", ksvc.Name)
	}

	return svc
}

func (l *KubeServiceListener) removeService(ksvc *v1.Service) {
	if ksvc == nil {
		return
	}
	l.m.RLock()
	svc, ok := l.services[ksvc.UID]
	l.m.RUnlock()

	if ok {
		l.m.Lock()
		delete(l.services, ksvc.UID)
		delete(l.endpointsAnnotated, fmt.Sprintf("%s/%s", ksvc.Namespace, ksvc.Name))
		l.m.Unlock()

		l.delService <- svc

	} else {
		log.Debugf("Entity %s not found, not removing", ksvc.UID)
	}
}

func (l *KubeServiceListener) addedEndpt(obj interface{}) {
	castedObj, ok := obj.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected a Endpoints type, got: %v", obj)
		return
	}
	l.createEndpoint(castedObj, false)
}

func (l *KubeServiceListener) deletedEndpt(obj interface{}) {
	castedObj, ok := obj.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected a Endpoint type, got: %v", obj)
		return
	}
	l.removeEndpoint(castedObj)
}

func (l *KubeServiceListener) updatedEndpt(old, obj interface{}) {
	// Cast the updated object or return on failure
	castedObj, ok := obj.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected a Endpoints type, got: %v", obj)
		return
	}
	// Cast the old object, consider it an add on cast failure
	castedOld, ok := old.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected a Endpoints type, got: %v", old)
		l.createEndpoint(castedObj, false)
		return
	}
	if endpointsDiffer(castedObj, castedOld) {
		l.removeEndpoint(castedObj)
		l.createEndpoint(castedObj, false)
	}
}

// endpointsDiffer compares two endpoints to only go forward
// when relevant fields are changed. This logic must be
// updated if more fields are used.
func endpointsDiffer(first, second *v1.Endpoints) bool {
	// Quick exit if resversion did not change
	if first.ResourceVersion == second.ResourceVersion {
		return false
	}

	// Subsets
	if len(first.Subsets) != len(second.Subsets) {
		return true
	}

	// Addresses and Ports
	for i := range first.Subsets {
		if len(first.Subsets[i].Addresses) != len(second.Subsets[i].Addresses) {
			return true
		}
		if len(first.Subsets[i].Ports) != len(second.Subsets[i].Ports) {
			return true
		}
		// Addresses
		for j := range first.Subsets[i].Addresses {
			if first.Subsets[i].Addresses[j].IP != second.Subsets[i].Addresses[j].IP {
				return true
			}
			if first.Subsets[i].Addresses[j].Hostname != second.Subsets[i].Addresses[j].Hostname {
				return true
			}
		}
		// Ports
		for j := range first.Subsets[i].Ports {
			if first.Subsets[i].Ports[j].Port != second.Subsets[i].Ports[j].Port {
				return true
			}
			if first.Subsets[i].Ports[j].Name != second.Subsets[i].Ports[j].Name {
				return true
			}
		}
	}
	// No relevant change
	return false
}

func (l *KubeServiceListener) createEndpoint(kendpt *v1.Endpoints, firstRun bool) {
	if kendpt == nil {
		return
	}
	endptId := fmt.Sprintf("%s/%s", kendpt.Namespace, kendpt.Name)

	l.m.Lock()
	for id, isAnnotated := range l.endpointsAnnotated {
		if id == endptId && isAnnotated {
			endpts := processEndpoint(kendpt, firstRun)
			newEndpts, removedEndpts := diffEndpoints(endpts, l.endpoints[endptId])
			l.updateEndpoints(newEndpts, removedEndpts, endptId)
			for _, endpt := range newEndpts {
				log.Debugf("sending new endpoint: %v", endpt)
				l.newService <- endpt
			}
			for _, endpt := range removedEndpts {
				log.Debugf("removing endpoint: %v", endpt)
				l.delService <- endpt
			}
		}
	}
	l.m.Unlock()
}

func processEndpoint(kendpt *v1.Endpoints, firstRun bool) []*KubeEndpointService {
	var newEndpointServices []*KubeEndpointService

	// Hosts and Ports
	var hosts = make(map[string]string)
	var ports []ContainerPort
	for i := range kendpt.Subsets {
		// Hosts
		for _, host := range kendpt.Subsets[i].Addresses {
			hosts[host.IP] = host.Hostname
		}
		// Ports
		for _, port := range kendpt.Subsets[i].Ports {
			ports = append(ports, ContainerPort{int(port.Port), port.Name})
		}
	}

	for ip, host := range hosts {
		// create a separate AD service per host
		endpt := &KubeEndpointService{
			entity:       apiserver.EntityForEndpoints(kendpt.Namespace, kendpt.Name, ip),
			creationTime: integration.After,
			hosts:        map[string]string{ip: host},
			ports:        ports,
			tags: []string{
				fmt.Sprintf("kube_endpoint:%s", kendpt.Name),
				fmt.Sprintf("kube_namespace:%s", kendpt.Namespace),
			},
			adID: fmt.Sprintf("%s%s/%s", kubeEndpointIDPrefix, kendpt.Namespace, kendpt.Name),
		}
		if firstRun {
			endpt.creationTime = integration.Before
		}
		newEndpointServices = append(newEndpointServices, endpt)
	}
	return newEndpointServices
}

func diffEndpoints(current, old []*KubeEndpointService) ([]*KubeEndpointService, []*KubeEndpointService) {
	var new []*KubeEndpointService
	var removed []*KubeEndpointService
	for _, endpt := range current {
		// looking for new endpoints
		if !containsEndpointService(old, endpt) {
			new = append(new, endpt)
		}
	}
	for _, endpt := range old {
		// looking for removed endpoints
		if !containsEndpointService(current, endpt) {
			removed = append(removed, endpt)
		}
	}

	return new, removed
}

// containsEndpointService return true if a slice of endpoints services contain a given endpoint service
func containsEndpointService(svcs []*KubeEndpointService, svc *KubeEndpointService) bool {
	for _, s := range svcs {
		if reflect.DeepEqual(*s, *svc) {
			return true
		}
	}
	return false
}

// updateEndopoints uses differences in endpoints detected in diffEndpoints function to update the listener's endpoints map
func (l *KubeServiceListener) updateEndpoints(new, removed []*KubeEndpointService, endptId string) {
	endpts, found := l.endpoints[endptId]
	if found {
		// clean removed endpoints endpoints map
		tmp := endpts[:0]
		for _, endpt := range endpts {
			if !containsEndpointService(removed, endpt) {
				tmp = append(tmp, endpt)
			}
		}
		l.endpoints[endptId] = tmp
	}
	// append new endpoints
	l.endpoints[endptId] = append(l.endpoints[endptId], new...)
}

func (l *KubeServiceListener) removeEndpoint(kendpt *v1.Endpoints) {
	if kendpt == nil {
		return
	}
	endptId := fmt.Sprintf("%s/%s", kendpt.Namespace, kendpt.Name)

	l.m.RLock()
	endpts, ok := l.endpoints[endptId]
	l.m.RUnlock()

	if ok {
		l.m.Lock()
		delete(l.endpoints, endptId)
		l.m.Unlock()

		for _, endpt := range endpts {
			l.delService <- endpt
		}

	} else {
		log.Debugf("Entity %s not found, not removing", kendpt.UID)
	}
}

// GetEntity returns the unique entity name linked to that service
func (s *KubeServiceService) GetEntity() string {
	return s.entity
}

// GetADIdentifiers returns the service AD identifiers
func (s *KubeServiceService) GetADIdentifiers() ([]string, error) {
	// Only the entity for now, to match on annotation
	return []string{s.entity}, nil
}

// GetHosts returns the pod hosts
func (s *KubeServiceService) GetHosts() (map[string]string, error) {
	return s.hosts, nil
}

// GetPid is not supported for PodContainerService
func (s *KubeServiceService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

// GetPorts returns the container's ports
func (s *KubeServiceService) GetPorts() ([]ContainerPort, error) {
	return s.ports, nil
}

// GetTags retrieves tags
func (s *KubeServiceService) GetTags() ([]string, error) {
	return s.tags, nil
}

// GetHostname returns nil and an error because port is not supported in Kubelet
func (s *KubeServiceService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

// GetCreationTime returns the creation time of the service compare to the agent start.
func (s *KubeServiceService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

func isServiceAnnotated(ksvc *v1.Service) bool {
	_, found := ksvc.Annotations[kubeServiceAnnotationFormat]
	return found
}

// GetEntity returns the unique entity name linked to that endpoint
func (s *KubeEndpointService) GetEntity() string {
	return s.entity
}

// GetADIdentifiers returns the service AD identifiers
func (s *KubeEndpointService) GetADIdentifiers() ([]string, error) {
	// Only the entity without the IP for now, to match on annotation
	return []string{s.adID}, nil
}

// GetHosts returns the endpoint hosts
func (s *KubeEndpointService) GetHosts() (map[string]string, error) {
	return s.hosts, nil
}

// GetPid is not supported
func (s *KubeEndpointService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

// GetPorts returns the endpoint's ports
func (s *KubeEndpointService) GetPorts() ([]ContainerPort, error) {
	return s.ports, nil
}

// GetTags retrieves tags
func (s *KubeEndpointService) GetTags() ([]string, error) {
	return s.tags, nil
}

// GetHostname returns nil and an error because port is not supported in Kubelet
func (s *KubeEndpointService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

// GetCreationTime returns the creation time of the service compare to the agent start.
func (s *KubeEndpointService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

func isEndpointAnnotated(ksvc *v1.Service) bool {
	_, found := ksvc.Annotations[kubeEndpointAnnotationFormat]
	return found
}
