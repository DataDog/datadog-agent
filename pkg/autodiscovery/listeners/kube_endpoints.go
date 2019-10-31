// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package listeners

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	infov1 "k8s.io/client-go/informers/core/v1"
	listv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	kubeEndpointsAnnotationFormat = "ad.datadoghq.com/endpoints.instances"
)

// KubeEndpointsListener listens to kubernetes endpoints creation
type KubeEndpointsListener struct {
	endpointsInformer infov1.EndpointsInformer
	serviceLister     listv1.ServiceLister
	endpoints         map[types.UID][]*KubeEndpointService
	newService        chan<- Service
	delService        chan<- Service
	m                 sync.RWMutex
}

// KubeEndpointService represents an endpoint in a Kubernetes Endpoints
type KubeEndpointService struct {
	entity       string
	tags         []string
	hosts        map[string]string
	ports        []ContainerPort
	creationTime integration.CreationTime
}

func init() {
	Register("kube_endpoints", NewKubeEndpointsListener)
}

func NewKubeEndpointsListener() (ServiceListener, error) {
	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}
	endpointsInformer := ac.InformerFactory.Core().V1().Endpoints()
	if endpointsInformer == nil {
		return nil, fmt.Errorf("cannot get endpoints informer: %s", err)
	}
	serviceInformer := ac.InformerFactory.Core().V1().Services()
	if serviceInformer == nil {
		return nil, fmt.Errorf("cannot get service informer: %s", err)
	}
	return &KubeEndpointsListener{
		endpoints:         make(map[types.UID][]*KubeEndpointService),
		endpointsInformer: endpointsInformer,
		serviceLister:     serviceInformer.Lister(),
	}, nil
}

func (l *KubeEndpointsListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	l.endpointsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    l.added,
		UpdateFunc: l.updated,
		DeleteFunc: l.deleted,
	})

	// Initial fill
	endpoints, err := l.endpointsInformer.Lister().List(labels.Everything())
	if err != nil {
		log.Errorf("Cannot list Kubernetes endpoints: %s", err)
	}
	for _, e := range endpoints {
		l.createService(e, true)
	}
}

// Stop is a stub
func (l *KubeEndpointsListener) Stop() {
	// We cannot deregister from the informer
}

func (l *KubeEndpointsListener) added(obj interface{}) {
	castedObj, ok := obj.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected an Endpoints type, got: %v", obj)
		return
	}
	l.createService(castedObj, false)
}

func (l *KubeEndpointsListener) deleted(obj interface{}) {
	castedObj, ok := obj.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected an Endpoints type, got: %v", obj)
		return
	}
	l.removeService(castedObj)
}

func (l *KubeEndpointsListener) updated(old, obj interface{}) {
	// Cast the updated object or return on failure
	castedObj, ok := obj.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected an Endpoints type, got: %v", obj)
		return
	}
	// Cast the old object, consider it an add on cast failure
	castedOld, ok := old.(*v1.Endpoints)
	if !ok {
		log.Errorf("Expected an Endpoints type, got: %v", old)
		l.createService(castedObj, false)
		return
	}
	if l.endpointsDiffer(castedObj, castedOld) {
		l.removeService(castedObj)
		l.createService(castedObj, false)
	}
}

// endpointsDiffer compares two endpoints to only go forward
// when relevant fields are changed. This logic must be
// updated if more fields are used.
func (l *KubeEndpointsListener) endpointsDiffer(first, second *v1.Endpoints) bool {
	// Quick exit if resversion did not change
	if first.ResourceVersion == second.ResourceVersion {
		return false
	}
	// AD annotations on the corresponding service
	if l.isEndpointsAnnotated(first) != l.isEndpointsAnnotated(second) {
		return true
	}
	// Subsets
	return subsetsDiffer(first, second)
}

// subsetsDiffer detects if two Endpoints have different subsets.
// The function is separated from endpointsDiffer to facilitate testing.
func subsetsDiffer(first, second *v1.Endpoints) bool {
	return !equality.Semantic.DeepEqual(first.Subsets, second.Subsets)
}

// isEndpointsAnnotated looks for the corresponding service of a kubernetes endpoints object
// and returns true if the service has endpoints annotations, otherwise returns false.
func (l *KubeEndpointsListener) isEndpointsAnnotated(kep *v1.Endpoints) bool {
	ksvc, err := l.serviceLister.Services(kep.Namespace).Get(kep.Name)
	if err != nil {
		log.Tracef("Cannot get Kubernetes service: %s", err)
	}
	if ksvc != nil {
		if _, found := ksvc.Annotations[kubeEndpointsAnnotationFormat]; found {
			return true
		}
	}
	return false
}

func (l *KubeEndpointsListener) createService(kep *v1.Endpoints, firstRun bool) {
	if kep == nil {
		return
	}
	if !l.isEndpointsAnnotated(kep) {
		// Ignore endpoints with no AD annotation on their corresponding service
		return
	}

	eps := processEndpoints(kep, firstRun)

	l.m.Lock()
	l.endpoints[kep.UID] = eps
	l.m.Unlock()

	for _, ep := range eps {
		log.Debugf("Creating a new AD service: %s", ep.entity)
		l.newService <- ep
	}
}

// processEndpoints parses a kubernetes Endpoints object
// and returns a slice of KubeEndpointService per endpoint
func processEndpoints(kep *v1.Endpoints, firstRun bool) []*KubeEndpointService {
	var eps []*KubeEndpointService
	for i := range kep.Subsets {
		ports := []ContainerPort{}
		// Ports
		for _, port := range kep.Subsets[i].Ports {
			ports = append(ports, ContainerPort{int(port.Port), port.Name})
		}
		// Hosts
		for _, host := range kep.Subsets[i].Addresses {
			// create a separate AD service per host
			ep := &KubeEndpointService{
				entity:       apiserver.EntityForEndpoints(kep.Namespace, kep.Name, host.IP),
				creationTime: integration.After,
				hosts:        map[string]string{"endpoint": host.IP},
				ports:        ports,
				tags: []string{
					fmt.Sprintf("kube_service:%s", kep.Name),
					fmt.Sprintf("kube_namespace:%s", kep.Namespace),
					fmt.Sprintf("kube_endpoint_ip:%s", host.IP),
				},
			}
			if firstRun {
				ep.creationTime = integration.Before
			}
			eps = append(eps, ep)
		}
	}
	return eps
}

func (l *KubeEndpointsListener) removeService(kep *v1.Endpoints) {
	if kep == nil {
		return
	}
	l.m.RLock()
	eps, ok := l.endpoints[kep.UID]
	l.m.RUnlock()
	if ok {
		l.m.Lock()
		delete(l.endpoints, kep.UID)
		l.m.Unlock()
		for _, ep := range eps {
			log.Debugf("Deleting AD service: %s", ep.entity)
			l.delService <- ep
		}
	} else {
		log.Debugf("Entity %s not found, not removing", kep.UID)
	}
}

// GetEntity returns the unique entity name linked to that service
func (s *KubeEndpointService) GetEntity() string {
	return s.entity
}

// GetEntity returns the unique entity name linked to that service
func (s *KubeEndpointService) GetTaggerEntity() string {
	return s.entity
}

// GetADIdentifiers returns the service AD identifiers
func (s *KubeEndpointService) GetADIdentifiers() ([]string, error) {
	return []string{s.entity}, nil
}

// GetHosts returns the pod hosts
func (s *KubeEndpointService) GetHosts() (map[string]string, error) {
	if s.hosts == nil {
		return map[string]string{}, nil
	}
	return s.hosts, nil
}

// GetPid is not supported
func (s *KubeEndpointService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

// GetPorts returns the endpoint's ports
func (s *KubeEndpointService) GetPorts() ([]ContainerPort, error) {
	if s.ports == nil {
		return []ContainerPort{}, nil
	}
	return s.ports, nil
}

// GetTags retrieves tags
func (s *KubeEndpointService) GetTags() ([]string, error) {
	if s.tags == nil {
		return []string{}, nil
	}
	return s.tags, nil
}

// GetHostname returns nil and an error because port is not supported in Kubelet
func (s *KubeEndpointService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

// GetCreationTime returns the creation time of the endpoint compare to the agent start.
func (s *KubeEndpointService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

// IsReady returns if the service is ready
func (s *KubeEndpointService) IsReady() bool {
	return true
}

// GetAnnotatedCheckNames stub
func (s *KubeEndpointService) GetAnnotatedCheckNames() string {
	return ""
}
